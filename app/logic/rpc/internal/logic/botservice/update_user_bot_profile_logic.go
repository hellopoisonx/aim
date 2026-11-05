package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserBotProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateUserBotProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserBotProfileLogic {
	return &UpdateUserBotProfileLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UpdateUserBotProfileLogic) UpdateUserBotProfile(in *pb.UpdateUserBotProfileReq) (*pb.UpdateUserBotProfileResp, error) {
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	bot, err := botSvc.UpdateUserBotProfile(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(),
		in.GetNickname(), in.GetAvatar())
	if err != nil {
		return nil, err
	}

	return &pb.UpdateUserBotProfileResp{Bot: userBotToProto(bot)}, nil
}
