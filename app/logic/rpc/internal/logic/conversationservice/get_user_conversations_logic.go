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

type GetUserConversationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserConversationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserConversationsLogic {
	return &GetUserConversationsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetUserConversations retrieves all conversations the user is a member of.
func (l *GetUserConversationsLogic) GetUserConversations(in *pb.GetUserConversationsReq) (*pb.GetUserConversationsResp, error) {
	if in.GetUserId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id is required and must be positive")
	}

	convSvc := l.svcCtx.ConversationService
	if convSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "conversation service is not configured")
	}

	conversations, err := convSvc.GetUserConversations(l.ctx, in.GetUserId())
	if err != nil {
		return nil, service.ConversationToGRPCError(err)
	}

	pbConversations := make([]*pb.ConversationResponse, 0, len(conversations))

	// Pre-fetch the caller's nickname for use in computing direct conversation display names.
	var callerNickname string
	if l.svcCtx.UserInfoService != nil {
		if callerInfo, err := l.svcCtx.UserInfoService.GetUserInfo(l.ctx, in.GetUserId()); err == nil {
			callerNickname = callerInfo.Nickname
		}
	}

	for _, conv := range conversations {
		members, err := convSvc.GetConversationMembers(l.ctx, conv.ID)
		if err != nil {
			return nil, service.ConversationToGRPCError(err)
		}

		memberIDs := make([]int64, len(members))
		for i, m := range members {
			memberIDs[i] = m.UserID
		}

		// For direct conversations, compute display name as the other member's nickname.
		name := conv.Name
		if conv.ConversationType == "direct" && callerNickname != "" {
			if parts := strings.SplitN(conv.Name, " | ", 2); len(parts) == 2 {
				if parts[0] == callerNickname {
					name = parts[1]
				} else {
					name = parts[0]
				}
			}
		}

		pbConversations = append(pbConversations, &pb.ConversationResponse{
			Id:               conv.ID,
			ConversationType: conv.ConversationType,
			IsActive:         conv.IsActive,
			CreatedAt:        service.UnixFromPGTimestamptz(conv.CreatedAt),
			MemberIds:        memberIDs,
			Name:             name,
			Avatar:           conv.Avatar,
			CreatorId:        conv.CreatorID,
		})
	}

	return &pb.GetUserConversationsResp{
		Conversations: pbConversations,
	}, nil
}
