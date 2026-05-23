package ws

import (
	"context"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// PresencePublishFn is a function that publishes a presence event (online/offline).
type PresencePublishFn func(ctx context.Context, userID int64, status string) error

// PresenceReaper periodically scans for stale connections (no heartbeat) and expired
// reconnect grace deadlines, then publishes offline events accordingly.
type PresenceReaper struct {
	manager      *Manager
	publishFn    PresencePublishFn
	tickInterval time.Duration
}

// NewPresenceReaper creates a new PresenceReaper.
func NewPresenceReaper(manager *Manager, publishFn PresencePublishFn) *PresenceReaper {
	return &PresenceReaper{
		manager:      manager,
		publishFn:    publishFn,
		tickInterval: 10 * time.Second, // scan every 10 seconds
	}
}

// SetTickInterval configures the scan interval.
func (r *PresenceReaper) SetTickInterval(d time.Duration) {
	r.tickInterval = d
}

// Run starts the reaper loop. It blocks until ctx is cancelled.
func (r *PresenceReaper) Run(ctx context.Context) {
	ticker := time.NewTicker(r.tickInterval)
	defer ticker.Stop()

	logx.WithContext(ctx).Infof("presence reaper started (interval=%v)", r.tickInterval)

	for {
		select {
		case <-ctx.Done():
			logx.WithContext(ctx).Info("presence reaper stopped")
			return
		case <-ticker.C:
			r.scan(ctx)
		}
	}
}

// scan performs one pass of stale connection and expired grace detection.
func (r *PresenceReaper) scan(ctx context.Context) {
	r.scanStaleConnections(ctx)
	r.scanExpiredGraces(ctx)
}

// scanStaleConnections checks for connections that haven't heartbeated recently
// and unregisters them, which triggers offline publication.
func (r *PresenceReaper) scanStaleConnections(ctx context.Context) {
	now := time.Now().UnixMilli()
	stale := r.manager.ScanStaleConnections(now)
	if len(stale) == 0 {
		return
	}

	for _, identity := range stale {
		// Only unregister if the connection is still present (it might have been
		// unregistered between scan and now).
		result, err := r.manager.Unregister(ctx, identity)
		if err != nil {
			logx.WithContext(ctx).Infof("reaper: unregister stale connection %d/%s: %v",
				identity.UserID, identity.DeviceID, err)
			continue
		}

		if result != nil && result.Switched && r.publishFn != nil {
			if err := r.publishFn(ctx, identity.UserID, result.Status); err != nil {
				logx.WithContext(ctx).Errorf("reaper: publish offline for stale %d/%s: %v",
					identity.UserID, identity.DeviceID, err)
			}
		}
	}
}

// scanExpiredGraces checks for reconnect grace deadlines that have passed
// without reconnection, and publishes offline events.
func (r *PresenceReaper) scanExpiredGraces(ctx context.Context) {
	now := time.Now()
	expired := r.manager.ScanExpiredGraces(now)
	if len(expired) == 0 {
		return
	}

	for _, identity := range expired {
		// Check if this identity has since re-registered (in which case the grace
		// was already cancelled by Register).
		if !r.manager.HasPendingGrace(identity) {
			continue
		}

		result, err := r.manager.ForceOfflineForExpiredGrace(ctx, identity)
		if err != nil {
			logx.WithContext(ctx).Errorf("reaper: force offline for expired grace %d/%s: %v",
				identity.UserID, identity.DeviceID, err)
			// Remove grace even on error to avoid infinite retry.
			r.manager.RemoveExpiredGrace(identity)
			continue
		}

		// Remove grace from tracking regardless of outcome.
		r.manager.RemoveExpiredGrace(identity)

		if result != nil && result.Switched && r.publishFn != nil {
			if err := r.publishFn(ctx, identity.UserID, result.Status); err != nil {
				logx.WithContext(ctx).Errorf("reaper: publish offline for expired grace %d/%s: %v",
					identity.UserID, identity.DeviceID, err)
			}
		}
	}
}
