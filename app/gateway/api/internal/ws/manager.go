// Package ws provides WebSocket connection management for the AIM gateway.
package ws

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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
	Identity Identity
	Cancel   context.CancelFunc
	Conn     *websocket.Conn

	serverSeq      atomic.Int64
	writeCh        chan writeRequest
	writerStop     chan struct{}
	writerDone     chan struct{}
	writerStopOnce sync.Once
	replay         *ReplayStore // 可能为 nil（未注册连接）

	ExpiresAt    int64       // Unix timestamp when token expires (in seconds), 0 if no expiry
	ExpiryTimer  *time.Timer // timer that fires at token expiry, nil if not set
	LastSeen     int64       // Unix timestamp (milliseconds) of last heartbeat or frame received
	LastAckedSeq int64       // highest server-emitted seq the client has acknowledged
	TokenExpired bool        // true if this connection was closed due to token expiration (grace active)
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
	mu               sync.RWMutex
	connections      map[Identity]*Connection
	redisClient      *redis.Client
	nodeID           string
	presenceTTL      time.Duration
	heartbeatTimeout time.Duration // max time without heartbeat before connection is considered stale
	reconnectGrace   time.Duration // grace period after token-expired close for reconnection

	pendingMu      sync.Mutex
	pendingOffline map[Identity]time.Time // identity → deadline for grace

	replay *ReplayStore // pending/replay 队列，服务侧面向心跳重放用
}

// NewManager creates a new connection manager without Redis or node identity.
// Use NewManagerWithPresence for production setups.
func NewManager() *Manager {
	m := &Manager{
		connections:    make(map[Identity]*Connection),
		pendingOffline: make(map[Identity]time.Time),
	}
	m.replay = mustNewReplayStore()
	return m
}

// NewManagerWithPresence creates a manager that synchronises Redis presence sets.
func NewManagerWithPresence(redisClient *redis.Client, nodeID string, ttlSeconds int) *Manager {
	m := &Manager{
		connections:      make(map[Identity]*Connection),
		redisClient:      redisClient,
		nodeID:           nodeID,
		presenceTTL:      time.Duration(ttlSeconds) * time.Second,
		heartbeatTimeout: 60 * time.Second, // default: 60s heartbeat timeout
		reconnectGrace:   30 * time.Second, // default: 30s token-refresh grace
		pendingOffline:   make(map[Identity]time.Time),
	}
	m.replay = mustNewReplayStore()
	return m
}

// mustNewReplayStore 创建默认参数的 ReplayStore；构建失败在当前
// 代码路径上不可能发生（依赖 go-zero collection.NewCache），如果
// 出现反而意味着 fail-fast。
func mustNewReplayStore() *ReplayStore {
	store, err := NewReplayStore()
	if err != nil {
		panic(fmt.Sprintf("ws: init replay store: %v", err))
	}
	return store
}

// ReplayStore returns the underlying pending/replay store.
func (m *Manager) ReplayStore() *ReplayStore {
	return m.replay
}

// SetHeartbeatTimeout configures the heartbeat timeout for stale connection detection.
func (m *Manager) SetHeartbeatTimeout(d time.Duration) {
	m.heartbeatTimeout = d
}

// SetReconnectGrace configures the grace period for token-expired reconnections.
func (m *Manager) SetReconnectGrace(d time.Duration) {
	m.reconnectGrace = d
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
// If the same user/device had a pending offline from token expiration, it is cancelled.
func (m *Manager) Register(ctx context.Context, identity Identity, conn *websocket.Conn, cancel context.CancelFunc) (*PresenceResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.connections[identity]; exists {
		return nil, fmt.Errorf("connection already exists for user_id=%d device_id=%s", identity.UserID, identity.DeviceID)
	}

	now := time.Now().UnixMilli()
	connEntry := newConnection(identity, conn, cancel, now)
	connEntry.replay = m.replay
	m.connections[identity] = connEntry
	connEntry.startWriter(ctx)

	// Cancel any pending offline for this identity (token-refresh grace).
	cancelled := m.cancelPendingOfflineLocked(identity)

	logx.WithContext(ctx).Infof("ws connection registered: user_id=%d device_id=%s total=%d cancelled_pending_offline=%v",
		identity.UserID, identity.DeviceID, len(m.connections), cancelled)

	return m.updatePresenceOnRegister(ctx, identity.UserID, identity.DeviceID), nil
}

// cancelPendingOfflineLocked removes a pending offline entry and returns true if one existed.
// Must be called with pendingMu held (or from Register which holds mu).
func (m *Manager) cancelPendingOfflineLocked(identity Identity) bool {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()

	if _, exists := m.pendingOffline[identity]; exists {
		delete(m.pendingOffline, identity)
		return true
	}
	return false
}

// CancelPendingOffline removes a pending offline entry (public variant for reaper/external use).
func (m *Manager) CancelPendingOffline(identity Identity) bool {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()

	if _, exists := m.pendingOffline[identity]; exists {
		delete(m.pendingOffline, identity)
		return true
	}
	return false
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

// MarkTokenExpired sets the token-expired flag on a connection before it is closed.
// This causes Unregister to register a reconnect grace instead of publishing offline.
func (m *Manager) MarkTokenExpired(identity Identity) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[identity]
	if !exists {
		return
	}
	conn.TokenExpired = true
}

// RecordClientAck advances LastAckedSeq for an identity to the highest seq the
// client has acknowledged. The seq counter is monotonic so older acks are
// dropped silently. Returns false when the identity has no active connection.
// 同时从 pending/replay 队列中清除 seq <= ackSeq 的帧，释放内存。
func (m *Manager) RecordClientAck(identity Identity, ackSeq int64) bool {
	m.mu.Lock()
	conn, exists := m.connections[identity]
	if !exists {
		m.mu.Unlock()
		return false
	}

	if ackSeq > conn.LastAckedSeq {
		conn.LastAckedSeq = ackSeq
	}
	m.mu.Unlock()

	if m.replay != nil {
		m.replay.Ack(identity, ackSeq)
	}
	return true
}

// RecordHeartbeat updates the LastSeen time for a connection and renews presence TTL.
func (m *Manager) RecordHeartbeat(ctx context.Context, identity Identity) {
	m.mu.RLock()
	conn, exists := m.connections[identity]
	m.mu.RUnlock()

	if !exists {
		return
	}

	conn.LastSeen = time.Now().UnixMilli()
	m.RenewPresenceTTL(ctx, identity.UserID)
}

// Unregister removes a connection by identity.
// When Redis is configured it cleans up the presence Sets.
// If the connection was closed due to token expiry, a reconnect grace is registered
// and no offline event is published. The offline will be published only if the
// grace expires without reconnection.
func (m *Manager) Unregister(ctx context.Context, identity Identity) (*PresenceResult, error) {
	m.mu.Lock()

	conn, exists := m.connections[identity]
	if !exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("connection not found for user_id=%d device_id=%s", identity.UserID, identity.DeviceID)
	}

	tokenExpired := conn.TokenExpired
	delete(m.connections, identity)
	total := len(m.connections)
	_ = conn.Cancel // cancel is called by handler on disconnect

	m.mu.Unlock()
	conn.stopWriter()
	if m.replay != nil {
		m.replay.Delete(identity)
	}

	if tokenExpired {
		// Register reconnect grace instead of publishing offline.
		m.registerGraceDeadline(identity)
		// Renew presence TTL so the user stays "online" during the grace period.
		m.RenewPresenceTTL(ctx, identity.UserID)

		logx.WithContext(ctx).Infof("ws connection unregistered (token expired, grace=%v): user_id=%d device_id=%s total=%d",
			m.reconnectGrace, identity.UserID, identity.DeviceID, total)
		return &PresenceResult{}, nil
	}

	logx.WithContext(ctx).Infof("ws connection unregistered: user_id=%d device_id=%s total=%d",
		identity.UserID, identity.DeviceID, total)

	return m.updatePresenceOnUnregister(ctx, identity.UserID, identity.DeviceID), nil
}

// registerGraceDeadline stores a grace deadline for the given identity.
func (m *Manager) registerGraceDeadline(identity Identity) {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()

	m.pendingOffline[identity] = time.Now().Add(m.reconnectGrace)
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
	m.mu.RLock()
	localCount := 0
	for _, c := range m.connections {
		if c.Identity.UserID == userID {
			localCount++
		}
	}
	m.mu.RUnlock()

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

// ScanStaleConnections returns identities whose LastSeen is older than the given threshold.
// Returns nil if heartbeatTimeout is not configured.
func (m *Manager) ScanStaleConnections(now int64) []Identity {
	if m.heartbeatTimeout <= 0 {
		return nil
	}

	threshold := now - m.heartbeatTimeout.Milliseconds()

	m.mu.RLock()
	defer m.mu.RUnlock()

	var stale []Identity
	for identity, conn := range m.connections {
		if conn.LastSeen < threshold {
			stale = append(stale, identity)
		}
	}
	return stale
}

// ScanExpiredGraces returns identities whose reconnect grace has expired.
// Each entry is paired with whether the identity has any remaining active connections.
func (m *Manager) ScanExpiredGraces(now time.Time) []Identity {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()

	var expired []Identity
	for identity, deadline := range m.pendingOffline {
		if now.After(deadline) {
			expired = append(expired, identity)
		}
	}
	return expired
}

// RemoveExpiredGrace removes a grace entry after it has been processed (offline published).
func (m *Manager) RemoveExpiredGrace(identity Identity) {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	delete(m.pendingOffline, identity)
}

// HasPendingGrace returns whether an identity has an active reconnect grace.
func (m *Manager) HasPendingGrace(identity Identity) bool {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	_, exists := m.pendingOffline[identity]
	return exists
}

// ForceOfflineForExpiredGrace publishes an offline event for a user/device whose grace expired.
// It removes the device from the presence set and checks if user went offline.
func (m *Manager) ForceOfflineForExpiredGrace(ctx context.Context, identity Identity) (*PresenceResult, error) {
	if m.redisClient == nil {
		return &PresenceResult{}, nil
	}

	presKey := userPresenceKey(identity.UserID)
	gwKey := userGatewayKey(identity.UserID)

	// Remove this device from presence set.
	pipe := m.redisClient.Pipeline()
	pipe.SRem(ctx, presKey, identity.DeviceID)

	// Check if there are any remaining local connections for this user.
	m.mu.RLock()
	localCount := 0
	for _, c := range m.connections {
		if c.Identity.UserID == identity.UserID {
			localCount++
		}
	}
	m.mu.RUnlock()

	// If no local connections, remove node from gateway set.
	if localCount == 0 {
		pipe.SRem(ctx, gwKey, m.nodeID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		logx.WithContext(ctx).Errorf("force offline Redis pipeline failed for user %d: %v", identity.UserID, err)
		return &PresenceResult{}, nil
	}

	// Check if all devices are gone → offline.
	remaining, err := m.redisClient.SCard(ctx, presKey).Result()
	if err != nil {
		logx.WithContext(ctx).Errorf("force offline SCARD failed for user %d: %v", identity.UserID, err)
		return &PresenceResult{}, nil
	}

	if remaining == 0 {
		logx.WithContext(ctx).Infof("user %d transitioned to offline (grace expired, device %s on node %s)",
			identity.UserID, identity.DeviceID, m.nodeID)
		return &PresenceResult{Switched: true, Status: "offline"}, nil
	}

	logx.WithContext(ctx).Infof("removed device %s for user %d (grace expired), remaining devices=%d",
		identity.DeviceID, identity.UserID, remaining)
	return &PresenceResult{}, nil
}

// CloseAll closes all connections and clears the manager.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	connections := make([]*Connection, 0, len(m.connections))
	for identity, conn := range m.connections {
		connections = append(connections, conn)
		delete(m.connections, identity)
	}
	m.mu.Unlock()

	for _, conn := range connections {
		if conn.Cancel != nil {
			conn.Cancel()
		}

		conn.stopWriter()
	}
}
