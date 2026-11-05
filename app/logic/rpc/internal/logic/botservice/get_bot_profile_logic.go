package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetBotProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetBotProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetBotProfileLogic {
	return &GetBotProfileLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetBotProfile returns the bot's user_info snapshot for `/api/bot/v1/me`.
func (l *GetBotProfileLogic) GetBotProfile(in *pb.GetBotProfileReq) (*pb.GetBotProfileResp, error) {
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	info, err := botSvc.GetBotProfile(l.ctx, in.GetBotUserId())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	return &pb.GetBotProfileResp{
		Profile: pbconv.User(info),
	}, nil
}
