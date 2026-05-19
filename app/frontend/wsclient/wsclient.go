// Package wsclient provides a WebSocket client for the AIM gateway protocol.
// It uses the coder/websocket library and encodes/decodes binary frames
// using protobuf from shared/proto/ws/pb.
package wsclient

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"google.golang.org/protobuf/proto"
)

// FrameType represents the type of a WebSocket frame.
type FrameType = pb.FrameType

const (
	FrameTypeSendMessage      = pb.FrameType_FRAME_TYPE_SEND_MESSAGE
	FrameTypeHeartbeat        = pb.FrameType_FRAME_TYPE_HEARTBEAT
	FrameTypeTyping           = pb.FrameType_FRAME_TYPE_TYPING
	FrameTypeReadReceipt      = pb.FrameType_FRAME_TYPE_READ_RECEIPT
	FrameTypeAck              = pb.FrameType_FRAME_TYPE_ACK
	FrameTypePushMessage      = pb.FrameType_FRAME_TYPE_PUSH_MESSAGE
	FrameTypePushPresence     = pb.FrameType_FRAME_TYPE_PUSH_PRESENCE
	FrameTypePushNotification = pb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION
	FrameTypePushTyping       = pb.FrameType_FRAME_TYPE_PUSH_TYPING
	FrameTypeReconnect        = pb.FrameType_FRAME_TYPE_RECONNECT
	FrameTypeServerAck        = pb.FrameType_FRAME_TYPE_SERVER_ACK
	FrameTypeTokenExpired     = pb.FrameType_FRAME_TYPE_TOKEN_EXPIRED
)

// WsFrame mirrors the protobuf WsFrame for documentation and local use.
type WsFrame = pb.WsFrame

// Client handles WebSocket communication with the AIM gateway.
type Client struct {
	conn *websocket.Conn
	url  string
	opts *ClientOptions

	mu        sync.RWMutex
	connected bool
	closeOnce sync.Once
	closeChan chan struct{}
	readSeq   atomic.Int64
	writeSeq  atomic.Int64
}

// ClientOptions configure the WebSocket client.
type ClientOptions struct {
	// AccessToken is the JWT access token for authentication.
	AccessToken string
	// OnFrame called for each received frame.
	OnFrame func(frame *WsFrame)
	// OnDisconnect called when connection closes.
	OnDisconnect func(err error)
	// OnConnect called when connection is established.
	OnConnect func()
}

// NewClient creates a new WebSocket client.
func NewClient(url string, opts *ClientOptions) *Client {
	if opts == nil {
		opts = &ClientOptions{}
	}

	return &Client{
		url:       url,
		opts:      opts,
		closeChan: make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return fmt.Errorf("already connected")
	}

	conn, _, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{
		HTTPHeader: buildHeaders(c.opts.AccessToken),
	})
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("dial: %w", err)
	}

	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	if c.opts.OnConnect != nil {
		c.opts.OnConnect()
	}

	go c.readLoop(context.WithoutCancel(ctx))

	return nil
}

// buildHeaders constructs HTTP headers for the WebSocket upgrade.
func buildHeaders(accessToken string) http.Header {
	headers := http.Header{}
	if accessToken != "" {
		headers.Set("Authorization", "Bearer "+accessToken)
	}

	return headers
}

// readLoop continuously reads frames from the connection.
func (c *Client) readLoop(ctx context.Context) {
	defer func() {
		c.mu.Lock()

		var closeErr error

		c.connected = false
		if c.conn != nil {
			closeErr = c.conn.Close(websocket.StatusNormalClosure, "")
		}
		c.mu.Unlock()

		close(c.closeChan)

		if c.opts.OnDisconnect != nil {
			if closeErr != nil {
				c.opts.OnDisconnect(fmt.Errorf("close websocket: %w", closeErr))
				return
			}

			c.opts.OnDisconnect(nil)
		}
	}()

	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			if c.isClosed() {
				return
			}

			if c.opts.OnDisconnect != nil {
				c.opts.OnDisconnect(err)
			}

			return
		}

		frame, err := DecodeFrame(data)
		if err != nil {
			continue // skip malformed frames
		}

		c.readSeq.Store(frame.Seq)

		if c.opts.OnFrame != nil {
			c.opts.OnFrame(frame)
		}
	}
}

// isClosed checks if the client has been closed.
func (c *Client) isClosed() bool {
	select {
	case <-c.closeChan:
		return true
	default:
		return false
	}
}

// Disconnect closes the WebSocket connection.
func (c *Client) Disconnect() error {
	var closeErr error

	c.closeOnce.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.conn != nil {
			closeErr = c.conn.Close(websocket.StatusNormalClosure, "client disconnect")
		}
	})

	if closeErr != nil {
		return fmt.Errorf("close websocket: %w", closeErr)
	}

	return nil
}

// IsConnected returns whether the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.connected
}

// NextSeq returns the next sequence number for outgoing frames.
func (c *Client) NextSeq() int64 {
	return c.writeSeq.Add(1)
}

// SendFrame sends a binary frame to the gateway.
func (c *Client) SendFrame(ctx context.Context, frameType FrameType, payload proto.Message) error {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}

	seq := c.NextSeq()
	frame := &WsFrame{
		Type:      frameType,
		Seq:       seq,
		Timestamp: 0,
	}

	if payload != nil {
		data, err := proto.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}

		frame.Payload = data
	}

	data, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}

	err = conn.Write(ctx, websocket.MessageBinary, data)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// SendMessage sends a chat message frame.
func (c *Client) SendMessage(ctx context.Context, conversationId int64, messageType, content, clientMsgId string, mentions []string) error {
	payload := &pb.SendMessagePayload{
		ConversationId: conversationId,
		MessageType:    messageType,
		Content:        content,
		ClientMsgId:    clientMsgId,
		Mentions:       mentions,
	}

	return c.SendFrame(ctx, FrameTypeSendMessage, payload)
}

// SendHeartbeat sends a heartbeat frame.
func (c *Client) SendHeartbeat(ctx context.Context, lastSeq int64) error {
	payload := &pb.HeartbeatPayload{
		LastSeq: lastSeq,
	}

	return c.SendFrame(ctx, FrameTypeHeartbeat, payload)
}

// SendTyping sends a typing notification frame.
func (c *Client) SendTyping(ctx context.Context, conversationId int64) error {
	payload := &pb.TypingPayload{
		ConversationId: conversationId,
	}

	return c.SendFrame(ctx, FrameTypeTyping, payload)
}

// SendReadReceipt sends a read receipt frame.
func (c *Client) SendReadReceipt(ctx context.Context, conversationId, lastMsgId int64) error {
	payload := &pb.ReadReceiptPayload{
		ConversationId: conversationId,
		LastMsgId:      lastMsgId,
	}

	return c.SendFrame(ctx, FrameTypeReadReceipt, payload)
}

// SendAck sends an acknowledgment frame.
func (c *Client) SendAck(ctx context.Context, ackSeq int64) error {
	payload := &pb.ClientAckPayload{
		AckSeq: ackSeq,
	}

	return c.SendFrame(ctx, FrameTypeAck, payload)
}

// CloseChan returns the channel closed when the connection terminates.
func (c *Client) CloseChan() <-chan struct{} {
	return c.closeChan
}

// ReadSeq returns the last received sequence number.
func (c *Client) ReadSeq() int64 {
	return c.readSeq.Load()
}

// WriteSeq returns the last sent sequence number.
func (c *Client) WriteSeq() int64 {
	return c.writeSeq.Load()
}

// DecodeFrame decodes a binary WsFrame from gateway.
func DecodeFrame(data []byte) (*WsFrame, error) {
	var frame WsFrame
	if err := proto.Unmarshal(data, &frame); err != nil {
		return nil, fmt.Errorf("unmarshal frame: %w", err)
	}

	return &frame, nil
}

// EncodeFrame encodes a WsFrame to binary for sending to gateway.
func EncodeFrame(frame *WsFrame) ([]byte, error) {
	data, err := proto.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("marshal frame: %w", err)
	}

	return data, nil
}

// EncodeClientFrame encodes a client-to-server frame with the given type and payload.
func EncodeClientFrame(frameType FrameType, seq int64, payload proto.Message) (*WsFrame, error) {
	frame := &WsFrame{
		Type:      frameType,
		Seq:       seq,
		Timestamp: time.Now().UnixMilli(),
	}

	if payload != nil {
		data, err := proto.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}

		frame.Payload = data
	}

	return frame, nil
}

// ReadSeqFromFrame extracts the sequence number from a received frame.
func ReadSeqFromFrame(frame *WsFrame) int64 {
	return frame.Seq
}

type FramePayload struct {
	Frame   *WsFrame `json:"frame"`
	Payload any      `json:"payload,omitempty"`
}

func DecodePayload(frame *WsFrame) (proto.Message, error) {
	if frame == nil {
		return nil, fmt.Errorf("frame is nil")
	}

	var payload proto.Message

	switch frame.Type {
	case FrameTypeSendMessage:
		payload = &pb.SendMessagePayload{}
	case FrameTypeHeartbeat:
		payload = &pb.HeartbeatPayload{}
	case FrameTypeTyping:
		payload = &pb.TypingPayload{}
	case FrameTypeReadReceipt:
		payload = &pb.ReadReceiptPayload{}
	case FrameTypeAck:
		payload = &pb.ClientAckPayload{}
	case FrameTypePushMessage:
		payload = &pb.PushMessagePayload{}
	case FrameTypePushPresence:
		payload = &pb.PushPresencePayload{}
	case FrameTypePushNotification:
		payload = &pb.PushNotificationPayload{}
	case FrameTypePushTyping:
		payload = &pb.PushTypingPayload{}
	case FrameTypeReconnect:
		payload = &pb.ReconnectPayload{}
	case FrameTypeServerAck:
		payload = &pb.ServerAckPayload{}
	case FrameTypeTokenExpired:
		payload = &pb.TokenExpiredPayload{}
	default:
		return nil, fmt.Errorf("unsupported frame type: %s", frame.Type.String())
	}

	if len(frame.Payload) == 0 {
		return payload, nil
	}

	if err := proto.Unmarshal(frame.Payload, payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	return payload, nil
}

func DecodeFramePayload(frame *WsFrame) (*FramePayload, error) {
	payload, err := DecodePayload(frame)
	if err != nil {
		return nil, err
	}

	return &FramePayload{Frame: frame, Payload: payload}, nil
}
