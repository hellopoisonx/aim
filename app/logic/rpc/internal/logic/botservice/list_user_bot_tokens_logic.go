package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListUserBotTokensLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListUserBotTokensLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListUserBotTokensLogic {
	return &ListUserBotTokensLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListUserBotTokensLogic) ListUserBotTokens(in *pb.ListUserBotTokensReq) (*pb.ListUserBotTokensResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	tokens, err := l.svcCtx.BotService.ListUserBotTokens(l.ctx,
		in.GetOwnerUserId(), in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	resp := make([]*pb.UserBotTokenInfo, 0, len(tokens))
	for _, t := range tokens {
		resp = append(resp, userBotTokenToProto(t))
	}

	return &pb.ListUserBotTokensResp{Tokens: resp}, nil
}
