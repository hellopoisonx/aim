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

type RemoveGroupMemberLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRemoveGroupMemberLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveGroupMemberLogic {
	return &RemoveGroupMemberLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RemoveGroupMemberLogic) RemoveGroupMember(req *types.RemoveGroupMemberRequest) (resp *types.MutationResult, err error) {
	userID, err := authctx.RequireUserID(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.LogicConversationClient.RemoveGroupMembers(l.ctx, &conversationservice.RemoveGroupMembersReq{
		ConversationId: req.Id,
		OperatorId:     userID,
		MemberIds:      []int64{req.UserId},
	})
	if err != nil {
		return nil, errorx.SanitizeGRPCError(l, "remove group member", err)
	}

	return &types.MutationResult{Success: true}, nil
}
