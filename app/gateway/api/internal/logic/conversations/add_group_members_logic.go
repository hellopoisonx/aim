package conversations

import (
	"context"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddGroupMembersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddGroupMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddGroupMembersLogic {
	return &AddGroupMembersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AddGroupMembersLogic) AddGroupMembers(req *types.AddGroupMembersRequest) (resp *types.AddGroupMembersResponse, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	rpcResp, err := l.svcCtx.LogicConversationClient.AddGroupMembers(l.ctx, &conversationservice.AddGroupMembersReq{
		ConversationId: req.Id,
		OperatorId:     userID,
		OperatorName:   "",
		MemberIds:      req.MemberIds,
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "add group members", err)
	}

	conv := rpcResp.GetConversation()
	if conv == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "internal error")
	}

	item := types.AddGroupMembersResponse(conversationItemFromPB(conv))

	return &item, nil
}
