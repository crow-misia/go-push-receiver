/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"

	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

type mcs struct {
	conn             *tls.Conn
	logger           *slog.Logger
	creds            *FCMCredentials
	incomingStreamId int32
	heartbeatAck     chan bool
	heartbeat        *Heartbeat
	disconnectDm     sync.Once
	events           chan Event
}

func (c *Client) newMCS(conn *tls.Conn) *mcs {
	return &mcs{
		conn:             conn,
		logger:           c.logger,
		creds:            c.creds,
		incomingStreamId: 0,
		heartbeatAck:     make(chan bool),
		heartbeat:        c.heartbeat,
		events:           c.Events,
	}
}

func (mcs *mcs) disconnect(reason string) {
	mcs.disconnectDm.Do(func() {
		close(mcs.heartbeatAck)
		mcs.events <- &DisconnectedEvent{Reason: reason}
	})
}

func (mcs *mcs) SendLoginPacket(ctx context.Context, receivedPersistentId []string) error {
	androidId := proto.String(strconv.FormatInt(mcs.creds.AndroidID, 10))

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
		DeviceId:             proto.String(fmt.Sprintf("android-%s", strconv.FormatInt(mcs.creds.AndroidID, 16))),
		NetworkType:          proto.Int32(1), // Wi-Fi
		Resource:             androidId,
		User:                 androidId,
		UseRmq2:              proto.Bool(true),
		LastRmqId:            proto.Int64(1), // Sending not enabled yet so this stays as 1.
		Setting:              setting,
		AdaptiveHeartbeat:    proto.Bool(mcs.heartbeat.adaptive),
		ReceivedPersistentId: receivedPersistentId,
	}

	return mcs.sendRequest(ctx, tagLoginRequest, request, true)
}

func (mcs *mcs) SendHeartbeatPingPacket(ctx context.Context) error {
	streamId := mcs.incomingStreamId
	request := &pb.HeartbeatPing{
		LastStreamIdReceived: proto.Int32(streamId),
	}

	return mcs.sendRequest(ctx, tagHeartbeatPing, request, false)
}

func (mcs *mcs) SendHeartbeatAckPacket(ctx context.Context) error {
	streamId := mcs.incomingStreamId
	request := &pb.HeartbeatAck{
		LastStreamIdReceived: proto.Int32(streamId),
	}

	return mcs.sendRequest(ctx, tagHeartbeatAck, request, false)
}

func (mcs *mcs) sendRequest(ctx context.Context, tag tagType, request proto.Message, containVersion bool) error {
	header := make([]byte, 0, 100)
	if containVersion {
		header = append(header, fcmVersion, byte(tag))
	} else {
		header = append(header, byte(tag))
	}

	if mcs.logger.Enabled(ctx, slog.LevelDebug) {
		mcs.logger.Debug("MCS request", "tag", tag, "message", protojson.MarshalOptions{Multiline: false}.Format(request))
	}

	requestSize := proto.Size(request)
	if requestSize < 0 {
		return fmt.Errorf("invalid request size %d", requestSize)
	}
	header = protowire.AppendVarint(header, uint64(requestSize))
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

func (mcs *mcs) PerformReadTag(ctx context.Context) (proto.Message, error) {
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

	return mcs.UnmarshalTagData(ctx, tag, buf)
}

func (mcs *mcs) UnmarshalTagData(ctx context.Context, tag tagType, buf []byte) (proto.Message, error) {
	receive := tag.GenerateMessage()
	if receive == nil {
		return nil, errors.Errorf("unknown tag: %x", tag)
	}

	if err := proto.Unmarshal(buf, receive); err != nil {
		return receive, errors.Wrapf(err, "unmarshal tag(%x) data", tag)
	}

	// output receive
	if mcs.logger.Enabled(ctx, slog.LevelDebug) {
		mcs.logger.Debug("MCS receive", "tag", tag, "message", protojson.MarshalOptions{Multiline: false}.Format(receive))
	}

	// handling tag
	if err := mcs.handleTag(ctx, receive); err != nil {
		return receive, errors.Wrap(err, "handling failed.")
	}

	return receive, nil
}

func (mcs *mcs) handleTag(ctx context.Context, receive proto.Message) error {
	switch data := receive.(type) {
	case *pb.HeartbeatPing:
		mcs.updateIncomingStreamId(data.GetLastStreamIdReceived())
		mcs.heartbeatAck <- true
		return mcs.SendHeartbeatAckPacket(ctx)
	case *pb.HeartbeatAck:
		mcs.updateIncomingStreamId(data.GetLastStreamIdReceived())
		mcs.heartbeatAck <- true
	case *pb.LoginResponse:
		mcs.updateIncomingStreamId(data.GetLastStreamIdReceived())
	case *pb.IqStanza:
		mcs.updateIncomingStreamId(data.GetLastStreamIdReceived())
	}
	return nil
}

func (mcs *mcs) updateIncomingStreamId(lastStreamIdReceived int32) {
	if lastStreamIdReceived > 0 {
		mcs.incomingStreamId = lastStreamIdReceived
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
