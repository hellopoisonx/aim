// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package auth

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/auth/rpc/authservice"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	sharedjwt "github.com/hellopoisonx/aim/app/shared/jwt"
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
	token := bearerToken(l.ctx)
	if token == "" {
		return nil, errorx.NewCodeError(40100, "missing authorization")
	}

	claims, err := sharedjwt.NewManager(l.svcCtx.Config.Auth.AccessSecret).ValidateAccessToken(token)
	if err != nil {
		return nil, errorx.NewCodeError(40100, "invalid authorization")
	}

	rpcResp, err := l.svcCtx.AuthClient.Logout(l.ctx, &authservice.LogoutReq{UserId: claims.UserID, DeviceId: claims.DeviceID})
	if err != nil {
		return nil, l.sanitizeAuthRPCError("logout", err)
	}

	// Best-effort: close the WS connection for this device so the kicked client
	// observes the disconnect immediately rather than waiting for token expiry.
	if l.svcCtx.WsManager != nil {
		kickReq := &gwpb.KickUserReq{
			UserId:   claims.UserID,
			DeviceId: claims.DeviceID,
			Reason:   "logout",
		}
		if _, kickErr := ws.NewGatewayServer(l.svcCtx.WsManager).KickUser(l.ctx, kickReq); kickErr != nil {
			l.Errorf("kick on logout failed: user_id=%d device_id=%s err=%v", claims.UserID, claims.DeviceID, kickErr)
		}
	}

	return &types.LogoutResponse{Success: rpcResp.Success}, nil
}

func bearerToken(ctx context.Context) string {
	value := authctx.Authorization(ctx)

	token, ok := strings.CutPrefix(value, "Bearer ")
	if !ok {
		return ""
	}

	return strings.TrimSpace(token)
}
