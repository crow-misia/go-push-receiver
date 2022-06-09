/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"crypto/tls"
	"fmt"
	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"io"
	"strconv"
	"sync"
	"time"
)

type mcs struct {
	conn             *tls.Conn
	log              ilogger
	creds            *FCMCredentials
	writeTimeout     time.Duration
	incomingStreamID int32
	heartbeatAck     chan bool
	disconnectDm     sync.Once
	events           chan Event
}

func newMCS(conn *tls.Conn, log ilogger, creds *FCMCredentials, events chan Event) *mcs {
	return &mcs{
		conn:             conn,
		log:              log,
		creds:            creds,
		incomingStreamID: 0,
		heartbeatAck:     make(chan bool),
		events:           events,
	}
}

func (mcs *mcs) disconnect() {
	mcs.disconnectDm.Do(func() {
		close(mcs.heartbeatAck)
		_ = mcs.conn.Close()
		mcs.events <- &DisconnectedEvent{}
	})
}

func (mcs *mcs) SendLoginPacket() error {
	androidID := proto.String(strconv.FormatUint(mcs.creds.AndroidID, 10))
	setting := &pb.Setting{
		Name:  proto.String("new_vc"),
		Value: proto.String("1"),
	}

	request := &pb.LoginRequest{
		AccountId:         proto.Int64(1000000),
		AuthService:       pb.LoginRequest_ANDROID_ID.Enum(),
		AuthToken:         proto.String(strconv.FormatUint(mcs.creds.SecurityToken, 10)),
		Id:                proto.String(fmt.Sprintf("chrome-%s", chromeVersion)),
		Domain:            proto.String(mcsDomain),
		DeviceId:          proto.String(fmt.Sprintf("android-%s", strconv.FormatUint(mcs.creds.AndroidID, 16))),
		NetworkType:       proto.Int32(1), // Wi-Fi
		Resource:          androidID,
		User:              androidID,
		UseRmq2:           proto.Bool(true),
		LastRmqId:         proto.Int64(1), // Sending not enabled yet so this stays as 1.
		Setting:           []*pb.Setting{setting},
		AdaptiveHeartbeat: proto.Bool(false),
	}

	return mcs.sendRequest(tagLoginRequest, request, true)
}

func (mcs *mcs) SendHeartbeatPacketPing() error {
	streamID := mcs.incomingStreamID
	request := &pb.HeartbeatPing{
		LastStreamIdReceived: proto.Int32(streamID),
	}

	return mcs.sendRequest(tagHeartbeatPing, request, false)
}

func (mcs *mcs) sendRequest(tag tagType, request proto.Message, containVersion bool) error {
	header := make([]byte, 0, 100)
	if containVersion {
		header = append(header, fcmVersion, byte(tag))
	} else {
		header = append(header, byte(tag))
	}

	mcs.log.Print("MCS request ", request)

	header = protowire.AppendVarint(header, uint64(proto.Size(request)))
	data, err := proto.Marshal(request)
	if err != nil {
		return errors.Wrap(err, "encode protocol buffer data")
	}

	// output request
	_, err = mcs.conn.Write(append(header, data...))
	return err
}

func (mcs *mcs) ReceiveVersion() error {
	buf := make([]byte, versionPacketLen)
	length, err := mcs.conn.Read(buf)
	if err != nil {
		return errors.Wrap(err, "receive version packet")
	}
	if length != versionPacketLen || buf[0] != fcmVersion {
		return errors.Errorf("Version do not match. Received %d, Expecting %d", buf[0], fcmVersion)
	}
	return nil
}

func (mcs *mcs) PerformReadTag() (interface{}, error) {
	var err error

	// receive tag
	tag, err := mcs.receiveTag()
	if err != nil {
		return nil, errors.Wrap(err, "receive tag packet")
	}

	// receive size
	size, err := mcs.receiveSize()
	if err != nil {
		return nil, errors.Wrap(err, "receive size packet")
	}

	// receive data
	offset := 0
	buf := make([]byte, size)
	for {
		length, err := mcs.conn.Read(buf[offset:])
		if err != nil {
			return nil, errors.Wrap(err, "receive data packet")
		}
		offset += length
		if offset >= size {
			break
		}
	}

	return mcs.UnmarshalTagData(tag, buf)
}

func (mcs *mcs) UnmarshalTagData(tag tagType, buf []byte) (interface{}, error) {
	var err error
	var response interface{}

	responseGenerator, exists := tagMapping[tag]
	if exists {
		response = responseGenerator()
		err = proto.Unmarshal(buf, response.(proto.Message))

		// output response
		mcs.log.Print("MCS response ", response)

		// handling tag
		mcs.handleTag(response)

		return response, errors.Wrapf(err, "unmarshal tag(%x) data", tag)
	}
	return nil, errors.Errorf("unknown tag: %x", tag)
}

func (mcs *mcs) handleTag(response interface{}) {
	switch response.(type) {
	case *pb.HeartbeatAck:
		mcs.incomingStreamID = *response.(*pb.HeartbeatAck).LastStreamIdReceived
		mcs.heartbeatAck <- true
	}
}

func (mcs *mcs) receiveTag() (tagType, error) {
	buf := make([]byte, tagPacketLen)
	n, err := mcs.conn.Read(buf)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, io.ErrClosedPipe
	}
	return tagType(buf[0]), nil
}

func (mcs *mcs) receiveSize() (int, error) {
	offset := 0
	buf := make([]byte, sizePacketLenMax)
	for {
		if offset >= sizePacketLenMax {
			return 0, io.ErrUnexpectedEOF
		}
		length, err := mcs.conn.Read(buf[offset : offset+1])
		if err != nil {
			return 0, err
		}
		offset += length
		n, n2 := protowire.ConsumeVarint(buf[0:offset])
		if n2 > 0 {
			return int(n), nil
		}
	}
}

type tagMessageGenerator func() interface{}

// Tag mappings.
var tagMapping = map[tagType]tagMessageGenerator{
	tagHeartbeatPing:     func() interface{} { return &pb.HeartbeatPing{} },
	tagHeartbeatAck:      func() interface{} { return &pb.HeartbeatAck{} },
	tagLoginRequest:      func() interface{} { return &pb.LoginRequest{} },
	tagLoginResponse:     func() interface{} { return &pb.LoginResponse{} },
	tagClose:             func() interface{} { return &pb.Close{} },
	tagIqStanza:          func() interface{} { return &pb.IqStanza{} },
	tagDataMessageStanza: func() interface{} { return &pb.DataMessageStanza{} },
	tagStreamErrorStanza: func() interface{} { return &pb.StreamErrorStanza{} },
}
