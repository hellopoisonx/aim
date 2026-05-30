package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateUserBotLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateUserBotLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserBotLogic {
	return &CreateUserBotLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateUserBotLogic) CreateUserBot(in *pb.CreateUserBotReq) (*pb.CreateUserBotResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	bot, err := l.svcCtx.BotService.CreateUserBot(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(),
		in.GetEmail(), in.GetNickname(), in.GetAvatar())
	if err != nil {
		return nil, err
	}

	return &pb.CreateUserBotResp{Bot: userBotToProto(bot)}, nil
}
