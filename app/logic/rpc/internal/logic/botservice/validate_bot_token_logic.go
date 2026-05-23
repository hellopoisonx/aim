package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ValidateBotTokenLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewValidateBotTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ValidateBotTokenLogic {
	return &ValidateBotTokenLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ValidateBotToken resolves a plaintext token into a bot identity.
func (l *ValidateBotTokenLogic) ValidateBotToken(in *pb.ValidateBotTokenReq) (*pb.ValidateBotTokenResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	if in.GetPlaintextToken() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "token is required")
	}

	identity, err := l.svcCtx.BotService.ValidateBotToken(l.ctx, in.GetPlaintextToken())
	if err != nil {
		return nil, err
	}

	return &pb.ValidateBotTokenResp{
		Identity: &pb.BotIdentity{
			BotUserId:  identity.BotUserID,
			TokenId:    identity.TokenID,
			Scopes:     identity.Scopes,
			Nickname:   identity.Nickname,
			Avatar:     identity.Avatar,
			UserStatus: int32(identity.UserStatus),
		},
	}, nil
}
