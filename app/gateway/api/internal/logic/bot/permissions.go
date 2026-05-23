package bot

import (
	"context"
	"errors"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/botservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"
)

func requireBotAction(ctx context.Context, action string) (botctx.BotIdentity, error) {
	identity, ok := botctx.FromContext(ctx)
	if !ok {
		return botctx.BotIdentity{}, errorx.NewCodeError(errorx.CodeBotTokenInvalid, "missing bot identity")
	}

	if err := identity.RequireAction(action); err != nil {
		return botctx.BotIdentity{}, err
	}

	return identity, nil
}

func requireWebhookEventActions(ctx context.Context, svcCtx *svc.ServiceContext, identity botctx.BotIdentity, events []string) error {
	if svcCtx.LogicBotClient == nil {
		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	resp, err := svcCtx.LogicBotClient.ResolveBotWebhookEventActions(ctx, &botservice.ResolveBotWebhookEventActionsReq{Events: events})
	if err != nil {
		var ce *errorx.CodeError
		if errors.As(err, &ce) {
			return ce
		}

		if ce := errorx.FromGRPCError(err); ce != nil {
			return ce
		}

		return errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	for _, ea := range resp.GetEventActions() {
		if ea.GetEvent() == "" || ea.GetAction() == "" {
			return errorx.NewCodeError(errorx.CodeBotWebhookInvalid, "invalid webhook event action")
		}

		if err := identity.RequireAction(ea.GetAction()); err != nil {
			return err
		}
	}

	return nil
}
