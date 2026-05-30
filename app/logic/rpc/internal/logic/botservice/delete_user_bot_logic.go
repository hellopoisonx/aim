package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteUserBotLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteUserBotLogic {
	return &DeleteUserBotLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *DeleteUserBotLogic) DeleteUserBot(in *pb.DeleteUserBotReq) (*pb.DeleteUserBotResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	deleted, err := l.svcCtx.BotService.DeleteUserBot(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	return &pb.DeleteUserBotResp{Deleted: deleted}, nil
}
