package mqs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypingConsumerSkipsOriginGatewayNode(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	svcCtx.LogicConversationClient = &fakeConversationClient{memberIDs: []int64{42, 43}}

	ctx := context.Background()
	require.NoError(t, svcCtx.RedisClient.SAdd(ctx, userGatewayKeyPrefix+"43", "0", "1").Err())

	event := typingEvent{
		FromUserID:     42,
		ConversationID: 100,
		Timestamp:      123,
		GatewayNodeID:  "0",
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)

	err = NewTypingConsumer(ctx, svcCtx).Consume(ctx, "100", string(data))
	require.NoError(t, err)
	require.Len(t, fakeGw.typingPushes, 1)
	require.Equal(t, int64(43), fakeGw.typingPushes[0].GetTargetUserId())
}

func TestReadReceiptConsumerSkipsOriginGatewayNode(t *testing.T) {
	fakeGw := newFakeGatewayClient()
	svcCtx := newTestServiceContext(t, fakeGw)
	svcCtx.LogicConversationClient = &fakeConversationClient{memberIDs: []int64{42, 43}}

	ctx := context.Background()
	require.NoError(t, svcCtx.RedisClient.SAdd(ctx, userGatewayKeyPrefix+"43", "0", "1").Err())

	event := readReceiptEvent{
		FromUserID:        42,
		ConversationID:    100,
		LastReadMessageID: 99,
		UpdatedAt:         123456,
		GatewayNodeID:     "0",
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)

	err = NewReadReceiptConsumer(ctx, svcCtx).Consume(ctx, "100", string(data))
	require.NoError(t, err)
	require.Len(t, fakeGw.readReceiptPushes, 1)
	require.Equal(t, int64(43), fakeGw.readReceiptPushes[0].GetTargetUserId())
}
