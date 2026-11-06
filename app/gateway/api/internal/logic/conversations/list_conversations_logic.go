package conversations

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
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
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.GetUserConversations(l.ctx, &conversationservice.GetUserConversationsReq{
		UserId: userID,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "get user conversations", err)
	}

	convos := rpcResp.GetConversations()
	items := make([]types.ConversationItem, 0, len(convos))
	for _, conv := range convos {
		items = append(items, pbconv.Conversation(conv))
	}

	return &types.ListConversationsResponse{
		Conversations: items,
	}, nil
}
