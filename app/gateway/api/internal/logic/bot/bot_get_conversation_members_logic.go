package bot

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/botperm"

	"github.com/zeromicro/go-zero/core/logx"
)

type BotGetConversationMembersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBotGetConversationMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BotGetConversationMembersLogic {
	return &BotGetConversationMembersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BotGetConversationMembersLogic) BotGetConversationMembers(req *types.BotGetConversationMembersRequest) (*types.BotGetConversationMembersResponse, error) {
	identity, err := requireBotAction(l.ctx, botperm.ActionConversationMembersRead)
	if err != nil {
		return nil, err
	}

	conversationID, err := parseRequiredID(req.Id, "id")
	if err != nil {
		return nil, err
	}

	client, err := requireLogicConversationClient(l.svcCtx)
	if err != nil {
		return nil, err
	}

	if err := ensureBotConversationMember(l.ctx, client, identity, conversationID); err != nil {
		return nil, err
	}

	rpcResp, err := client.GetConversationMembersDetail(l.ctx, &conversationservice.GetConversationMembersDetailReq{
		ConversationId: conversationID,
	})
	if err != nil {
		return nil, sanitizeBotLogicRPCError(err)
	}

	members := rpcResp.GetMembers()
	items := make([]types.BotMemberDetailItem, 0, len(members))
	for _, m := range members {
		items = append(items, types.BotMemberDetailItem{
			UserId:      formatID(m.GetUserId()),
			Email:       m.GetEmail(),
			Avatar:      m.GetAvatar(),
			Role:        m.GetRole(),
			JoinedAt:    m.GetJoinedAt(),
			DisplayName: m.GetDisplayName(),
		})
	}

	return &types.BotGetConversationMembersResponse{Members: items}, nil
}
