package logic

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateBotCredentialLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateBotCredentialLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateBotCredentialLogic {
	return &CreateBotCredentialLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreateBotCredential creates a disabled placeholder credential for a bot.
func (l *CreateBotCredentialLogic) CreateBotCredential(in *pb.CreateBotCredentialReq) (*pb.CreateBotCredentialResp, error) {
	nickname := strings.TrimSpace(in.GetNickname())
	if nickname == "" {
		return nil, errorx.NewCodeError(service.CodeInvalidArgument, "nickname is required")
	}

	passwordHash, err := service.HashPassword(uuid.NewString() + uuid.NewString())
	if err != nil {
		return nil, errorx.NewCodeError(service.CodeInternal, "create bot credential failed")
	}

	credential, err := l.svcCtx.Users.CreateBotCredential(l.ctx, in.GetEmail(), passwordHash, nickname)
	if err != nil {
		if service.IsDuplicateEmail(err) {
			return nil, errorx.NewCodeError(service.CodeConflict, "email already registered")
		}

		return nil, errorx.NewCodeError(service.CodeInternal, "create bot credential failed")
	}

	return &pb.CreateBotCredentialResp{UserId: credential.ID, Email: credential.Email}, nil
}
