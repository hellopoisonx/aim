package friendshipservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"

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

	if l.svcCtx.DB == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "database is not configured")
	}

	records, err := model.New(l.svcCtx.DB).ListPendingFriendApplications(l.ctx, userID)
	if err != nil {
		return nil, FriendshipToGRPCError(err)
	}

	applications := make([]*pb.FriendshipResponse, 0, len(records))
	for _, record := range records {
		applications = append(applications, &pb.FriendshipResponse{
			UserId:    record.UserID,
			FriendId:  record.FriendID,
			Status:    record.Status,
			CreatedAt: service.UnixFromPGTimestamptz(record.CreatedAt),
			UpdatedAt: service.UnixFromPGTimestamptz(record.UpdatedAt),
		})
	}

	return &pb.ListFriendApplicationsResp{Applications: applications}, nil
}
