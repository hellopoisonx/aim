package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateUserBotDirectConversationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateUserBotDirectConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserBotDirectConversationLogic {
	return &CreateUserBotDirectConversationLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateUserBotDirectConversationLogic) CreateUserBotDirectConversation(in *pb.CreateUserBotDirectConversationReq) (*pb.CreateUserBotDirectConversationResp, error) {
	if l.svcCtx.BotService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "bot service not configured")
	}
	// Verify owner relationship and get bot profile info.
	botInfo, err := l.svcCtx.BotService.GetUserBot(l.ctx, in.GetOwnerUserId(), in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	if l.svcCtx.ConversationService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service not configured")
	}

	conv, err := l.svcCtx.ConversationService.CreateConversation(l.ctx,
		"direct", in.GetOwnerUserId(),
		[]int64{in.GetBotUserId()}, botInfo.Nickname, "")
	if err != nil {
		return nil, err
	}

	return &pb.CreateUserBotDirectConversationResp{
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
