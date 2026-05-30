package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

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
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	revoked, err := l.svcCtx.BotService.RevokeUserBotToken(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId(), in.GetTokenId())
	if err != nil {
		return nil, err
	}

	return &pb.RevokeUserBotTokenResp{Revoked: revoked}, nil
}
