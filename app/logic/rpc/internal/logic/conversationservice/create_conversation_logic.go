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

	if in.GetConversationType() != service.ConversationTypeDirect && in.GetConversationType() != service.ConversationTypeGroup {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	memberIDs := in.GetMemberIds()
	var name string
	var displayName string // the name to return to the caller
	if in.GetConversationType() == service.ConversationTypeDirect {
		if len(memberIDs) != 1 {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation member_ids must contain exactly one peer user id")
		}

		if memberIDs[0] <= 0 {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must contain positive user ids")
		}

		if memberIDs[0] == in.GetCreatorId() {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "direct conversation peer must not be creator")
		}

		// For direct conversations, name is not required from the caller.
		// Construct it as "creator_nickname | peer_nickname" and store in DB.
		// Return the peer's name as the display name.
		peerID := memberIDs[0]
		if l.svcCtx.UserInfoService != nil {
			creatorInfo, err := l.svcCtx.UserInfoService.GetUserInfo(l.ctx, in.GetCreatorId())
			if err != nil {
				return nil, service.ToGRPCError(err)
			}
			peerInfo, err := l.svcCtx.UserInfoService.GetUserInfo(l.ctx, peerID)
			if err != nil {
				return nil, service.ToGRPCError(err)
			}
			name = creatorInfo.Nickname + " | " + peerInfo.Nickname
			displayName = peerInfo.Nickname
		}
	} else {
		name = strings.TrimSpace(in.GetName())
		if name == "" {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "name is required")
		}

		if len(memberIDs) == 0 {
			return nil, errorx.NewCodeError(errorx.CodeBadInput, "member_ids must not be empty")
		}
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

	respName := conv.Name
	if in.GetConversationType() == service.ConversationTypeDirect && displayName != "" {
		respName = displayName
	}
	return &pb.CreateConversationResp{
		Conversation: &pb.ConversationResponse{
			Id:               conv.ID,
			ConversationType: conv.ConversationType,
			IsActive:         conv.IsActive,
			CreatedAt:        service.UnixFromPGTimestamptz(conv.CreatedAt),
			MemberIds:        respMemberIDs,
			Name:             respName,
			Avatar:           conv.Avatar,
			CreatorId:        conv.CreatorID,
		},
	}, nil
}
