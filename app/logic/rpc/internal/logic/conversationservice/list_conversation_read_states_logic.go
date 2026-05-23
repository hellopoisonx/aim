package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListConversationReadStatesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListConversationReadStatesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListConversationReadStatesLogic {
	return &ListConversationReadStatesLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ListConversationReadStates returns the per-member last-read cursors for a conversation.
func (l *ListConversationReadStatesLogic) ListConversationReadStates(in *pb.ListConversationReadStatesReq) (*pb.ListConversationReadStatesResp, error) {
	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	states, err := convSvc.ListConversationReadStates(l.ctx, in.GetConversationId())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	items := make([]*pb.ReadStateItem, len(states))
	for i, st := range states {
		items[i] = &pb.ReadStateItem{
			UserId:            st.UserID,
			LastReadMessageId: st.LastReadMessageID,
			UpdatedAt:         st.UpdatedAt,
		}
	}

	return &pb.ListConversationReadStatesResp{ReadStates: items}, nil
}
