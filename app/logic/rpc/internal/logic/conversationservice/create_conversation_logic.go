package conversationservicelogic

import (
	"context"
	"strings"

	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/pbconv"
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

	if sharedconversation.Type(in.GetConversationType()) != sharedconversation.TypeDirect && sharedconversation.Type(in.GetConversationType()) != sharedconversation.TypeGroup {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'")
	}

	memberIDs := in.GetMemberIds()
	var name string
	var displayName string // the name to return to the caller
	if sharedconversation.Type(in.GetConversationType()) == sharedconversation.TypeDirect {
		if err := sharedconversation.RequireDirectPeer(memberIDs, in.GetCreatorId()); err != nil {
			return nil, err
		}

		// For direct conversations, name is not required from the caller.
		// Construct it as "creator_nickname | peer_nickname" and store in DB.
		// Return the peer's name as the display name.
		peerID := memberIDs[0]
		if l.svcCtx.UserInfoService != nil {
			creatorInfo, err := l.svcCtx.UserInfoService.GetUserInfo(l.ctx, in.GetCreatorId())
			if err != nil {
				return nil, err
			}
			peerInfo, err := l.svcCtx.UserInfoService.GetUserInfo(l.ctx, peerID)
			if err != nil {
				return nil, err
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

	convSvc, err := requireConversationService(l.svcCtx.ConversationService)
	if err != nil {
		return nil, err
	}

	conv, err := convSvc.CreateConversation(l.ctx, sharedconversation.Type(in.GetConversationType()), in.GetCreatorId(), memberIDs, name, in.GetAvatar())
	if err != nil {
		return nil, err
	}

	respMemberIDs, err := conversationMemberIDs(l.ctx, convSvc, conv.ID)
	if err != nil {
		return nil, err
	}

	respName := conv.Name
	if sharedconversation.Type(in.GetConversationType()) == sharedconversation.TypeDirect && displayName != "" {
		respName = displayName
	}

	return &pb.CreateConversationResp{
		Conversation: pbconv.Conversation(conv, respMemberIDs, respName),
	}, nil
}
