// Package ws provides WebSocket connection management for the AIM gateway.
package ws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

// Identity represents a connected client's identity.
type Identity struct {
	UserID   int64
	DeviceID string
}

// Connection represents a single WebSocket connection with its context.
type Connection struct {
	Identity    Identity
	Cancel      context.CancelFunc
	Conn        *websocket.Conn
	ExpiresAt   int64       // Unix timestamp when token expires (in seconds), 0 if no expiry
	ExpiryTimer *time.Timer // timer that fires at token expiry, nil if not set
}

// Manager tracks all active WebSocket connections by user_id and device_id.
type Manager struct {
	mu          sync.RWMutex
	connections map[Identity]*Connection
}

// NewManager creates a new connection manager.
func NewManager() *Manager {
	return &Manager{
		connections: make(map[Identity]*Connection),
	}
}

// Register adds a new connection for the given identity.
func (m *Manager) Register(ctx context.Context, identity Identity, conn *websocket.Conn, cancel context.CancelFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.connections[identity]; exists {
		return fmt.Errorf("connection already exists for user_id=%d device_id=%s", identity.UserID, identity.DeviceID)
	}

	m.connections[identity] = &Connection{
		Identity: identity,
		Cancel:   cancel,
		Conn:     conn,
	}

	logx.WithContext(ctx).Infof("ws connection registered: user_id=%d device_id=%s total=%d",
		identity.UserID, identity.DeviceID, len(m.connections))

	return nil
}

// Unregister removes a connection by identity.
func (m *Manager) Unregister(ctx context.Context, identity Identity) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[identity]
	if !exists {
		return fmt.Errorf("connection not found for user_id=%d device_id=%s", identity.UserID, identity.DeviceID)
	}

	delete(m.connections, identity)

	_ = conn.Cancel // cancel is called by handler on disconnect

	logx.WithContext(ctx).Infof("ws connection unregistered: user_id=%d device_id=%s total=%d",
		identity.UserID, identity.DeviceID, len(m.connections))

	return nil
}

// Get returns a connection by identity.
func (m *Manager) Get(identity Identity) (*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[identity]
	if !exists {
		return nil, fmt.Errorf("connection not found for user_id=%d device_id=%s", identity.UserID, identity.DeviceID)
	}

	return conn, nil
}

// Count returns the total number of active connections.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.connections)
}

// CountByUser returns the number of connections for a specific user.
func (m *Manager) CountByUser(userID int64) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0

	for _, conn := range m.connections {
		if conn.Identity.UserID == userID {
			count++
		}
	}

	return count
}

// GetByUserID returns all connections for a specific user.
func (m *Manager) GetByUserID(userID int64) []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Connection

	for _, conn := range m.connections {
		if conn.Identity.UserID == userID {
			result = append(result, conn)
		}
	}

	return result
}

// ForEachUser iterates over all connections for a specific user and calls fn for each.
func (m *Manager) ForEachUser(userID int64, fn func(*Connection)) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, conn := range m.connections {
		if conn.Identity.UserID == userID {
			fn(conn)
		}
	}
}

// All returns all active connections.
func (m *Manager) All() []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		result = append(result, conn)
	}

	return result
}

// ListIdentities returns all identities currently connected.
func (m *Manager) ListIdentities() []Identity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	identities := make([]Identity, 0, len(m.connections))
	for identity := range m.connections {
		identities = append(identities, identity)
	}

	return identities
}

// CloseAll closes all connections and clears the manager.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for identity, conn := range m.connections {
		if conn.Cancel != nil {
			conn.Cancel()
		}

		delete(m.connections, identity)
	}
}
