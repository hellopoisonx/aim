// Package wshandler implements the gateway WebSocket endpoint: upgrade, the
// per-connection read loop and frame dispatch.
package wshandler

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/coder/websocket"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/httperr"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	wsauth "github.com/hellopoisonx/aim/app/gateway/api/internal/ws/auth"
	sharedtracing "github.com/hellopoisonx/aim/app/shared/tracing"
	"github.com/zeromicro/go-zero/core/logx"
)

// WsHandler handles WebSocket upgrade requests at GET /ws.
type WsHandler struct {
	srv      *svc.ServiceContext
	manager  *ws.Manager
	frameSeq atomic.Int64 // server-side sequence counter
}

// NewWsHandler wires the endpoint to the service context and connection manager.
func NewWsHandler(srv *svc.ServiceContext, manager *ws.Manager) *WsHandler {
	return &WsHandler{
		srv:     srv,
		manager: manager,
	}
}

// ServeWS upgrades the HTTP connection to WebSocket and handles the session.
func (h *WsHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	// 1. Extract and validate JWT
	authHeader := r.Header.Get("Authorization")

	claims, err := wsauth.ExtractAndValidate(authHeader, h.srv.Config.Auth.AccessSecret)
	if err != nil {
		// Write JSON error response before upgrade.
		httperr.Write(w, err)
		return
	}

	// 2. Upgrade to WebSocket
	_ = h.srv.Config.WebSocket
	ctx := r.Context()

	// Configure WebSocket options
	opts := websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
		OriginPatterns:  h.srv.Config.WebSocket.OriginPatterns,
	}

	conn, err := websocket.Accept(w, r, &opts)
	if err != nil {
		logx.WithContext(ctx).Errorf("ws upgrade failed: %v", err)
		return
	}

	conn.SetReadLimit(h.srv.Config.WebSocket.MaxMsgSize)

	// 3. Create per-connection context with cancellation.
	// Detach it from the short-lived HTTP upgrade span: the WebSocket connection
	// can stay open for a long time, while per-frame spans and downstream RPC/Kafka
	// spans complete quickly. Keeping the upgrade span as their parent can make
	// tracing backends see children whose parent span has not been exported yet.
	ctx, cancel := context.WithCancel(sharedtracing.DetachSpanContext(r.Context()))
	defer cancel()

	// 4. Register connection
	identity := authctx.Identity{
		UserID:   claims.UserID,
		DeviceID: claims.DeviceID,
	}

	// Inject identity into context for downstream handlers
	ctx = authctx.WithIdentity(ctx, identity)

	presResult, err := h.manager.Register(ctx, identity, conn, cancel)
	if err != nil {
		logx.WithContext(ctx).Errorf("ws register failed: %v", err)

		_ = conn.Close(websocket.StatusInternalError, "registration failed")

		return
	}

	// 5. Ensure unregister on disconnect (armed before any early return below)
	defer func() {
		unregResult, err := h.manager.Unregister(ctx, identity)
		if err != nil {
			logx.WithContext(ctx).Errorf("ws unregister failed: %v", err)
		}

		// Publish presence event if user just went offline.
		if unregResult != nil && unregResult.Switched && h.srv.PresencePub != nil {
			if err := h.srv.PresencePub.PublishPresence(ctx, identity.UserID, unregResult.Status); err != nil {
				logx.WithContext(ctx).Errorf("failed to publish presence event on unregister: %v", err)
			}
		}
	}()

	// 6. Publish presence event if user just came online.
	if presResult != nil && presResult.Switched && h.srv.PresencePub != nil {
		if err := h.srv.PresencePub.PublishPresence(ctx, identity.UserID, presResult.Status); err != nil {
			logx.WithContext(ctx).Errorf("failed to publish presence event on register: %v", err)
		}
	}

	// 7. Schedule token expiry timer
	tokenExpiresAt := claims.ExpiresAt.Time
	if tokenExpiresAt.After(time.Now()) {
		duration := time.Until(tokenExpiresAt)

		connEntry, err := h.manager.Get(identity)
		if err == nil {
			connEntry.ExpiresAt = tokenExpiresAt.Unix()
			connEntry.ExpiryTimer = time.AfterFunc(duration, func() {
				h.sendTokenExpired(ctx, conn, identity, tokenExpiresAt.Unix())
			})
		}
	} else {
		// Token already expired before connection - send expired frame and close
		h.sendTokenExpired(ctx, conn, identity, tokenExpiresAt.Unix())
		return
	}

	// 8. Read loop with config limits
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// Normal disconnect via context cancellation
				logx.WithContext(ctx).Infof("ws connection closed by client")
			} else {
				// Check for WebSocket close
				closeErr := &websocket.CloseError{}
				if errors.As(err, &closeErr) {
					logx.WithContext(ctx).Infof("ws close: code=%d reason=%s", closeErr.Code, closeErr.Reason)
				} else {
					logx.WithContext(ctx).Errorf("ws read error: %v", err)
				}
			}

			return
		}

		// Handle the received frame
		if err := h.handleFrame(ctx, conn, data); err != nil {
			logx.WithContext(ctx).Errorf("handle frame error: %v", err)
		}
	}
}
