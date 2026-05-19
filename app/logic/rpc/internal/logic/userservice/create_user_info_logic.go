package userservicelogic

import (
	"context"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/model"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateUserInfoLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateUserInfoLogic {
	return &CreateUserInfoLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreateUserInfo creates a new user profile.
func (l *CreateUserInfoLogic) CreateUserInfo(in *pb.CreateUserInfoReq) (*pb.CreateUserInfoResp, error) {
	if in.GetId() <= 0 {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive")
	}

	if in.GetEmail() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "email is required")
	}

	if in.GetNickname() == "" {
		return nil, errorx.NewCodeError(errorx.CodeBadInput, "nickname is required")
	}

	userSvc := l.svcCtx.UserInfoService
	if userSvc == nil {
		return nil, errorx.NewCodeError(errorx.CodeInternal, "user service is not configured")
	}

	info, err := userSvc.CreateUserInfo(l.ctx, in.GetId(), in.GetEmail(), in.GetNickname(), in.GetAvatar())
	if err != nil {
		return nil, service.ToGRPCError(err)
	}

	return &pb.CreateUserInfoResp{
		User: userInfoToResponse(info),
	}, nil
}

func userInfoToResponse(info model.UserInfo) *pb.UserInfoResponse {
	return &pb.UserInfoResponse{
		Id:        info.ID,
		Email:     info.Email,
		Status:    int32(info.Status),
		Nickname:  info.Nickname,
		Avatar:    info.Avatar,
		CreatedAt: info.CreatedAt.Time.Unix(),
		UpdatedAt: info.UpdatedAt.Time.Unix(),
	}
}
