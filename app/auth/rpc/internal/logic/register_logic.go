package logic

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"

	authsvc "github.com/hellopoisonx/aim/app/auth/rpc/internal/service"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *RegisterLogic) Register(in *pb.RegisterReq) (*pb.RegisterResp, error) {
	email := authsvc.NormalizeEmail(in.GetEmail())
	name := strings.TrimSpace(in.GetUsername())

	if email == "" || in.GetPassword() == "" || name == "" || in.GetDeviceId() == "" {
		return nil, errorx.NewCodeError(authsvc.CodeInvalidArgument, "missing required auth fields")
	}

	passwordHash, err := authsvc.HashPassword(in.GetPassword())
	if err != nil {
		return nil, errorx.NewCodeError(authsvc.CodeInternal, "hash password failed")
	}

	user, err := l.svcCtx.Users.CreateUser(l.ctx, email, passwordHash, name)
	if err != nil {
		if authsvc.IsDuplicateEmail(err) {
			return nil, errorx.NewCodeError(authsvc.CodeConflict, "email already registered")
		}

		return nil, errorx.NewCodeError(authsvc.CodeInternal, "create user failed")
	}

	// Publish user created event to Kafka
	if l.svcCtx.UserEventPublisher != nil {
		event := events.UserCreatedEvent{
			TraceContextFields: tracing.InjectTraceContext(l.ctx),
			UserID:             user.ID,
			Email:              email,
			Nickname:           name,
			Avatar:             in.GetAvatar(),
			CreatedAt:          time.Now().UnixMilli(),
		}

		eventBytes, err := json.Marshal(event)
		if err != nil {
			return nil, errorx.NewCodeError(authsvc.CodeInternal, "publish user created event failed")
		}

		if err := l.svcCtx.UserEventPublisher.Publish(l.ctx, strconv.FormatInt(user.ID, 10), eventBytes); err != nil {
			return nil, errorx.NewCodeError(authsvc.CodeInternal, "publish user created event failed")
		}
	}

	return &pb.RegisterResp{UserId: user.ID}, nil
}
