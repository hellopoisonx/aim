package ws

import (
	"testing"
	"time"

	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestReplayStore(t *testing.T, capacity int, ttl time.Duration) *ReplayStore {
	t.Helper()
	store, err := NewReplayStoreWithOptions(capacity, ttl, 64)
	require.NoError(t, err)
	return store
}

func buildPushMessageFrame(seq int64) *wspb.WsFrame {
	return &wspb.WsFrame{
		Type:      wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE,
		Seq:       seq,
		Timestamp: time.Now().UnixMilli(),
	}
}

func TestReplayStoreAppendAndPendingAfter(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 16, time.Minute)
	identity := Identity{UserID: 1, DeviceID: "dev-1"}

	for _, seq := range []int64{1, 2, 3} {
		store.Append(identity, buildPushMessageFrame(seq))
	}

	pending := store.PendingAfter(identity, 1)
	require.Len(t, pending, 2)
	assert.Equal(t, int64(2), pending[0].GetSeq())
	assert.Equal(t, int64(3), pending[1].GetSeq())

	all := store.PendingAfter(identity, 0)
	assert.Len(t, all, 3)
}

func TestReplayStoreAckTrimsConfirmedFrames(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 16, time.Minute)
	identity := Identity{UserID: 2, DeviceID: "dev-2"}

	for _, seq := range []int64{10, 11, 12, 13} {
		store.Append(identity, buildPushMessageFrame(seq))
	}

	store.Ack(identity, 11)
	pending := store.PendingAfter(identity, 0)
	require.Len(t, pending, 2)
	assert.Equal(t, int64(12), pending[0].GetSeq())
	assert.Equal(t, int64(13), pending[1].GetSeq())
}

func TestReplayStoreRejectsNonReplayableFrames(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 16, time.Minute)
	identity := Identity{UserID: 3, DeviceID: "dev-3"}

	store.Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_PUSH_TYPING,
		Seq:  1,
	})
	store.Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_PUSH_PRESENCE,
		Seq:  2,
	})
	store.Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_SERVER_ACK,
		Seq:  3,
	})
	store.Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_RECONNECT,
		Seq:  4,
	})
	store.Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_TOKEN_EXPIRED,
		Seq:  5,
	})

	assert.Equal(t, 0, store.Len(identity))
}

func TestReplayStoreRejectsUnassignedSeq(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 16, time.Minute)
	identity := Identity{UserID: 4, DeviceID: "dev-4"}

	store.Append(identity, buildPushMessageFrame(0))
	assert.Equal(t, 0, store.Len(identity))
}

func TestReplayStoreCapacityTrim(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 3, time.Minute)
	identity := Identity{UserID: 5, DeviceID: "dev-5"}

	for seq := int64(1); seq <= 5; seq++ {
		store.Append(identity, buildPushMessageFrame(seq))
	}

	pending := store.PendingAfter(identity, 0)
	require.Len(t, pending, 3)
	assert.Equal(t, int64(3), pending[0].GetSeq())
	assert.Equal(t, int64(5), pending[2].GetSeq())
}

func TestReplayStoreTTLExpiration(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 16, 20*time.Millisecond)
	identity := Identity{UserID: 6, DeviceID: "dev-6"}

	store.Append(identity, buildPushMessageFrame(1))
	time.Sleep(40 * time.Millisecond)
	store.Append(identity, buildPushMessageFrame(2))

	pending := store.PendingAfter(identity, 0)
	require.Len(t, pending, 1)
	assert.Equal(t, int64(2), pending[0].GetSeq())
}

func TestReplayStoreDelete(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 16, time.Minute)
	identity := Identity{UserID: 7, DeviceID: "dev-7"}

	store.Append(identity, buildPushMessageFrame(1))
	store.Delete(identity)
	assert.Equal(t, 0, store.Len(identity))
	assert.Nil(t, store.PendingAfter(identity, 0))
}

func TestReplayStoreClonesOnAppendAndRead(t *testing.T) {
	t.Parallel()

	store := newTestReplayStore(t, 16, time.Minute)
	identity := Identity{UserID: 8, DeviceID: "dev-8"}

	frame := buildPushMessageFrame(1)
	frame.Payload = []byte("original")
	store.Append(identity, frame)

	// 修改原 frame，不应影响 pending 队列。
	frame.Payload = []byte("mutated")

	pending := store.PendingAfter(identity, 0)
	require.Len(t, pending, 1)
	assert.Equal(t, []byte("original"), pending[0].GetPayload())

	// 修改返回的克隆，再次读取仍是原值。
	pending[0].Payload = []byte("also-mutated")
	pending2 := store.PendingAfter(identity, 0)
	require.Len(t, pending2, 1)
	assert.Equal(t, []byte("original"), pending2[0].GetPayload())
}

func TestIsReplayableWhitelist(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ft       wspb.FrameType
		expected bool
	}{
		{wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE, true},
		{wspb.FrameType_FRAME_TYPE_PUSH_NOTIFICATION, true},
		{wspb.FrameType_FRAME_TYPE_PUSH_FRIEND_APPLICATION, true},
		{wspb.FrameType_FRAME_TYPE_PUSH_READ_RECEIPT, true},
		{wspb.FrameType_FRAME_TYPE_PUSH_TYPING, false},
		{wspb.FrameType_FRAME_TYPE_PUSH_PRESENCE, false},
		{wspb.FrameType_FRAME_TYPE_RECONNECT, false},
		{wspb.FrameType_FRAME_TYPE_TOKEN_EXPIRED, false},
		{wspb.FrameType_FRAME_TYPE_SERVER_ACK, false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, IsReplayable(tc.ft), tc.ft.String())
	}
}
