package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListFriendApplicationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListFriendApplicationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListFriendApplicationsLogic {
	return &ListFriendApplicationsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ListFriendApplications lists pending friend applications received by the user.
func (l *ListFriendApplicationsLogic) ListFriendApplications(in *pb.ListFriendApplicationsReq) (*pb.ListFriendApplicationsResp, error) {
	userID := in.GetUserId()
	if userID <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "user_id must be positive")
	}

	store, err := requireFriendshipStore(l.svcCtx)
	if err != nil {
		return nil, err
	}

	records, err := store.ListPendingFriendApplications(l.ctx, userID)
	if err != nil {
		return nil, errorx.ToGRPCError(err)
	}

	applications := make([]*pb.FriendshipResponse, 0, len(records))
	for _, record := range records {
		applications = append(applications, friendshipToProto(record))
	}

	return &pb.ListFriendApplicationsResp{Applications: applications}, nil
}
