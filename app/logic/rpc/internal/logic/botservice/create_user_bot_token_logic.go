package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateUserBotTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserBotTokenLogic {
	return &CreateUserBotTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateUserBotTokenLogic) CreateUserBotToken(in *pb.CreateUserBotTokenReq) (*pb.CreateUserBotTokenResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	info, plaintext, err := l.svcCtx.BotService.CreateUserBotToken(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(),
		in.GetName(), in.GetExpiresAt(), in.GetActions())
	if err != nil {
		return nil, err
	}

	return &pb.CreateUserBotTokenResp{Token: userBotTokenToProto(info), PlaintextToken: plaintext}, nil
}
