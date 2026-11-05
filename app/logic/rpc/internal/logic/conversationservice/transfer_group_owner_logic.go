package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type TransferGroupOwnerLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewTransferGroupOwnerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TransferGroupOwnerLogic {
	return &TransferGroupOwnerLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *TransferGroupOwnerLogic) TransferGroupOwner(in *pb.TransferGroupOwnerReq) (*pb.TransferGroupOwnerResp, error) {
	convSvc, err := requireGroupOp(l.svcCtx.ConversationService, in.GetConversationId(), in.GetOperatorId())
	if err != nil {
		return nil, err
	}

	if in.GetTargetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "target_user_id is required")
	}

	conv, err := convSvc.TransferGroupOwner(l.ctx, in.GetConversationId(), in.GetOperatorId(), in.GetTargetUserId())
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	memberIDs, err := conversationMemberIDs(l.ctx, convSvc, conv.ID)
	if err != nil {
		return nil, err
	}

	return &pb.TransferGroupOwnerResp{
		Conversation: pbconv.Conversation(conv, memberIDs, conv.Name),
	}, nil
}
