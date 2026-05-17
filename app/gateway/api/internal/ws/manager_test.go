package ws_test

import (
	"context"
	"sync"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestManagerRegister(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	err := mgr.Register(identity, nil, cancel)
	require.NoError(t, err)

	assert.Equal(t, 1, mgr.Count())
}

func TestManagerRegisterDuplicate(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	err := mgr.Register(identity, nil, cancel)
	require.NoError(t, err)

	// Second registration should fail
	err = mgr.Register(identity, nil, cancel)
	assert.Error(t, err)
}

func TestManagerUnregister(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	err := mgr.Register(identity, nil, cancel)
	require.NoError(t, err)

	err = mgr.Unregister(identity)
	require.NoError(t, err)
	assert.Equal(t, 0, mgr.Count())
}

func TestManagerUnregisterNotFound(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()

	identity := ws.Identity{UserID: 999, DeviceID: "nonexistent"}

	err := mgr.Unregister(identity)
	assert.Error(t, err)
}

func TestManagerGet(t *testing.T) {
	t.Parallel()

	mgr := ws.NewManager()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	identity := ws.Identity{UserID: 1, DeviceID: "device-1"}

	err := mgr.Register(identity, nil, cancel)
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
	err := mgr.Register(ws.Identity{UserID: 1, DeviceID: "device-1"}, nil, cancel1)
	require.NoError(t, err)
	err = mgr.Register(ws.Identity{UserID: 1, DeviceID: "device-2"}, nil, cancel2)
	require.NoError(t, err)

	// User 2 with one device
	_, cancel3 := context.WithCancel(context.Background())
	err = mgr.Register(ws.Identity{UserID: 2, DeviceID: "device-1"}, nil, cancel3)
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

	err := mgr.Register(ws.Identity{UserID: 1, DeviceID: "device-1"}, nil, cancel1)
	require.NoError(t, err)
	err = mgr.Register(ws.Identity{UserID: 2, DeviceID: "device-1"}, nil, cancel2)
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
			_ = mgr.Register(identity, nil, cancel)
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
			_ = mgr.Register(identity, nil, cancel)
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

	err := mgr.Register(ws.Identity{UserID: 1, DeviceID: "device-1"}, nil, cancel1)
	require.NoError(t, err)
	err = mgr.Register(ws.Identity{UserID: 2, DeviceID: "device-1"}, nil, cancel2)
	require.NoError(t, err)

	mgr.CloseAll()
	assert.Equal(t, 0, mgr.Count())
}
