/*
 * Copyright (c) 2025 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"fmt"
	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	"google.golang.org/protobuf/proto"
)

// tagType is FCM Request/Response Tag type
type tagType byte

// Tag enumeration.
const (
	tagHeartbeatPing       tagType = 0
	tagHeartbeatAck        tagType = 1
	tagLoginRequest        tagType = 2
	tagLoginResponse       tagType = 3
	tagClose               tagType = 4
	tagMessageStanza       tagType = 5
	tagPresenceStanza      tagType = 6
	tagIqStanza            tagType = 7
	tagDataMessageStanza   tagType = 8
	tagBatchPresenceStanza tagType = 9
	tagStreamErrorStanza   tagType = 10
	tagHTTPRequest         tagType = 11
	tagHTTPResponse        tagType = 12
	tagBindAccountRequest  tagType = 13
	tagBindAccountResponse tagType = 14
	tagTalkMetadata        tagType = 15
	tagNumProtoTypes       tagType = 16
	tagUnknown             tagType = 255
)

func (t tagType) String() string {
	switch t {
	case tagHeartbeatPing:
		return "HeartbeatPing(0)"
	case tagHeartbeatAck:
		return "HeartbeatAck(1)"
	case tagLoginRequest:
		return "LoginRequest(2)"
	case tagLoginResponse:
		return "LoginResponse(3)"
	case tagClose:
		return "Close(4)"
	case tagMessageStanza:
		return "MessageStanza(5)"
	case tagPresenceStanza:
		return "PresenceStanza(6)"
	case tagIqStanza:
		return "IqStanza(7)"
	case tagDataMessageStanza:
		return "DataMessageStanza(8)"
	case tagBatchPresenceStanza:
		return "BatchPresenceStanza(9)"
	case tagStreamErrorStanza:
		return "StreamErrorStanza(10)"
	case tagHTTPRequest:
		return "HTTPRequest(11)"
	case tagHTTPResponse:
		return "HTTPResponse(12)"
	case tagBindAccountRequest:
		return "BindAccountRequest(13)"
	case tagBindAccountResponse:
		return "BindAccountResponse(14)"
	case tagTalkMetadata:
		return "TalkMetadata(15)"
	case tagNumProtoTypes:
		return "NumProtoTypes(16)"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

// GenerateMessage Generate Tag Message
func (t tagType) GenerateMessage() proto.Message {
	switch t {
	case tagHeartbeatPing:
		return &pb.HeartbeatPing{}
	case tagHeartbeatAck:
		return &pb.HeartbeatAck{}
	case tagLoginRequest:
		return &pb.LoginRequest{}
	case tagLoginResponse:
		return &pb.LoginResponse{}
	case tagClose:
		return &pb.Close{}
	case tagIqStanza:
		return &pb.IqStanza{}
	case tagDataMessageStanza:
		return &pb.DataMessageStanza{}
	case tagStreamErrorStanza:
		return &pb.StreamErrorStanza{}
	default:
		return nil
	}
}
