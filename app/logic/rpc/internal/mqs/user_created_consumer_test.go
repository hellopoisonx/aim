package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/config"
	"github.com/hellopoisonx/aim/app/logic/rpc/model"
	logicsvc "github.com/hellopoisonx/aim/app/logic/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/tracing"
	"go.opentelemetry.io/otel"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-queue/kq"
	"go.opentelemetry.io/otel/trace"
)

// --- Fake UserInfoService ---

type fakeUserInfoService struct {
	calls []createdUser
	err   error
}

type createdUser struct {
	id       int64
	email    string
	nickname string
	avatar   string
	traceID  trace.TraceID
}

func (f *fakeUserInfoService) CreateUserInfo(ctx context.Context, id int64, email, nickname, avatar string) (model.UserInfo, error) {
	if f.err != nil {
		return model.UserInfo{}, f.err
	}

	f.calls = append(f.calls, createdUser{id: id, email: email, nickname: nickname, avatar: avatar, traceID: trace.SpanContextFromContext(ctx).TraceID()})

	return model.UserInfo{}, nil
}

func (f *fakeUserInfoService) GetUserInfo(_ context.Context, id int64) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeUserInfoService) GetUserInfoByEmail(_ context.Context, email string) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeUserInfoService) GetUserInfoByNickname(_ context.Context, nickname string) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeUserInfoService) UpdateUserInfoProfile(_ context.Context, id int64, nickname, avatar string) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeUserInfoService) UpdateUserInfoStatus(_ context.Context, id int64, status int16) (model.UserInfo, error) {
	return model.UserInfo{}, nil
}

func (f *fakeUserInfoService) SearchUserInfoByNickname(_ context.Context, nickname string, limit int32) ([]model.UserInfo, error) {
	return nil, nil
}

func newTestSvcCtxWithUserSvc(userSvc *fakeUserInfoService) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config: config.Config{
			UserCreatedConsumerConf: kq.KqConf{},
		},
		UserInfoService: userSvc,
	}
}

// --- Test cases ---

func TestUserCreatedConsumer_Consume_Success(t *testing.T) {
	userSvc := &fakeUserInfoService{}
	svcCtx := newTestSvcCtxWithUserSvc(userSvc)
	consumer := NewUserCreatedConsumer(context.Background(), svcCtx)

	event := events.UserCreatedEvent{
		UserID:    12345,
		Email:     "ada@example.com",
		Nickname:  "Ada",
		Avatar:    "https://example.com/avatar.png",
		CreatedAt: 1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "12345", string(value))
	require.NoError(t, err)
	require.Len(t, userSvc.calls, 1)
	require.Equal(t, int64(12345), userSvc.calls[0].id)
	require.Equal(t, "ada@example.com", userSvc.calls[0].email)
	require.Equal(t, "Ada", userSvc.calls[0].nickname)
	require.Equal(t, "https://example.com/avatar.png", userSvc.calls[0].avatar)
}

func TestUserCreatedConsumer_Consume_PropagatesTraceContext(t *testing.T) {
	userSvc := &fakeUserInfoService{}
	svcCtx := newTestSvcCtxWithUserSvc(userSvc)
	consumer := NewUserCreatedConsumer(context.Background(), svcCtx)

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)

	event := events.UserCreatedEvent{
		TraceContextFields: tracing.TraceContextFields{
			TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		},
		UserID:    12345,
		Email:     "ada@example.com",
		Nickname:  "Ada",
		CreatedAt: 1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "12345", string(value))
	require.NoError(t, err)
	require.Len(t, userSvc.calls, 1)
	require.Equal(t, traceID, userSvc.calls[0].traceID)
}

func TestUserCreatedConsumer_Consume_InvalidJSON(t *testing.T) {
	userSvc := &fakeUserInfoService{}
	svcCtx := newTestSvcCtxWithUserSvc(userSvc)
	consumer := NewUserCreatedConsumer(context.Background(), svcCtx)

	err := consumer.Consume(context.Background(), "12345", "invalid json{")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid character")
}

func TestUserCreatedConsumer_Consume_ServiceError(t *testing.T) {
	userSvc := &fakeUserInfoService{err: errors.New("database error")} // different from ErrUserExists
	svcCtx := newTestSvcCtxWithUserSvc(userSvc)
	consumer := NewUserCreatedConsumer(context.Background(), svcCtx)

	event := events.UserCreatedEvent{
		UserID:    12345,
		Email:     "ada@example.com",
		Nickname:  "Ada",
		Avatar:    "",
		CreatedAt: 1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "12345", string(value))
	require.Error(t, err)
	require.Contains(t, err.Error(), "database error")
}

func TestUserCreatedConsumer_Consume_RecordsSpanError(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	tp := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(spans))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	userSvc := &fakeUserInfoService{err: errors.New("database error")}
	svcCtx := newTestSvcCtxWithUserSvc(userSvc)
	consumer := NewUserCreatedConsumer(context.Background(), svcCtx)

	event := events.UserCreatedEvent{UserID: 12345, Email: "ada@example.com", Nickname: "Ada", Avatar: "", CreatedAt: 1700000000000}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "12345", string(value))
	require.Error(t, err)

	ended := spans.Ended()
	require.NotEmpty(t, ended)
	require.NotEmpty(t, ended[0].Events())
}

func TestUserCreatedConsumer_Consume_DuplicateIdempotentSuccess(t *testing.T) {
	userSvc := &fakeUserInfoService{err: logicsvc.ErrUserExists}
	svcCtx := newTestSvcCtxWithUserSvc(userSvc)
	consumer := NewUserCreatedConsumer(context.Background(), svcCtx)

	event := events.UserCreatedEvent{
		UserID:    12345,
		Email:     "ada@example.com",
		Nickname:  "Ada",
		Avatar:    "",
		CreatedAt: 1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "12345", string(value))
	require.NoError(t, err) // ErrUserExists should be treated as idempotent success
}

func TestUserCreatedConsumer_Consume_NilServiceSkips(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Config: config.Config{
			UserCreatedConsumerConf: kq.KqConf{},
		},
		UserInfoService: nil,
	}
	consumer := NewUserCreatedConsumer(context.Background(), svcCtx)

	event := events.UserCreatedEvent{
		UserID:    12345,
		Email:     "ada@example.com",
		Nickname:  "Ada",
		Avatar:    "",
		CreatedAt: 1700000000000,
	}
	value, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.Consume(context.Background(), "12345", string(value))
	require.Error(t, err)
	require.Contains(t, err.Error(), "user info service is not configured")
}
