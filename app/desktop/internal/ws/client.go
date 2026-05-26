package ws

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

type Frame = pb.WsFrame
type Handler func(*Frame)

type Client struct {
	url          string
	token        string
	conn         *websocket.Conn
	mu           sync.RWMutex
	writeMu      sync.Mutex
	closed       chan struct{}
	closeOnce    sync.Once
	closedFlag   atomic.Bool
	seq          atomic.Int64
	lastRead     atomic.Int64
	onFrame      Handler
	onDisconnect func(error)
	onConnect    func()
}

func New(url, token string, onFrame Handler, onConnect func(), onDisconnect func(error)) *Client {
	return &Client{url: url, token: token, closed: make(chan struct{}), onFrame: onFrame, onConnect: onConnect, onDisconnect: onDisconnect}
}
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return nil
	}
	h := http.Header{}
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	conn, _, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		return err
	}
	c.conn = conn
	if c.onConnect != nil {
		go c.onConnect()
	}
	go c.readLoop(context.WithoutCancel(ctx))
	go c.heartbeatLoop(context.WithoutCancel(ctx))
	return nil
}
func (c *Client) readLoop(ctx context.Context) {
	var readErr error
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close(websocket.StatusNormalClosure, "")
			c.conn = nil
		}
		c.mu.Unlock()

		if !c.closedFlag.Swap(true) {
			c.closeOnce.Do(func() { close(c.closed) })
			if c.onDisconnect != nil {
				c.onDisconnect(readErr)
			}
		}
	}()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			readErr = err
			return
		}
		f, err := Decode(data)
		if err != nil {
			continue
		}
		c.lastRead.Store(f.Seq)
		if c.onFrame != nil {
			c.onFrame(f)
		}
	}
}
func (c *Client) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		case <-t.C:
			_ = c.Heartbeat(ctx)
		}
	}
}
func (c *Client) Disconnect() error {
	var err error
	c.closedFlag.Store(true)
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conn != nil {
			err = c.conn.Close(websocket.StatusNormalClosure, "client disconnect")
			c.conn = nil
		}
	})
	return err
}
func (c *Client) IsConnected() bool { c.mu.RLock(); defer c.mu.RUnlock(); return c.conn != nil }
func (c *Client) Send(ctx context.Context, typ pb.FrameType, payload proto.Message) (*Frame, error) {
	var b []byte
	var err error
	if payload != nil {
		b, err = proto.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	f := &pb.WsFrame{Type: typ, Seq: c.seq.Add(1), Payload: b, Timestamp: time.Now().UnixMilli()}
	data, err := Encode(f)
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return nil, fmt.Errorf("websocket not connected")
	}
	if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		return nil, err
	}
	return f, nil
}
func (c *Client) SendMessage(ctx context.Context, conversationID int64, messageType, content, clientMsgID string, mentions []string) (*Frame, error) {
	return c.Send(ctx, pb.FrameType_FRAME_TYPE_SEND_MESSAGE, &pb.SendMessagePayload{ConversationId: conversationID, MessageType: messageType, Content: content, ClientMsgId: clientMsgID, Mentions: mentions})
}
func (c *Client) Typing(ctx context.Context, conversationID int64) error {
	_, err := c.Send(ctx, pb.FrameType_FRAME_TYPE_TYPING, &pb.TypingPayload{ConversationId: conversationID})
	return err
}
func (c *Client) ReadReceipt(ctx context.Context, conversationID, lastMsgID int64) error {
	_, err := c.Send(ctx, pb.FrameType_FRAME_TYPE_READ_RECEIPT, &pb.ReadReceiptPayload{ConversationId: conversationID, LastMsgId: lastMsgID})
	return err
}
func (c *Client) Ack(ctx context.Context, seq int64) error {
	_, err := c.Send(ctx, pb.FrameType_FRAME_TYPE_ACK, &pb.ClientAckPayload{AckSeq: seq})
	return err
}
func (c *Client) Heartbeat(ctx context.Context) error {
	_, err := c.Send(ctx, pb.FrameType_FRAME_TYPE_HEARTBEAT, &pb.HeartbeatPayload{LastSeq: c.lastRead.Load()})
	return err
}
func Encode(f *Frame) ([]byte, error) { return proto.Marshal(f) }
func Decode(data []byte) (*Frame, error) {
	var f pb.WsFrame
	if err := proto.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
