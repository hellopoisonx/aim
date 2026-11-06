package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type RevokeUserBotTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRevokeUserBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeUserBotTokenLogic {
	return &RevokeUserBotTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RevokeUserBotTokenLogic) RevokeUserBotToken(in *pb.RevokeUserBotTokenReq) (*pb.RevokeUserBotTokenResp, error) {
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	revoked, err := botSvc.RevokeUserBotToken(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetTokenId())
	if err != nil {
		return nil, err
	}

	return &pb.RevokeUserBotTokenResp{Revoked: revoked}, nil
}
