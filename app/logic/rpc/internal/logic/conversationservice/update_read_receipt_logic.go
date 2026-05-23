package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateReadReceiptLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateReadReceiptLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateReadReceiptLogic {
	return &UpdateReadReceiptLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UpdateReadReceipt upserts the caller's last-read cursor for a conversation.
func (l *UpdateReadReceiptLogic) UpdateReadReceipt(in *pb.UpdateReadReceiptReq) (*pb.UpdateReadReceiptResp, error) {
	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	state, err := convSvc.UpdateReadReceipt(l.ctx, in.GetConversationId(), in.GetUserId(), in.GetLastReadMessageId())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	return &pb.UpdateReadReceiptResp{
		ReadState: &pb.ReadStateItem{
			UserId:            state.UserID,
			LastReadMessageId: state.LastReadMessageID,
			UpdatedAt:         state.UpdatedAt,
		},
	}, nil
}
