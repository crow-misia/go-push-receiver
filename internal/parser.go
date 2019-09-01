package internal

import (
	"crypto/tls"
	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"io"
)

type Parser struct {
	conn *tls.Conn
}

func NewParser(conn *tls.Conn) *Parser {
	return &Parser{
		conn: conn,
	}
}

func (p *Parser) ReceiveVersion() error {
	buf := make([]byte, VersionPacketLen)
	length, err := p.conn.Read(buf)
	if err != nil {
		return errors.Wrap(err, "receive version packet")
	}
	if length != VersionPacketLen || buf[0] != FcmVersion {
		return errors.Errorf("Version do not match. Received %d, Expecting %d", buf[0], FcmVersion)
	}
	return nil
}

func (p *Parser) PerformReadTag() (TagType, interface{}, error) {
	var err error

	// receive tag
	tag, err := p.receiveTag()
	if err != nil {
		return TagNumProtoTypes, nil, errors.Wrap(err, "receive tag packet")
	}

	// receive size
	size, err := p.receiveSize()
	if err != nil {
		return TagNumProtoTypes, nil, errors.Wrap(err, "receive size packet")
	}

	// receive data
	offset := 0
	buf := make([]byte, size)
	for {
		length, err := p.conn.Read(buf[offset:])
		if err != nil {
			return TagNumProtoTypes, nil, errors.Wrap(err, "receive data packet")
		}
		offset += length
		if offset >= size {
			break
		}
	}

	var response proto.Message
	switch tag {
	case TagHeartbeatPing:
		response = &pb.HeartbeatPing{}
		break
	case TagHeartbeatAck:
		response = &pb.HeartbeatAck{}
		break
	case TagLoginRequest:
		response = &pb.LoginRequest{}
		break
	case TagLoginResponse:
		response = &pb.LoginResponse{}
		break
	case TagClose:
		response = &pb.Close{}
		break
	case TagIqStanza:
		response = &pb.IqStanza{}
		break
	case TagDataMessageStanza:
		response = &pb.DataMessageStanza{}
		break
	case TagStreamErrorStanza:
		response = &pb.StreamErrorStanza{}
		break
	default:
		return TagNumProtoTypes, nil, errors.Errorf("unknown tag: %x", tag)
	}
	err = proto.Unmarshal(buf, response)
	if err != nil {
		return TagNumProtoTypes, nil, errors.Wrapf(err, "unmarshal tag(%x) data", tag)
	}
	return tag, response, nil
}

func (p *Parser) receiveTag() (TagType, error) {
	buf := make([]byte, TagPacketLen)
	n, err := p.conn.Read(buf)
	if err != nil {
		return 0, err
	} else if n == 0 {
		return 0, io.ErrClosedPipe
	}
	return TagType(buf[0]), nil
}

func (p *Parser) receiveSize() (int, error) {
	offset := 0
	buf := make([]byte, SizePacketLenMax)
	for {
		if offset >= SizePacketLenMax {
			return 0, io.ErrUnexpectedEOF
		}
		length, err := p.conn.Read(buf[offset : offset+1])
		if err != nil {
			return 0, err
		}
		offset += length
		n, n2 := proto.DecodeVarint(buf[0:offset])
		if n2 > 0 {
			return int(n), nil
		}
	}
}
