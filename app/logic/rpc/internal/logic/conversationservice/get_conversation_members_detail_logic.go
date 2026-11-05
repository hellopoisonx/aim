package conversationservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetConversationMembersDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetConversationMembersDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetConversationMembersDetailLogic {
	return &GetConversationMembersDetailLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetConversationMembersDetailLogic) GetConversationMembersDetail(in *pb.GetConversationMembersDetailReq) (*pb.GetConversationMembersDetailResp, error) {
	convSvc, err := requireConversationOp(l.svcCtx.ConversationService, in.GetConversationId())
	if err != nil {
		return nil, err
	}

	details, err := convSvc.GetConversationMembersDetail(l.ctx, in.GetConversationId())
	if err != nil {
		return nil, err
	}

	pbMembers := make([]*pb.MemberDetailItem, 0, len(details))
	for _, d := range details {
		pbMembers = append(pbMembers, &pb.MemberDetailItem{
			UserId:   d.UserID,
			Email:    d.Email,
			Avatar:   d.Avatar,
			Role:     d.Role,
			JoinedAt: d.JoinedAt,
			Name:     d.Name,
		})
	}

	return &pb.GetConversationMembersDetailResp{
		Members: pbMembers,
	}, nil
}
