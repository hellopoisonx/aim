package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/bottoken"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotSetWebhookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotSetWebhookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotSetWebhookLogic {
	return &BotSetWebhookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotSetWebhookLogic) BotSetWebhook(req *types.BotSetWebhookRequest) (*types.BotSetWebhookResponse, error) {
	identity, ok := botctx.FromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "missing bot identity")
	}

	if l.svcCtx.LogicBotClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	if req.Url == "" {
		return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "url is required")
	}

	if req.Secret != "" && req.RotateSecret {
		return nil, errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "specify either secret or rotate_secret, not both")
	}

	plainSecret := req.Secret
	if req.RotateSecret {
		generated, err := bottoken.GenerateWebhookSecret()
		if err != nil {
			return nil, errorx.NewCodeError(errorx.CodeInternal, "failed to generate webhook secret")
		}
		plainSecret = generated
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rpcResp, err := l.svcCtx.LogicBotClient.SetBotWebhook(l.ctx, &botservice.SetBotWebhookReq{
		BotUserId: identity.BotUserID,
		Url:       req.Url,
		Secret:    plainSecret,
		Events:    req.Events,
		Enabled:   enabled,
	})
	if err != nil {
		if ce := errorx.FromGRPCError(err); ce != nil {
			return nil, ce
		}
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	cfg := rpcResp.GetWebhook()
	if cfg == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	resp := &types.BotSetWebhookResponse{
		Webhook: types.BotWebhookConfig{
			Url:       cfg.GetUrl(),
			Events:    cfg.GetEvents(),
			Enabled:   cfg.GetEnabled(),
			UpdatedAt: cfg.GetUpdatedAt(),
		},
	}

	// Only echo the plaintext when the caller asked for rotation; if the
	// caller supplied their own Secret, they already know it.
	if req.RotateSecret {
		resp.PlaintextSecret = plainSecret
	}

	return resp, nil
}
