package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	deleted, err := botSvc.DeleteUserBot(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	return &pb.DeleteUserBotResp{Deleted: deleted}, nil
}
