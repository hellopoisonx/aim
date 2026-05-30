package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddUserBotToConversationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAddUserBotToConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddUserBotToConversationLogic {
	return &AddUserBotToConversationLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *AddUserBotToConversationLogic) AddUserBotToConversation(in *pb.AddUserBotToConversationReq) (*pb.AddUserBotToConversationResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}

	// Verify owner relationship.
	if _, err := l.svcCtx.BotService.GetUserBot(l.ctx, in.GetOwnerUserId(), in.GetBotUserId()); err != nil {
		return nil, err
	}

	if l.svcCtx.ConversationService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service not configured")
	}

	conv, err := l.svcCtx.ConversationService.AddGroupMembers(l.ctx,
		in.GetConversationId(), in.GetOperatorId(),
		in.GetOperatorName(), []int64{in.GetBotUserId()})
	if err != nil {
		return nil, err
	}

	return &pb.AddUserBotToConversationResp{
		Conversation: &pb.ConversationResponse{
			Id:               conv.ID,
			ConversationType: conv.ConversationType,
			IsActive:         conv.IsActive,
			CreatedAt:        conv.CreatedAt.Time.UnixMilli(),
			Name:             conv.Name,
			Avatar:           conv.Avatar,
			CreatorId:        conv.CreatorID,
		},
	}, nil
}
