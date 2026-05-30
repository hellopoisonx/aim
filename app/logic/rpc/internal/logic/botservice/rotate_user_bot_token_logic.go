package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RotateUserBotTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRotateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RotateUserBotTokenLogic {
	return &RotateUserBotTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RotateUserBotTokenLogic) RotateUserBotToken(in *pb.RotateUserBotTokenReq) (*pb.RotateUserBotTokenResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	info, plaintext, err := l.svcCtx.BotService.RotateUserBotToken(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetTokenId())
	if err != nil {
		return nil, err
	}

	return &pb.RotateUserBotTokenResp{Token: userBotTokenToProto(info), PlaintextToken: plaintext}, nil
}
