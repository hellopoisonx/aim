package conversations

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListConversationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListConversationsLogic {
	return &ListConversationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListConversationsLogic) ListConversations() (resp *types.ListConversationsResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.GetUserConversations(l.ctx, &conversationservice.GetUserConversationsReq{
		UserId: identity.UserID,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "get user conversations", err)
	}

	convos := rpcResp.GetConversations()
	items := make([]types.ConversationItem, 0, len(convos))
	for _, conv := range convos {
		memberIDs := conv.GetMemberIds()
		if memberIDs == nil {
			memberIDs = []int64{}
		}

		items = append(items, types.ConversationItem{
			ConversationId:   conv.GetId(),
			ConversationType: conv.GetConversationType(),
			IsActive:         conv.GetIsActive(),
			CreatedAt:        conv.GetCreatedAt(),
			MemberIds:        memberIDs,
			Name:             conv.GetName(),
			Avatar:           conv.GetAvatar(),
			CreatorId:        conv.GetCreatorId(),
		})
	}

	return &types.ListConversationsResponse{
		Conversations: items,
	}, nil
}

func (l *ListConversationsLogic) sanitizeLogicRPCError(operation string, err error) error {
	return sanitizeLogicRPCError(l, operation, err)
}
