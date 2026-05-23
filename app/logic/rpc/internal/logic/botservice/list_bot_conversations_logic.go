package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListBotConversationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListBotConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBotConversationsLogic {
	return &ListBotConversationsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ListBotConversations returns every conversation the bot is a member of.
func (l *ListBotConversationsLogic) ListBotConversations(in *pb.ListBotConversationsReq) (*pb.ListBotConversationsResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	if in.GetBotUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "bot_user_id is required")
	}

	items, err := l.svcCtx.BotService.ListBotConversations(l.ctx, in.GetBotUserId())
	if err != nil {
		return nil, errorx.NewCodeErrorf(errorx.CodeInternal, "list bot conversations failed: %v", err)
	}

	out := make([]*pb.BotConversationItem, 0, len(items))
	for _, c := range items {
		out = append(out, &pb.BotConversationItem{
			ConversationId:   c.ConversationID,
			ConversationType: c.ConversationType,
			Name:             c.Name,
			Avatar:           c.Avatar,
			CreatedAt:        c.CreatedAt.UnixMilli(),
		})
	}

	return &pb.ListBotConversationsResp{Conversations: out}, nil
}
