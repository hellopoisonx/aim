// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package friends

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListFriendApplicationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListFriendApplicationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFriendApplicationsLogic {
	return &ListFriendApplicationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListFriendApplicationsLogic) ListFriendApplications() (*types.ListFriendApplicationsResponse, error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.ListFriendApplications(l.ctx, &friendshipservice.ListFriendApplicationsReq{UserId: identity.UserID})
	if err != nil {
		return nil, l.sanitizeLogicRPCError("list friend applications", err)
	}

	applications := make([]types.FriendshipItem, 0, len(rpcResp.GetApplications()))
	for _, app := range rpcResp.GetApplications() {
		applications = append(applications, types.FriendshipItem{
			UserId:    app.GetUserId(),
			FriendId:  app.GetFriendId(),
			Status:    app.GetStatus(),
			CreatedAt: app.GetCreatedAt(),
			UpdatedAt: app.GetUpdatedAt(),
		})
	}

	return &types.ListFriendApplicationsResponse{Applications: applications}, nil
}

func (l *ListFriendApplicationsLogic) sanitizeLogicRPCError(operation string, err error) error {
	if codeErr := errorx.FromGRPCError(err); codeErr != nil {
		return codeErr
	}

	l.Errorf("logic rpc %s failed: %v", operation, err)

	return errorx.NewCodeError(errorx.CodeInternal, "internal error")
}
