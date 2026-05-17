// Package ws provides WebSocket frame codec helpers for the AIM gateway.
package ws

import (
	"fmt"
	"time"

	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"google.golang.org/protobuf/proto"
)

// EncodeFrame serializes a WsFrame protobuf to bytes.
func EncodeFrame(frame *pb.WsFrame) ([]byte, error) {
	data, err := proto.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("encode wsframe: %w", err)
	}
	return data, nil
}

// DecodeFrame deserializes bytes into a WsFrame protobuf.
func DecodeFrame(data []byte) (*pb.WsFrame, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("decode wsframe: empty data")
	}
	var frame pb.WsFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		return nil, fmt.Errorf("decode wsframe: %w", err)
	}
	return &frame, nil
}

// EncodePayload serializes a specific payload protobuf to bytes.
func EncodePayload(msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	return data, nil
}

// DecodePayload deserializes bytes into the appropriate payload type based on FrameType.
// It returns the concrete payload type or an error if the type is unknown.
func DecodePayload(frame *pb.WsFrame) (proto.Message, error) {
	switch frame.GetType() {
	case pb.FrameType_FRAME_TYPE_SEND_MESSAGE:
		var payload pb.SendMessagePayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode sendmessage: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_HEARTBEAT:
		var payload pb.HeartbeatPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode heartbeat: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_TYPING:
		var payload pb.TypingPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode typing: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_READ_RECEIPT:
		var payload pb.ReadReceiptPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode readreceipt: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_ACK:
		var payload pb.ClientAckPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode clientack: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_PUSH_MESSAGE:
		var payload pb.PushMessagePayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode pushmessage: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_PUSH_PRESENCE:
		var payload pb.PushPresencePayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode pushpresence: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION:
		var payload pb.PushNotificationPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode pushnotification: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_PUSH_TYPING:
		var payload pb.PushTypingPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode pushtyping: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_RECONNECT:
		var payload pb.ReconnectPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode reconnect: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED:
		var payload pb.TokenExpiredPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode tokenexpired: %w", err)
		}
		return &payload, nil
	case pb.FrameType_FRAME_TYPE_SERVER_ACK:
		var payload pb.ServerAckPayload
		if err := proto.Unmarshal(frame.GetPayload(), &payload); err != nil {
			return nil, fmt.Errorf("decode serverack: %w", err)
		}
		return &payload, nil
	default:
		return nil, fmt.Errorf("unknown frame type: %v", frame.GetType())
	}
}

// BuildFrame creates a WsFrame with the given type, sequence number, payload, and current timestamp.
func BuildFrame(frameType pb.FrameType, seq int64, payload []byte) *pb.WsFrame {
	return &pb.WsFrame{
		Type:      frameType,
		Seq:       seq,
		Payload:   payload,
		Timestamp: nowUnixMilli(),
	}
}

// NewServerAck builds a SERVER_ACK frame responding to a client frame.
// It is used for heartbeat ACK which doesn't carry status/code/message_id.
func NewServerAck(ackSeq int64, clientMsgID string, seq int64) (*pb.WsFrame, error) {
	payload := &pb.ServerAckPayload{
		AckSeq:      ackSeq,
		ClientMsgId: clientMsgID,
	}
	data, err := EncodePayload(payload)
	if err != nil {
		return nil, err
	}
	return BuildFrame(pb.FrameType_FRAME_TYPE_SERVER_ACK, seq, data), nil
}

// NewServerAckExtended builds a SERVER_ACK frame with extended fields for message send results.
func NewServerAckExtended(
	ackSeq int64,
	clientMsgID string,
	seq int64,
	status pb.AckStatus,
	code int32,
	msg string,
	messageID int64,
) (*pb.WsFrame, error) {
	payload := &pb.ServerAckPayload{
		AckSeq:      ackSeq,
		ClientMsgId: clientMsgID,
		Status:      status,
		Code:        code,
		Msg:         msg,
		MessageId:   messageID,
	}
	data, err := EncodePayload(payload)
	if err != nil {
		return nil, err
	}
	return BuildFrame(pb.FrameType_FRAME_TYPE_SERVER_ACK, seq, data), nil
}

// nowUnixMilli returns the current Unix timestamp in milliseconds.
func nowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
