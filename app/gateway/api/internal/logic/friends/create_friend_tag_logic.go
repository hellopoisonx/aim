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

// sanitizeLogicRPCError wraps RPC errors for the gateway.
func sanitizeLogicRPCError(logger logicRPCErrorLogger, operation string, err error) error {
	if codeErr := errorx.FromGRPCError(err); codeErr != nil {
		return codeErr
	}
	logger.Errorf("logic rpc %s failed: %v", operation, err)
	return errorx.NewCodeError(errorx.CodeInternal, "internal error")
}

// logicRPCErrorLogger is the logging interface used by the error helper.
type logicRPCErrorLogger interface {
	Errorf(format string, v ...any)
}

// friendTagToType converts a proto FriendTagResponse to a gateway type.
func friendTagToType(t interface {
	GetId() int64
	GetUserId() int64
	GetName() string
	GetCreatedAt() int64
	GetUpdatedAt() int64
}) types.FriendTagItem {
	if t == nil {
		return types.FriendTagItem{}
	}
	return types.FriendTagItem{
		Id:        t.GetId(),
		UserId:    t.GetUserId(),
		Name:      t.GetName(),
		CreatedAt: t.GetCreatedAt(),
		UpdatedAt: t.GetUpdatedAt(),
	}
}

type CreateFriendTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateFriendTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateFriendTagLogic {
	return &CreateFriendTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateFriendTagLogic) CreateFriendTag(req *types.CreateFriendTagRequest) (resp *types.CreateFriendTagResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}
	if l.svcCtx.LogicFriendshipClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicFriendshipClient.CreateFriendTag(l.ctx, &friendshipservice.CreateFriendTagReq{
		UserId: identity.UserID,
		Name:   req.Name,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "create friend tag", err)
	}

	return &types.CreateFriendTagResponse{
		Tag: friendTagToType(rpcResp.GetTag()),
	}, nil
}
