package botservicelogic

import (
	"context"

	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
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
	botSvc, err := requireBot(l.svcCtx, in.GetBotUserId())
	if err != nil {
		return nil, err
	}
	// Verify owner relationship and get bot profile info.
	botInfo, err := botSvc.GetUserBot(l.ctx, in.GetOwnerUserId(), in.GetBotUserId())
	if err != nil {
		return nil, err
	}

	if l.svcCtx.ConversationService == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service not configured")
	}

	conv, err := l.svcCtx.ConversationService.CreateConversation(l.ctx,
		sharedconversation.TypeDirect, in.GetOwnerUserId(),
		[]int64{in.GetBotUserId()}, botInfo.Nickname, "")
	if err != nil {
		return nil, err
	}

	return &pb.CreateUserBotDirectConversationResp{
		Conversation: pbconv.Conversation(conv, nil, conv.Name),
	}, nil
}
