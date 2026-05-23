package mqs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/stretchr/testify/require"
)

func TestConversationEventConsumer_SystemSenderInfo(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	consumer := NewConversationEventConsumer(context.Background(), &svc.ServiceContext{GatewayClient: fakeGw})

	event := conversationEvent{
		MessageID:      1,
		ConversationID: 2,
		SenderID:       0,
		MessageType:    "system",
		Content:        `{"event":"member_joined"}`,
		TargetUserIDs:  []int64{10, 11},
		Timestamp:      1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	require.NoError(t, consumer.Consume(context.Background(), "2", string(value)))
	require.Len(t, fakeGw.pushes, 2)
	for _, push := range fakeGw.pushes {
		require.True(t, push.req.GetIsSystem())
		require.Equal(t, int64(0), push.req.GetSenderId())
		require.Equal(t, sharedconversation.SystemSenderName, push.req.GetSenderInfo().GetName())
		require.Equal(t, sharedconversation.SystemSenderEmail, push.req.GetSenderInfo().GetEmail())
	}
}
