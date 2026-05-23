//nolint:wsl_v5 // WS frame handlers keep gateway protocol reactions colocated.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/shared/errorx"
	client "github.com/hellopoisonx/aim/app/tui/internal/client"
	"github.com/hellopoisonx/aim/app/tui/internal/wsclient"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
)

const directConversationType = "direct"

func normalizeConversationType(t string) string {
	if t == "single" {
		return directConversationType
	}
	return t
}

func displayNameFromSenderInfo(s client.SenderInfo) string {
	if s.Name != "" {
		return s.Name
	}
	if s.Email != "" {
		return s.Email
	}
	return ""
}

func displayNameFromWSSenderInfo(s *wspb.SenderInfo) string {
	if s == nil {
		return ""
	}
	if s.Name != "" {
		return s.Name
	}
	return s.Email
}

func (m *model) rememberMessageSenders(messages []client.MessageItem) {
	for _, msg := range messages {
		if name := displayNameFromSenderInfo(msg.SenderInfo); name != "" {
			m.setUserLabel(msg.SenderID, name)
		}
	}
}

func (m *model) maybeAckFrame(frame *wsclient.WsFrame) {
	if frame == nil || frame.Seq <= 0 {
		return
	}
	switch frame.Type {
	case wsclient.FrameTypePushMessage, wsclient.FrameTypePushReadReceipt, wsclient.FrameTypePushNotification:
	default:
		return
	}
	p := m.currentProfile()
	if p == nil || p.WS == nil {
		return
	}
	if err := p.WS.SendAck(context.Background(), frame.Seq); err != nil {
		m.postEvent("ERR client ack: " + err.Error())
	}
}

func (m *model) handleServerAck(p *wspb.ServerAckPayload) {
	if p.Status == wspb.AckStatus_ACK_STATUS_ACCEPTED && p.MessageId > 0 {
		m.reconcileMessageID(p.ClientMsgId, p.MessageId)
		return
	}
	if p.Status != wspb.AckStatus_ACK_STATUS_REJECTED {
		return
	}
	m.removeMessageByClientMsgID(p.ClientMsgId)
	msg := fmt.Sprintf("ERR 消息发送失败 code=%d %s", p.Code, p.Msg)
	if p.Code == errorx.CodeRateLimit {
		msg = "ERR 发送过于频繁，请稍后再试（限流 42900，请勿重发同一条消息）"
	}
	m.postEvent(msg)
}

func (m *model) removeMessageByClientMsgID(clientMsgID string) {
	if clientMsgID == "" {
		return
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	for i := range m.state.conversations {
		msgs := m.state.conversations[i].Messages
		for j := range msgs {
			if msgs[j].ClientMsgID != clientMsgID {
				continue
			}
			msgs = append(msgs[:j], msgs[j+1:]...)
			m.state.conversations[i].Messages = msgs
			return
		}
	}
}

func (m *model) scheduleWSReconnectOnMain() {
	_ = m.cmdWSDisconnect()
	ctx := context.Background()
	if msg := m.cmdAutoRefresh(ctx); strings.HasPrefix(msg, "ERR ") {
		m.postEvent(msg)
		return
	}
	if msg := m.cmdWSConnect(ctx); msg != "" {
		m.postEvent(msg)
	}
	if msg := m.cmdPresence(ctx); msg != "" && strings.HasPrefix(msg, "ERR ") {
		m.postEvent(msg)
	}
}

func pushMessageToItem(p *wspb.PushMessagePayload) client.MessageItem {
	sentAt := p.SentAt
	if sentAt == 0 {
		sentAt = time.Now().UnixMilli()
	}
	isSystem := p.IsSystem || (p.SenderId == 0 && p.MessageType == "system")
	item := client.MessageItem{
		ID:             p.MessageId,
		ConversationID: p.ConversationId,
		SenderID:       p.SenderId,
		MessageType:    p.MessageType,
		Content:        p.Content,
		ClientMsgID:    p.ClientMsgId,
		CreatedAt:      sentAt,
		IsSystem:       isSystem,
		Mentions:       append([]string(nil), p.GetMentions()...),
	}
	if p.SenderInfo != nil {
		item.SenderInfo = client.SenderInfo{Name: p.SenderInfo.Name, Email: p.SenderInfo.Email}
	}
	return item
}
