package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotDeleteWebhookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotDeleteWebhookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotDeleteWebhookLogic {
	return &BotDeleteWebhookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotDeleteWebhookLogic) BotDeleteWebhook() (*types.BotDeleteWebhookResponse, error) {
	identity, ok := botctx.FromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "missing bot identity")
	}

	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicBotClient.DeleteBotWebhook(l.ctx, &botservice.DeleteBotWebhookReq{
		BotUserId: identity.BotUserID,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	return &types.BotDeleteWebhookResponse{Deleted: rpcResp.GetDeleted()}, nil
}
