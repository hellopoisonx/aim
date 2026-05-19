package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type SearchUserInfoByNicknameLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewSearchUserInfoByNicknameLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SearchUserInfoByNicknameLogic {
	return &SearchUserInfoByNicknameLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// SearchUserInfoByNickname searches user profiles by nickname prefix.
func (l *SearchUserInfoByNicknameLogic) SearchUserInfoByNickname(in *pb.SearchUserInfoByNicknameReq) (*pb.SearchUserInfoByNicknameResp, error) {
	if in.GetNickname() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	limit := in.GetLimit()
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	infos, err := userSvc.SearchUserInfoByNickname(l.ctx, in.GetNickname(), limit)
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	users := make([]*pb.UserInfoResponse, 0, len(infos))
	for _, info := range infos {
		users = append(users, userInfoToResponse(info))
	}

	return &pb.SearchUserInfoByNicknameResp{
		Users: users,
	}, nil
}
