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
	identity, ok := ws.IdentityFromContext(l.ctx)
	if !ok {
		return nil, errorx.NewCodeError(errorx.CodeAuth, "unauthorized")
	}

	if req.ConversationType != "direct" && req.ConversationType != "group" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	if len(req.MemberIds) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	if l.svcCtx.LogicConversationClient == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.CreateConversation(l.ctx, &conversationservice.CreateConversationReq{
		ConversationType: req.ConversationType,
		CreatorId:        identity.UserID,
		MemberIds:        req.MemberIds,
		Name:             req.Name,
	})
	if err != nil {
		return nil, sanitizeLogicRPCError(l, "create conversation", err)
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

func sanitizeLogicRPCError(logger logicRPCErrorLogger, operation string, err error) error {
	if codeErr := errorx.FromGRPCError(err); codeErr != nil {
		return codeErr
	}

	logger.Errorf("logic rpc %s failed: %v", operation, err)

	return errorx.NewCodeError(errorx.CodeInternal, "internal error")
}

type logicRPCErrorLogger interface {
	Errorf(format string, v ...any)
}

func (l *CreateConversationLogic) sanitizeLogicRPCError(operation string, err error) error {
	return sanitizeLogicRPCError(l, operation, err)
}
