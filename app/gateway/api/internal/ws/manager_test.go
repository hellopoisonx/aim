package ws_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	wspb "github.com/hellopoisonx/aim/shared/proto/ws/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// collection.Cache 启动的 timingwheel / cacheStat goroutine 为进程级常驻，
	// 无公开关闭接口；ReplayStore 在 Manager 初始化时会拉起它们。
	// go-zero proc.signals / stat.usage 是包初始化的进程级 goroutine，不会退出。
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/zeromicro/go-zero/core/collection.(*TimingWheel).run"),
		goleak.IgnoreTopFunction("github.com/zeromicro/go-zero/core/collection.(*cacheStat).statLoop"),
		goleak.IgnoreTopFunction("github.com/zeromicro/go-zero/core/proc.init.1.func1"),
		goleak.IgnoreTopFunction("github.com/zeromicro/go-zero/core/stat.init.1.func1"),
	)
}

func TestManagerRegister(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	assert.Equal(t, 1, mgr.Count())
}

func TestManagerRegisterDuplicate(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	// Second registration should fail
	_, err = mgr.Register(context.Background(), identity, nil, cancel)
	assert.Error(t, err)
}

func TestManagerUnregister(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	_, err = mgr.Unregister(context.Background(), identity)
	require.NoError(t, err)
	assert.Equal(t, 0, mgr.Count())
}

func TestManagerUnregisterNotFound(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	identity := ws.Identity{UserID: 999, DeviceID: "nonexistent"}

	_, err := mgr.Unregister(context.Background(), identity)
	assert.Error(t, err)
}

func TestManagerGet(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	conn, err := mgr.Get(identity)
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.Equal(t, identity, conn.Identity)
}

func TestManagerGetNotFound(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	identity := ws.Identity{UserID: 999, DeviceID: "nonexistent"}

	_, err := mgr.Get(identity)
	assert.Error(t, err)
}

func TestManagerCountByUser(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	_, cancel1 := context.WithCancel(context.Background())
	_, cancel2 := context.WithCancel(context.Background())

	// User 1 with two devices
	_, err := mgr.Register(context.Background(), ws.Identity{UserID: 1, DeviceID: "device-1"}, nil, cancel1)
	require.NoError(t, err)
	_, err = mgr.Register(context.Background(), ws.Identity{UserID: 1, DeviceID: "device-2"}, nil, cancel2)
	require.NoError(t, err)

	// User 2 with one device
	_, cancel3 := context.WithCancel(context.Background())
	_, err = mgr.Register(context.Background(), ws.Identity{UserID: 2, DeviceID: "device-1"}, nil, cancel3)
	require.NoError(t, err)

	assert.Equal(t, 2, mgr.CountByUser(1))
	assert.Equal(t, 1, mgr.CountByUser(2))
	assert.Equal(t, 0, mgr.CountByUser(999))
}

func TestManagerListIdentities(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	_, cancel1 := context.WithCancel(context.Background())
	_, cancel2 := context.WithCancel(context.Background())

	_, err := mgr.Register(context.Background(), ws.Identity{UserID: 1, DeviceID: "device-1"}, nil, cancel1)
	require.NoError(t, err)
	_, err = mgr.Register(context.Background(), ws.Identity{UserID: 2, DeviceID: "device-1"}, nil, cancel2)
	require.NoError(t, err)

	identities := mgr.ListIdentities()
	assert.Len(t, identities, 2)
}

func TestManagerConcurrent(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	var wg sync.WaitGroup

	// Concurrent registrations
	for i := int64(1); i <= 10; i++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()

			_, cancel := context.WithCancel(context.Background())
			identity := ws.Identity{UserID: userID, DeviceID: "device-1"}
			_, _ = mgr.Register(context.Background(), identity, nil, cancel)
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 10, mgr.Count())
}

func TestManagerConcurrentSameUser(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	var wg sync.WaitGroup

	// Same user, different devices
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(deviceID string) {
			defer wg.Done()

			_, cancel := context.WithCancel(context.Background())
			identity := ws.Identity{UserID: 1, DeviceID: deviceID}
			_, _ = mgr.Register(context.Background(), identity, nil, cancel)
		}("device-" + string(rune('a'+i-1)))
	}

	wg.Wait()
	assert.Equal(t, 5, mgr.CountByUser(1))
}

func TestManagerCloseAll(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	_, cancel1 := context.WithCancel(context.Background())
	_, cancel2 := context.WithCancel(context.Background())

	_, err := mgr.Register(context.Background(), ws.Identity{UserID: 1, DeviceID: "device-1"}, nil, cancel1)
	require.NoError(t, err)
	_, err = mgr.Register(context.Background(), ws.Identity{UserID: 2, DeviceID: "device-1"}, nil, cancel2)
	require.NoError(t, err)

	mgr.CloseAll()
	assert.Equal(t, 0, mgr.Count())
}

// ─── Reconnect Grace Tests ────────────────────────────────────────────────────

func TestMarkTokenExpiredCreatesGrace(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetReconnectGrace(30 * time.Second)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	// Mark as token expired
	mgr.MarkTokenExpired(identity)

	// Unregister should not publish offline but register grace
	result, err := mgr.Unregister(context.Background(), identity)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Switched, "should not switch to offline (grace active)")

	// Grace should be pending
	assert.True(t, mgr.HasPendingGrace(identity), "grace should be pending")
}

func TestCancelPendingOfflineOnReregister(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetReconnectGrace(30 * time.Second)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}
	_, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	_, err := mgr.Register(context.Background(), identity, nil, cancel1)
	require.NoError(t, err)

	// Simulate token expired → unregister
	mgr.MarkTokenExpired(identity)
	_, err = mgr.Unregister(context.Background(), identity)
	require.NoError(t, err)
	assert.True(t, mgr.HasPendingGrace(identity))

	// Re-register same user/device — should cancel pending offline
	_, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	_, err = mgr.Register(context.Background(), identity, nil, cancel2)
	require.NoError(t, err)

	assert.False(t, mgr.HasPendingGrace(identity), "grace should be cancelled after re-register")
}

func TestNormalUnregisterStillPublishesOffline(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetReconnectGrace(30 * time.Second)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	// Normal unregister (no token expired mark)
	result, err := mgr.Unregister(context.Background(), identity)
	require.NoError(t, err)
	// With nil redis, the PresenceResult just returns empty, but the connection
	// is removed and no grace is registered
	assert.False(t, mgr.HasPendingGrace(identity), "no grace for normal disconnect")
	assert.Equal(t, 0, mgr.Count())
	require.NotNil(t, result)
}

// ─── Stale Connection Tests ──────────────────────────────────────────────────

func TestScanStaleConnections(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetHeartbeatTimeout(10 * time.Second)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	// Immediately after register, LastSeen is now — connection is not stale
	stale := mgr.ScanStaleConnections(time.Now().UnixMilli())
	assert.Empty(t, stale)

	// Move time forward past heartbeat timeout
	future := time.Now().Add(20 * time.Second).UnixMilli()
	stale = mgr.ScanStaleConnections(future)
	assert.Len(t, stale, 1)
	assert.Equal(t, identity, stale[0])
}

func TestRecordHeartbeatPreventsStale(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetHeartbeatTimeout(10 * time.Second)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	// Record heartbeat now — updates LastSeen
	mgr.RecordHeartbeat(context.Background(), identity)

	// Immediately after heartbeat, connection should not be stale
	stale := mgr.ScanStaleConnections(time.Now().UnixMilli())
	assert.Empty(t, stale)

	// After heartbeat but before timeout — still not stale
	stale = mgr.ScanStaleConnections(time.Now().Add(5 * time.Second).UnixMilli())
	assert.Empty(t, stale)
}

func TestScanStaleConnectionsNoTimeout(t *testing.T) {
	t.Parallel()

	// Manager with zero heartbeat timeout → scanning is disabled
	mgr := ws.NewManager()
	mgr.SetHeartbeatTimeout(0)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	stale := mgr.ScanStaleConnections(time.Now().Add(1 * time.Hour).UnixMilli())
	assert.Nil(t, stale, "nil returned when heartbeat timeout is disabled")
}

// ─── Grace Expiry Tests ─────────────────────────────────────────────────────

func TestScanExpiredGraces(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetReconnectGrace(100 * time.Millisecond)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	mgr.MarkTokenExpired(identity)
	_, err = mgr.Unregister(context.Background(), identity)
	require.NoError(t, err)

	// Grace not expired yet
	expired := mgr.ScanExpiredGraces(time.Now())
	assert.Empty(t, expired)

	// Wait for grace to expire
	time.Sleep(150 * time.Millisecond)
	expired = mgr.ScanExpiredGraces(time.Now())
	assert.Len(t, expired, 1)
	assert.Equal(t, identity, expired[0])

	// Remove expired grace
	mgr.RemoveExpiredGrace(identity)
	assert.False(t, mgr.HasPendingGrace(identity))
}

func TestForceOfflineForExpiredGrace(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetReconnectGrace(30 * time.Second)

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	// With nil redis, ForceOffline returns empty result
	result, err := mgr.ForceOfflineForExpiredGrace(context.Background(), identity)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Switched)
}

// ─── Multi-device Tests ──────────────────────────────────────────────────────

func TestMultiDeviceOnlineOffline(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetReconnectGrace(30 * time.Second)

	identity1 := ws.Identity{UserID: 1, DeviceID: "device-1"}
	identity2 := ws.Identity{UserID: 1, DeviceID: "device-2"}
	_, cancel1 := context.WithCancel(context.Background())
	_, cancel2 := context.WithCancel(context.Background())

	_, err := mgr.Register(context.Background(), identity1, nil, cancel1)
	require.NoError(t, err)
	_, err = mgr.Register(context.Background(), identity2, nil, cancel2)
	require.NoError(t, err)

	assert.Equal(t, 2, mgr.CountByUser(1))

	// Unregister one device — user should NOT go offline (still 1 device)
	result, err := mgr.Unregister(context.Background(), identity1)
	require.NoError(t, err)
	assert.NotNil(t, result)
	// With nil redis the presence result just returns empty
	assert.Equal(t, 1, mgr.CountByUser(1))

	// Unregister last device
	result, err = mgr.Unregister(context.Background(), identity2)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, mgr.CountByUser(1))
}

func TestMultiDeviceTokenExpiredDoesNotOffline(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManagerWithPresence(nil, "node-1", 60)
	mgr.SetReconnectGrace(30 * time.Second)

	identity1 := ws.Identity{UserID: 1, DeviceID: "device-1"}
	identity2 := ws.Identity{UserID: 1, DeviceID: "device-2"}
	_, cancel1 := context.WithCancel(context.Background())
	_, cancel2 := context.WithCancel(context.Background())

	_, err := mgr.Register(context.Background(), identity1, nil, cancel1)
	require.NoError(t, err)
	_, err = mgr.Register(context.Background(), identity2, nil, cancel2)
	require.NoError(t, err)

	// Device 1 token expires
	mgr.MarkTokenExpired(identity1)
	result, err := mgr.Unregister(context.Background(), identity1)
	require.NoError(t, err)
	// Should not switch to offline (grace + other device still connected)
	assert.False(t, result.Switched, "should not switch to offline: another device is connected")
	assert.Equal(t, 1, mgr.CountByUser(1)) // device-2 still connected
}

func TestManagerRecordClientAckTrimsReplayStore(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	identity := ws.Identity{UserID: 9901, DeviceID: "ack-device"}
	_, cancel := context.WithCancel(context.Background())

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	mgr.ReplayStore().Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE,
		Seq:  10,
	})
	mgr.ReplayStore().Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE,
		Seq:  11,
	})
	mgr.ReplayStore().Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE,
		Seq:  12,
	})

	require.True(t, mgr.RecordClientAck(identity, 11))
	conn, err := mgr.Get(identity)
	require.NoError(t, err)
	assert.Equal(t, int64(11), conn.LastAckedSeq)

	pending := mgr.ReplayStore().PendingAfter(identity, 0)
	require.Len(t, pending, 1)
	assert.Equal(t, int64(12), pending[0].GetSeq())
}

func TestManagerUnregisterDeletesReplayStore(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	identity := ws.Identity{UserID: 9902, DeviceID: "delete-device"}
	_, cancel := context.WithCancel(context.Background())

	_, err := mgr.Register(context.Background(), identity, nil, cancel)
	require.NoError(t, err)

	mgr.ReplayStore().Append(identity, &wspb.WsFrame{
		Type: wspb.FrameType_FRAME_TYPE_PUSH_MESSAGE,
		Seq:  1,
	})
	require.Equal(t, 1, mgr.ReplayStore().Len(identity))

	_, err = mgr.Unregister(context.Background(), identity)
	require.NoError(t, err)
	assert.Equal(t, 0, mgr.ReplayStore().Len(identity))
}
