package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	info, plaintext, err := botSvc.RotateUserBotToken(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetTokenId())
	if err != nil {
		return nil, err
	}

	return &pb.RotateUserBotTokenResp{Token: userBotTokenToProto(info), PlaintextToken: plaintext}, nil
}
