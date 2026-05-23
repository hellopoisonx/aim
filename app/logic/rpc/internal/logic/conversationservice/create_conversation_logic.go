package conversationservicelogic

import (
	"context"
	"strings"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateConversationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateConversationLogic {
	return &CreateConversationLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateConversationLogic) CreateConversation(in *pb.CreateConversationReq) (*pb.CreateConversationResp, error) {
	if in.GetCreatorId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "creator_id is required and must be positive")
	}

	if in.GetConversationType() != "direct" && in.GetConversationType() != "group" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	name := strings.TrimSpace(in.GetName())
	if name == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "name is required")
	}

	memberIDs := in.GetMemberIds()
	if in.GetConversationType() == "direct" {
		if len(memberIDs) != 1 {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation member_ids must contain exactly one peer user id")
		}
		if memberIDs[0] <= 0 {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
		}
		if memberIDs[0] == in.GetCreatorId() {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation peer must not be creator")
		}
	} else if len(memberIDs) == 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
	}

	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	conv, err := convSvc.CreateConversation(l.ctx, in.GetConversationType(), in.GetCreatorId(), memberIDs, name, in.GetAvatar())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	members, err := convSvc.GetConversationMembers(l.ctx, conv.ID)
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	respMemberIDs := make([]int64, len(members))
	for i, m := range members {
		respMemberIDs[i] = m.UserID
	}

	return &pb.CreateConversationResp{
		Conversation: &pb.ConversationResponse{
			Id:               conv.ID,
			ConversationType: conv.ConversationType,
			IsActive:         conv.IsActive,
			CreatedAt:        service.UnixFromPGTimestamptz(conv.CreatedAt),
			MemberIds:        respMemberIDs,
			Name:             conv.Name,
			Avatar:           conv.Avatar,
			CreatorId:        conv.CreatorID,
		},
	}, nil
}
