package botservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
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
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	// Verify owner relationship.
	if _, err := botSvc.GetUserBot(l.ctx, in.GetOwnerUserId(), in.GetBotUserId()); err != nil {
		return nil, err
	}

	if l.svcCtx.ConversationService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service not configured")
	}

	conv, err := l.svcCtx.ConversationService.AddGroupMembers(l.ctx,
		in.GetConversationId(), in.GetOperatorId(),
		[]int64{in.GetBotUserId()})
	if err != nil {
		return nil, err
	}

	return &pb.AddUserBotToConversationResp{
		Conversation: pbconv.Conversation(conv, nil, conv.Name),
	}, nil
}
