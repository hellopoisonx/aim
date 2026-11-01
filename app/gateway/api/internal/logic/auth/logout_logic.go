// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package auth

import (
	"context"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type LogoutLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLogoutLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LogoutLogic {
	return &LogoutLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LogoutLogic) Logout() (resp *types.LogoutResponse, err error) {
	identity, ok := authctx.IdentityFromContext(l.ctx)
	if !ok || identity.UserID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "missing authorization")
	}

	rpcResp, err := l.svcCtx.AuthClient.Logout(l.ctx, &authservice.LogoutReq{UserId: identity.UserID, DeviceId: identity.DeviceID})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "logout", err)
	}

	// Best-effort: close the WS connection for this device so the kicked client
	// observes the disconnect immediately rather than waiting for token expiry.
	if l.svcCtx.WsManager != nil {
		kickReq := &gwpb.KickUserReq{
			UserId:   identity.UserID,
			DeviceId: identity.DeviceID,
			Reason:   "logout",
		}
		if _, kickErr := l.svcCtx.Gateway().KickUser(l.ctx, kickReq); kickErr != nil {
			l.Errorf("kick on logout failed: user_id=%d device_id=%s err=%v", identity.UserID, identity.DeviceID, kickErr)
		}
	}

	return &types.LogoutResponse{Success: rpcResp.Success}, nil
}
