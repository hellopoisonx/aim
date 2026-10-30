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
		return decodeInto(frame, &pb.SendMessagePayload{})
	case pb.FrameType_FRAME_TYPE_HEARTBEAT:
		return decodeInto(frame, &pb.HeartbeatPayload{})
	case pb.FrameType_FRAME_TYPE_TYPING:
		return decodeInto(frame, &pb.TypingPayload{})
	case pb.FrameType_FRAME_TYPE_READ_RECEIPT:
		return decodeInto(frame, &pb.ReadReceiptPayload{})
	case pb.FrameType_FRAME_TYPE_ACK:
		return decodeInto(frame, &pb.ClientAckPayload{})
	case pb.FrameType_FRAME_TYPE_PUSH_MESSAGE:
		return decodeInto(frame, &pb.PushMessagePayload{})
	case pb.FrameType_FRAME_TYPE_PUSH_PRESENCE:
		return decodeInto(frame, &pb.PushPresencePayload{})
	case pb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION:
		return decodeInto(frame, &pb.PushNotificationPayload{})
	case pb.FrameType_FRAME_TYPE_PUSH_FRIEND_APPLICATION:
		return decodeInto(frame, &pb.PushFriendApplicationPayload{})
	case pb.FrameType_FRAME_TYPE_PUSH_TYPING:
		return decodeInto(frame, &pb.PushTypingPayload{})
	case pb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT:
		return decodeInto(frame, &pb.PushReadReceiptPayload{})
	case pb.FrameType_FRAME_TYPE_RECONNECT:
		return decodeInto(frame, &pb.ReconnectPayload{})
	case pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED:
		return decodeInto(frame, &pb.TokenExpiredPayload{})
	case pb.FrameType_FRAME_TYPE_SERVER_ACK:
		return decodeInto(frame, &pb.ServerAckPayload{})
	default:
		return nil, fmt.Errorf("unknown frame type: %v", frame.GetType())
	}
}

// decodeInto unmarshals the frame payload into target.
func decodeInto[T proto.Message](frame *pb.WsFrame, target T) (proto.Message, error) {
	if err := proto.Unmarshal(frame.GetPayload(), target); err != nil {
		return nil, fmt.Errorf("decode %s: %w", frame.GetType(), err)
	}

	return target, nil
}

// ServerAck describes the fields of a SERVER_ACK frame.
type ServerAck struct {
	// AckSeq is the seq of the client frame being acknowledged.
	AckSeq int64
	// ClientMsgID echoes the client's message id (may be empty).
	ClientMsgID string
	// Seq is the server sequence number for this frame (0 lets the writer
	// assign one).
	Seq int64
	// Status classifies the acknowledgement.
	Status pb.AckStatus
	// Code is the biz code for rejected/retryable acks.
	Code int32
	// Msg is the human-readable reason.
	Msg string
	// MessageID is the stored message id for accepted sends.
	MessageID int64
}

// NewServerAckFrame builds a SERVER_ACK frame from a ServerAck value.
func NewServerAckFrame(ack ServerAck) (*pb.WsFrame, error) {
	payload := &pb.ServerAckPayload{
		AckSeq:      ack.AckSeq,
		ClientMsgId: ack.ClientMsgID,
		Status:      ack.Status,
		Code:        ack.Code,
		Msg:         ack.Msg,
		MessageId:   ack.MessageID,
	}

	data, err := EncodePayload(payload)
	if err != nil {
		return nil, err
	}

	return BuildFrame(pb.FrameType_FRAME_TYPE_SERVER_ACK, ack.Seq, data), nil
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

// nowUnixMilli returns the current Unix timestamp in milliseconds.
func nowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
