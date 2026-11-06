package conversationservicelogic

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateGroupInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateGroupInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateGroupInfoLogic {
	return &UpdateGroupInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UpdateGroupInfoLogic) UpdateGroupInfo(in *pb.UpdateGroupInfoReq) (*pb.UpdateGroupInfoResp, error) {
	convSvc, err := requireGroupOp(l.svcCtx.ConversationService, in.GetConversationId(), in.GetOperatorId())
	if err != nil {
		return nil, err
	}

	var name, avatar *string
	if in.Name != nil {
		trimmed := strings.TrimSpace(in.GetName())
		if trimmed != "" {
			name = &trimmed
		}
	}
	if in.Avatar != nil {
		trimmed := strings.TrimSpace(in.GetAvatar())
		if trimmed != "" {
			avatar = &trimmed
		}
	}

	conv, err := convSvc.UpdateGroupInfo(l.ctx, in.GetConversationId(), in.GetOperatorId(), name, avatar)
	if err != nil {
		return nil, err
	}

	memberIDs, err := conversationMemberIDs(l.ctx, convSvc, conv.ID)
	if err != nil {
		return nil, err
	}

	return &pb.UpdateGroupInfoResp{
		Conversation: pbconv.Conversation(conv, memberIDs, conv.Name),
	}, nil
}
