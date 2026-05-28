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

type GetConversationMembersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetConversationMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationMembersLogic {
	return &GetConversationMembersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetConversationMembersLogic) GetConversationMembers(req *types.GetConversationMembersDetailRequest) (resp *types.GetConversationMembersResponse, err error) {
	if _, ok := ws.IdentityFromContext(l.ctx); !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.GetConversationMembersDetail(l.ctx, &conversationservice.GetConversationMembersDetailReq{
		ConversationId: req.Id,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "get conversation members", err)
	}

	members := rpcResp.GetMembers()
	items := make([]types.MemberDetailItem, 0, len(members))
	for _, m := range members {
		items = append(items, types.MemberDetailItem{
			UserId:      m.GetUserId(),
			Email:       m.GetEmail(),
			Avatar:      m.GetAvatar(),
			Role:        m.GetRole(),
			JoinedAt:    m.GetJoinedAt(),
			Name:      m.GetName(),
		})
	}

	return &types.GetConversationMembersResponse{
		Members: items,
	}, nil
}

func (l *GetConversationMembersLogic) sanitizeLogicRPCError(operation string, err error) error {
	return sanitizeLogicRPCError(l, operation, err)
}
