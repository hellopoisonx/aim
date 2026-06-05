package logic

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/model"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"

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
	email := service.NormalizeEmail(in.GetEmail())
	name := strings.TrimSpace(in.GetUsername())

	if email == "" || in.GetPassword() == "" || name == "" || in.GetDeviceId() == "" {
		return nil, errorx.NewCodeError(service.CodeInvalidArgument, "missing required auth fields")
	}

	passwordHash, err := service.HashPassword(in.GetPassword())
	if err != nil {
		return nil, errorx.NewCodeError(service.CodeInternal, "hash password failed")
	}

	// Publish event via outbox if configured
	topic := l.svcCtx.Config.KqPusherConf.Topic
	hasOutbox := l.svcCtx.OutboxPoller != nil && topic != "" && l.svcCtx.IDGen != nil && l.svcCtx.DB != nil

	var user service.UserCredential

	if hasOutbox {
		user, err = l.createUserWithOutbox(l.ctx, email, passwordHash, name, topic)
	} else {
		user, err = l.svcCtx.Users.CreateUser(l.ctx, email, passwordHash, name)
	}
	if err != nil {
		if service.IsDuplicateEmail(err) {
			return nil, errorx.NewCodeError(service.CodeConflict, "email already registered")
		}
		return nil, errorx.NewCodeError(service.CodeInternal, "create user failed")
	}

	return &pb.RegisterResp{UserId: user.ID}, nil
}

// createUserWithOutbox handles user creation with outbox event recording in a transaction.
func (l *RegisterLogic) createUserWithOutbox(
	ctx context.Context,
	email, passwordHash, name string,
	topic string,
) (service.UserCredential, error) {
	tx, err := l.svcCtx.DB.Begin(ctx)
	if err != nil {
		return service.UserCredential{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txQueries := model.New(tx)
	txStore := service.NewSQLUserStoreWithIDGenerator(txQueries, l.svcCtx.IDGen)

	user, err := txStore.CreateUser(ctx, email, passwordHash, name)
	if err != nil {
		return service.UserCredential{}, err
	}

	// Build and insert outbox event
	event := events.UserCreatedEvent{
		TraceContextFields: tracing.InjectTraceContext(ctx),
		UserID:             user.ID,
		Email:              email,
		Nickname:           name,
		CreatedAt:          time.Now().UnixMilli(),
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		return service.UserCredential{}, err
	}

	_, err = txQueries.InsertOutboxRecord(ctx, model.InsertOutboxRecordParams{
		Topic:   topic,
		Key:     strconv.FormatInt(user.ID, 10),
		Payload: eventBytes,
	})
	if err != nil {
		return service.UserCredential{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return service.UserCredential{}, err
	}

	// Fast path: wake the poller immediately
	l.svcCtx.OutboxPoller.Wake()

	return user, nil
}
