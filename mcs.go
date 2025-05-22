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
	"log/slog"
	"strconv"
	"sync"
	"time"
)

type mcs struct {
	conn             *tls.Conn
	logger           *slog.Logger
	creds            *FCMCredentials
	writeTimeout     time.Duration
	incomingStreamID int32
	heartbeatAck     chan bool
	heartbeat        *Heartbeat
	disconnectDm     sync.Once
	events           chan Event
}

func newMCS(conn *tls.Conn, logger *slog.Logger, creds *FCMCredentials, heartbeat *Heartbeat, events chan Event) *mcs {
	return &mcs{
		conn:             conn,
		logger:           logger,
		creds:            creds,
		incomingStreamID: 0,
		heartbeatAck:     make(chan bool),
		heartbeat:        heartbeat,
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

func (mcs *mcs) SendLoginPacket(receivedPersistentId []string) error {
	androidID := proto.String(strconv.FormatUint(mcs.creds.AndroidId, 10))

	setting := []*pb.Setting{
		{
			Name:  proto.String("new_vc"),
			Value: proto.String("1"),
		},
	}
	if mcs.heartbeat.serverInterval > 0 {
		setting = append(setting, &pb.Setting{
			Name:  proto.String("hbping"),
			Value: proto.String(strconv.FormatInt(mcs.heartbeat.serverInterval.Milliseconds(), 10)),
		})
	}

	request := &pb.LoginRequest{
		AccountId:            proto.Int64(1000000),
		AuthService:          pb.LoginRequest_ANDROID_ID.Enum(),
		AuthToken:            proto.String(strconv.FormatUint(mcs.creds.SecurityToken, 10)),
		Id:                   proto.String(fmt.Sprintf("chrome-%s", chromeVersion)),
		Domain:               proto.String(mcsDomain),
		DeviceId:             proto.String(fmt.Sprintf("android-%s", strconv.FormatUint(mcs.creds.AndroidId, 16))),
		NetworkType:          proto.Int32(1), // Wi-Fi
		Resource:             androidID,
		User:                 androidID,
		UseRmq2:              proto.Bool(true),
		LastRmqId:            proto.Int64(1), // Sending not enabled yet so this stays as 1.
		Setting:              setting,
		AdaptiveHeartbeat:    proto.Bool(mcs.heartbeat.adaptive),
		ReceivedPersistentId: receivedPersistentId,
	}

	return mcs.sendRequest(tagLoginRequest, request, true)
}

func (mcs *mcs) SendHeartbeatPingPacket() error {
	streamID := mcs.incomingStreamID
	request := &pb.HeartbeatPing{
		LastStreamIdReceived: proto.Int32(streamID),
	}

	return mcs.sendRequest(tagHeartbeatPing, request, false)
}

func (mcs *mcs) SendHeartbeatAckPacket() error {
	streamID := mcs.incomingStreamID
	request := &pb.HeartbeatAck{
		LastStreamIdReceived: proto.Int32(streamID),
	}

	return mcs.sendRequest(tagHeartbeatAck, request, false)
}

func (mcs *mcs) sendRequest(tag tagType, request proto.Message, containVersion bool) error {
	header := make([]byte, 0, 100)
	if containVersion {
		header = append(header, fcmVersion, byte(tag))
	} else {
		header = append(header, byte(tag))
	}

	mcs.logger.Debug("MCS request", "tag", tag, "message", request)

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
	length, err := io.ReadFull(mcs.conn, buf)
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
	buf := make([]byte, size)
	_, err = io.ReadFull(mcs.conn, buf)
	if err != nil {
		return nil, errors.Wrap(err, "receive data packet")
	}

	return mcs.UnmarshalTagData(tag, buf)
}

func (mcs *mcs) UnmarshalTagData(tag tagType, buf []byte) (interface{}, error) {
	receive := tag.GenerateMessage()
	if receive == nil {
		return nil, errors.Errorf("unknown tag: %x", tag)
	}

	if err := proto.Unmarshal(buf, receive.(proto.Message)); err != nil {
		return receive, errors.Wrapf(err, "unmarshal tag(%x) data", tag)
	}

	// output receive
	mcs.logger.Debug("MCS receive", "tag", tag, "message", receive)

	// handling tag
	if err := mcs.handleTag(receive); err != nil {
		return receive, errors.Wrap(err, "handling failed.")
	}

	return receive, nil
}

func (mcs *mcs) handleTag(receive interface{}) error {
	switch receive.(type) {
	case *pb.HeartbeatPing:
		mcs.updateIncomingStreamID((*receive.(*pb.HeartbeatPing)).GetLastStreamIdReceived())
		mcs.heartbeatAck <- true
		return mcs.SendHeartbeatAckPacket()
	case *pb.HeartbeatAck:
		mcs.updateIncomingStreamID((*receive.(*pb.HeartbeatAck)).GetLastStreamIdReceived())
		mcs.heartbeatAck <- true
	case *pb.LoginResponse:
		mcs.updateIncomingStreamID((*receive.(*pb.LoginResponse)).GetLastStreamIdReceived())
	case *pb.IqStanza:
		mcs.updateIncomingStreamID((*receive.(*pb.IqStanza)).GetLastStreamIdReceived())
	}
	return nil
}

func (mcs *mcs) updateIncomingStreamID(lastStreamIdReceived int32) {
	if lastStreamIdReceived > 0 {
		mcs.incomingStreamID = lastStreamIdReceived
	}
}

func (mcs *mcs) receiveTag() (tagType, error) {
	buf := make([]byte, tagPacketLen)
	n, err := io.ReadFull(mcs.conn, buf)
	if err != nil {
		return tagUnknown, err
	}
	if n == 0 {
		return tagUnknown, io.ErrClosedPipe
	}
	return tagType(buf[0]), nil
}

func (mcs *mcs) receiveSize() (uint64, error) {
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
		v, n := protowire.ConsumeVarint(buf[0:offset])
		if n > 0 {
			return v, nil
		}
	}
}
