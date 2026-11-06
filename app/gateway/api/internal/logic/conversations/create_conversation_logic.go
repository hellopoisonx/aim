package conversations

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/pbconv"

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

func (l *CreateConversationLogic) CreateConversation(req *types.CreateConversationRequest) (resp *types.ConversationItem, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	conversationType := sharedconversation.Type(req.ConversationType)
	if conversationType != sharedconversation.TypeDirect && conversationType != sharedconversation.TypeGroup {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	name := strings.TrimSpace(req.Name)

	memberIDs := req.MemberIds
	if conversationType == sharedconversation.TypeDirect {
		if err := sharedconversation.RequireDirectPeer(memberIDs, userID); err != nil {
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
		ConversationType: string(conversationType),
		CreatorId:        userID,
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

	item := pbconv.Conversation(conv)

	return &item, nil
}
