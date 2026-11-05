package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateUserBotTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserBotTokenLogic {
	return &UpdateUserBotTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UpdateUserBotTokenLogic) UpdateUserBotToken(in *pb.UpdateUserBotTokenReq) (*pb.UpdateUserBotTokenResp, error) {
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	info, err := botSvc.UpdateUserBotToken(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetTokenId(),
		in.GetName(), deadlineFromMillis(in.GetExpiresAt()), in.GetActions())
	if err != nil {
		return nil, err
	}

	return &pb.UpdateUserBotTokenResp{Token: userBotTokenToProto(info)}, nil
}
