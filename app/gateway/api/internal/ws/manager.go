// Package ws provides WebSocket connection management for the AIM gateway.
package ws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"
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

// PresenceResult describes the outcome of a Register/Unregister on the user-level presence Set.
type PresenceResult struct {
	// Switched is true when the user-level presence SCARD crossed 0↔1 (online) or 1↔0 (offline).
	Switched bool
	// Status is "online" when the user just became present, "offline" when absent.
	Status string
}

// Manager tracks all active WebSocket connections by user_id and device_id.
type Manager struct {
	mu          sync.RWMutex
	connections map[Identity]*Connection
	redisClient *redis.Client
	nodeID      string
	presenceTTL time.Duration
}

// NewManager creates a new connection manager without Redis or node identity.
// Use NewManagerWithPresence for production setups.
func NewManager() *Manager {
	return &Manager{
		connections: make(map[Identity]*Connection),
	}
}

// NewManagerWithPresence creates a manager that synchronises Redis presence sets.
func NewManagerWithPresence(redisClient *redis.Client, nodeID string, ttlSeconds int) *Manager {
	return &Manager{
		connections: make(map[Identity]*Connection),
		redisClient: redisClient,
		nodeID:      nodeID,
		presenceTTL: time.Duration(ttlSeconds) * time.Second,
	}
}

// userGatewayKey returns aim:user_gateway:{user_id}.
func userGatewayKey(userID int64) string {
	return fmt.Sprintf("aim:user_gateway:%d", userID)
}

// userPresenceKey returns aim:presence:{user_id}.
func userPresenceKey(userID int64) string {
	return fmt.Sprintf("aim:presence:%d", userID)
}

// Register adds a new connection for the given identity.
// When Redis is configured it maintains aim:user_gateway and aim:presence Sets.
func (m *Manager) Register(ctx context.Context, identity Identity, conn *websocket.Conn, cancel context.CancelFunc) (*PresenceResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.connections[identity]; exists {
		return nil, fmt.Errorf("connection already exists for user_id=%d device_id=%s", identity.UserID, identity.DeviceID)
	}

	m.connections[identity] = &Connection{
		Identity: identity,
		Cancel:   cancel,
		Conn:     conn,
	}

	logx.WithContext(ctx).Infof("ws connection registered: user_id=%d device_id=%s total=%d",
		identity.UserID, identity.DeviceID, len(m.connections))

	return m.updatePresenceOnRegister(ctx, identity.UserID, identity.DeviceID), nil
}

// updatePresenceOnRegister adds device to the presence Set and node to the gateway Set.
func (m *Manager) updatePresenceOnRegister(ctx context.Context, userID int64, deviceID string) *PresenceResult {
	if m.redisClient == nil {
		return &PresenceResult{}
	}

	gwKey := userGatewayKey(userID)
	presKey := userPresenceKey(userID)

	// Check SCARD before adding to detect 0→1 transition.
	wasOnline, err := m.redisClient.SCard(ctx, presKey).Result()
	if err != nil {
		logx.WithContext(ctx).Errorf("presence SCARD failed for user %d: %v", userID, err)
		return &PresenceResult{}
	}

	pipe := m.redisClient.Pipeline()
	pipe.SAdd(ctx, gwKey, m.nodeID)
	pipe.Expire(ctx, gwKey, m.presenceTTL)
	pipe.SAdd(ctx, presKey, deviceID)
	pipe.Expire(ctx, presKey, m.presenceTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		logx.WithContext(ctx).Errorf("presence Redis pipeline failed on register for user %d: %v", userID, err)
		return &PresenceResult{}
	}

	if wasOnline == 0 {
		logx.WithContext(ctx).Infof("user %d transitioned to online (device %s on node %s)", userID, deviceID, m.nodeID)
		return &PresenceResult{Switched: true, Status: "online"}
	}

	return &PresenceResult{}
}

// Unregister removes a connection by identity.
// When Redis is configured it cleans up the presence Sets.
func (m *Manager) Unregister(ctx context.Context, identity Identity) (*PresenceResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[identity]
	if !exists {
		return nil, fmt.Errorf("connection not found for user_id=%d device_id=%s", identity.UserID, identity.DeviceID)
	}

	delete(m.connections, identity)

	_ = conn.Cancel // cancel is called by handler on disconnect

	logx.WithContext(ctx).Infof("ws connection unregistered: user_id=%d device_id=%s total=%d",
		identity.UserID, identity.DeviceID, len(m.connections))

	return m.updatePresenceOnUnregister(ctx, identity.UserID, identity.DeviceID), nil
}

// updatePresenceOnUnregister removes device from presence Set.
// If no local connections remain, also removes node from gateway Set.
func (m *Manager) updatePresenceOnUnregister(ctx context.Context, userID int64, deviceID string) *PresenceResult {
	if m.redisClient == nil {
		return &PresenceResult{}
	}

	gwKey := userGatewayKey(userID)
	presKey := userPresenceKey(userID)

	// Count remaining local connections for this user.
	localCount := 0
	for _, c := range m.connections {
		if c.Identity.UserID == userID {
			localCount++
		}
	}

	pipe := m.redisClient.Pipeline()
	pipe.SRem(ctx, presKey, deviceID)

	if localCount == 0 {
		pipe.SRem(ctx, gwKey, m.nodeID)
	}

	// Renew TTLs if user still has some connections.
	if localCount > 0 {
		pipe.Expire(ctx, presKey, m.presenceTTL)
		pipe.Expire(ctx, gwKey, m.presenceTTL)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		logx.WithContext(ctx).Errorf("presence Redis pipeline failed on unregister for user %d: %v", userID, err)
		return &PresenceResult{}
	}

	// Check if all devices are now gone → transition to offline.
	remaining, err := m.redisClient.SCard(ctx, presKey).Result()
	if err != nil {
		logx.WithContext(ctx).Errorf("presence SCARD failed on unregister for user %d: %v", userID, err)
		return &PresenceResult{}
	}

	if remaining == 0 {
		logx.WithContext(ctx).Infof("user %d transitioned to offline (device %s on node %s)", userID, deviceID, m.nodeID)
		return &PresenceResult{Switched: true, Status: "offline"}
	}

	return &PresenceResult{}
}

// RenewPresenceTTL refreshes the TTL on the presence and gateway Sets for a user.
func (m *Manager) RenewPresenceTTL(ctx context.Context, userID int64) {
	if m.redisClient == nil {
		return
	}

	pipe := m.redisClient.Pipeline()
	pipe.Expire(ctx, userGatewayKey(userID), m.presenceTTL)
	pipe.Expire(ctx, userPresenceKey(userID), m.presenceTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		logx.WithContext(ctx).Errorf("presence TTL renewal failed for user %d: %v", userID, err)
	}
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
