// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

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

type CreateGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateGroupLogic {
	return &CreateGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateGroupLogic) CreateGroup(req *types.CreateGroupRequest) (resp *types.CreateConversationResponse, err error) {
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if len(req.MemberIds) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.CreateConversation(l.ctx, &conversationservice.CreateConversationReq{
		ConversationType: "group",
		CreatorId:        identity.UserID,
		MemberIds:        req.MemberIds,
		Name:             req.Name,
		Avatar:           req.Avatar,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "create group", err)
	}

	conv := rpcResp.GetConversation()
	if conv == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	memberIDs := conv.GetMemberIds()
	if memberIDs == nil {
		memberIDs = []int64{}
	}

	return &types.CreateConversationResponse{
		ConversationId:   conv.GetId(),
		ConversationType: conv.GetConversationType(),
		IsActive:         conv.GetIsActive(),
		CreatedAt:        conv.GetCreatedAt(),
		MemberIds:        memberIDs,
		Name:             conv.GetName(),
		Avatar:           conv.GetAvatar(),
		CreatorId:        conv.GetCreatorId(),
	}, nil
}
