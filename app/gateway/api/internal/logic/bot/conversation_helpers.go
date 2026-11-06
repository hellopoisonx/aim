package bot

import (
	"context"
	"strconv"
	"strings"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/fanout"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/membership"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

func parseOptionalID(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errorx.NewCodeError(errorx.CodeBadInput, field+" must be a positive decimal string")
	}

	return id, nil
}

// logicConversationClient returns the client injected at startup; the gateway
// fails fast when it cannot be built, so no nil guard is needed here.
func logicConversationClient(svcCtx *svc.ServiceContext) conversationservice.ConversationService {
	return svcCtx.LogicConversationClient
}

func ensureBotConversationMember(
	ctx context.Context,
	client conversationservice.ConversationService,
	identity botctx.Identity,
	conversationID int64,
) error {
	return membership.RequireMember(ctx, noLogLogger{}, client, conversationID, identity.BotUserID, membership.ActorBot)
}

// botConversationRequest authorises the bot for action, parses the target
// conversation id and checks the bot is a member of it.
func botConversationRequest(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	action string,
	rawConversationID string,
) (int64, conversationservice.ConversationService, error) {
	identity, err := requireBotAction(ctx, action)
	if err != nil {
		return 0, nil, err
	}

	conversationID, err := parseRequiredID(rawConversationID, "id")
	if err != nil {
		return 0, nil, err
	}

	client := logicConversationClient(svcCtx)
	if err := ensureBotConversationMember(ctx, client, identity, conversationID); err != nil {
		return 0, nil, err
	}

	return conversationID, client, nil
}

// noLogLogger satisfies errorx.ErrorLogger for call paths that own logging.
type noLogLogger struct{}

func (noLogLogger) Errorf(string, ...any) {}

func publishBotReadReceipt(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	identity botctx.Identity,
	conversationID int64,
	readState *pb.ReadStateItem,
) {
	if svcCtx == nil || readState == nil {
		return
	}

	if svcCtx.ReadReceiptPub != nil {
		if err := svcCtx.ReadReceiptPub.PublishReadReceipt(
			ctx,
			identity.BotUserID,
			conversationID,
			readState.GetLastReadMessageId(),
			readState.GetUpdatedAt(),
		); err != nil {
			logx.WithContext(ctx).Errorf("bot read receipt publish failed: %v", err)
		}
	}

	fanout.ReadReceipt(ctx, svcCtx, identity.BotUserID, conversationID, readState.GetLastReadMessageId(), readState.GetUpdatedAt())
}
