package conversations

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateConversationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateConversationLogic {
	return &CreateConversationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateConversationLogic) CreateConversation(req *types.CreateConversationRequest) (resp *types.CreateConversationResponse, err error) {
	identity, ok := authctx.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if req.ConversationType != "direct" && req.ConversationType != "group" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	name := strings.TrimSpace(req.Name)

	memberIDs := req.MemberIds
	if req.ConversationType == "direct" {
		if err := sharedconversation.RequireDirectPeer(memberIDs, identity.UserID); err != nil {
			return nil, err
		}
	} else {
		if name == "" {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "name is required")
		}

		if len(memberIDs) == 0 {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
		}
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.CreateConversation(l.ctx, &conversationservice.CreateConversationReq{
		ConversationType: req.ConversationType,
		CreatorId:        identity.UserID,
		MemberIds:        memberIDs,
		Name:             name,
		Avatar:           req.Avatar,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "create conversation", err)
	}

	conv := rpcResp.GetConversation()
	if conv == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	respMemberIDs := conv.GetMemberIds()
	if respMemberIDs == nil {
		respMemberIDs = []int64{}
	}

	return &types.CreateConversationResponse{
		ConversationId:   conv.GetId(),
		ConversationType: conv.GetConversationType(),
		IsActive:         conv.GetIsActive(),
		CreatedAt:        conv.GetCreatedAt(),
		MemberIds:        respMemberIDs,
		Name:             conv.GetName(),
		Avatar:           conv.GetAvatar(),
		CreatorId:        conv.GetCreatorId(),
	}, nil
}
