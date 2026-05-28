package bot

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/botctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	gatewayws "github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"

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

func sanitizeBotLogicRPCError(err error) error {
	var ce *errorx.CodeError
	if errors.As(err, &ce) {
		return ce
	}
	if ce := errorx.FromGRPCError(err); ce != nil {
		return ce
	}
	return errorx.NewCodeError(errorx.CodeInternal, "internal error")
}

func requireLogicConversationClient(svcCtx *svc.ServiceContext) (conversationservice.ConversationService, error) {
	if svcCtx == nil || svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}
	return svcCtx.LogicConversationClient, nil
}

func ensureBotConversationMember(
	ctx context.Context,
	client conversationservice.ConversationService,
	identity botctx.BotIdentity,
	conversationID int64,
) error {
	membersResp, err := client.GetConversationMembers(ctx, &conversationservice.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	if err != nil {
		return sanitizeBotLogicRPCError(err)
	}

	if !slices.Contains(membersResp.GetMemberIds(), identity.BotUserID) {
		return errorx.NewCodeError(errorx.CodeForbidden, "bot is not a member of this conversation")
	}

	return nil
}

func convertBotReadState(st *pb.ReadStateItem) types.BotReadStateItem {
	return types.BotReadStateItem{
		UserId:            formatID(st.GetUserId()),
		LastReadMessageId: formatID(st.GetLastReadMessageId()),
		UpdatedAt:         st.GetUpdatedAt(),
		Email:             st.GetEmail(),
		Avatar:            st.GetAvatar(),
		Name:             st.GetName(),
	}
}

func publishBotReadReceipt(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	identity botctx.BotIdentity,
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

	fanOutBotReadReceiptLocal(ctx, svcCtx, identity.BotUserID, conversationID, readState)
}

func fanOutBotReadReceiptLocal(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	fromUserID int64,
	conversationID int64,
	readState *pb.ReadStateItem,
) {
	if svcCtx == nil || svcCtx.LogicConversationClient == nil || svcCtx.WsManager == nil || readState == nil {
		return
	}

	membersResp, err := svcCtx.LogicConversationClient.GetConversationMembers(ctx, &conversationservice.GetConversationMembersReq{
		ConversationId: conversationID,
	})
	if err != nil {
		logx.WithContext(ctx).Debugf("bot read receipt local fan-out skipped for conv %d: %v", conversationID, err)
		return
	}

	gateway := gatewayws.NewGatewayServer(svcCtx.WsManager)
	for _, memberID := range membersResp.GetMemberIds() {
		if memberID <= 0 || memberID == fromUserID || len(svcCtx.WsManager.GetByUserID(memberID)) == 0 {
			continue
		}

		_, _ = gateway.PushReadReceipt(ctx, &gwpb.PushReadReceiptReq{
			TargetUserId:      memberID,
			ConversationId:    conversationID,
			FromUserId:        fromUserID,
			LastReadMessageId: readState.GetLastReadMessageId(),
			UpdatedAt:         readState.GetUpdatedAt(),
		})
	}
}
