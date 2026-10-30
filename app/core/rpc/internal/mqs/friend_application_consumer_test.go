package mqs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/events"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// recordingPusher captures friend-application pushes.
type recordingPusher struct {
	pushes []*gwpb.PushFriendApplicationReq
}

func (p *recordingPusher) PushMessage(context.Context, *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	return &gwpb.PushMessageResp{}, nil
}

func (p *recordingPusher) PushPresence(context.Context, *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	return &gwpb.PushPresenceResp{}, nil
}

func (p *recordingPusher) PushTyping(context.Context, *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	return &gwpb.PushTypingResp{}, nil
}

func (p *recordingPusher) PushReadReceipt(context.Context, *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	return &gwpb.PushReadReceiptResp{}, nil
}

func (p *recordingPusher) PushNotification(context.Context, *gwpb.PushNotificationReq) (*gwpb.PushNotificationResp, error) {
	return &gwpb.PushNotificationResp{}, nil
}

func (p *recordingPusher) PushFriendApplication(_ context.Context, req *gwpb.PushFriendApplicationReq) (*gwpb.PushFriendApplicationResp, error) {
	p.pushes = append(p.pushes, req)
	return &gwpb.PushFriendApplicationResp{Success: true}, nil
}

func newFriendApplicationSvcCtx(t *testing.T, pusher *recordingPusher, nodes ...string) *svc.ServiceContext {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	if len(nodes) > 0 {
		require.NoError(t, client.SAdd(t.Context(), "aim:user_gateway:42", toAny(nodes)...).Err())
	}

	return &svc.ServiceContext{GatewayClient: pusher, RedisClient: client}
}

func toAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}

	return out
}

func TestFriendApplicationConsumerPushesToTargetNode(t *testing.T) {
	pusher := &recordingPusher{}
	svcCtx := newFriendApplicationSvcCtx(t, pusher, "gw-a")

	payload, err := json.Marshal(events.FriendApplicationEvent{
		TargetUserID: 42, UserID: 7, FriendID: 42, Status: "pending",
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
	})
	require.NoError(t, err)

	consumer := NewFriendApplicationConsumer(t.Context(), svcCtx)
	require.NoError(t, consumer.Consume(t.Context(), "42", string(payload)))

	require.Len(t, pusher.pushes, 1)
	require.Equal(t, int64(42), pusher.pushes[0].GetTargetUserId())
	require.Equal(t, int64(7), pusher.pushes[0].GetUserId())
	require.Equal(t, "pending", pusher.pushes[0].GetStatus())
}

func TestFriendApplicationConsumerDropsBadPayload(t *testing.T) {
	pusher := &recordingPusher{}
	consumer := NewFriendApplicationConsumer(t.Context(), newFriendApplicationSvcCtx(t, pusher))

	// A malformed payload must not wedge the partition.
	require.NoError(t, consumer.Consume(t.Context(), "42", "not json"))
	require.Empty(t, pusher.pushes)
}

func TestFriendApplicationConsumerSkipsWithoutGatewayClient(t *testing.T) {
	consumer := NewFriendApplicationConsumer(t.Context(), &svc.ServiceContext{})
	payload, err := json.Marshal(events.FriendApplicationEvent{TargetUserID: 42})
	require.NoError(t, err)
	require.NoError(t, consumer.Consume(t.Context(), "42", string(payload)))
}
