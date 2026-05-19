package logic

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	authsvc "github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// --- Fake publisher ---

type fakePublisher struct {
	calls []publishedEvent
	err   error
}

type publishedEvent struct {
	key   string
	value []byte
}

func (p *fakePublisher) Publish(_ context.Context, key string, value []byte) error {
	if p.err != nil {
		return p.err
	}

	p.calls = append(p.calls, publishedEvent{key: key, value: value})

	return nil
}

func TestAuthClosedLoop(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	issuer := fixedIssuer{}
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, issuer, nil) // nil publisher = no-op

	registered, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "Ada@Example.COM",
		Password: "password123",
		DeviceId: "desktop-1",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), registered.UserId)
	require.NotEqual(t, "password123", users.byEmail["ada@example.com"].PasswordHash)

	loggedIn, err := NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.NoError(t, err)
	require.Equal(t, int64(1), loggedIn.UserId)
	require.Equal(t, "access-1-desktop-1", loggedIn.AccessToken)
	require.NotEmpty(t, loggedIn.RefreshToken)
	require.Equal(t, int64(123), loggedIn.ExpiresAt)

	refreshed, err := NewRefreshTokenLogic(ctx, svcCtx).RefreshToken(&pb.RefreshTokenReq{RefreshToken: loggedIn.RefreshToken})
	require.NoError(t, err)
	require.NotEqual(t, loggedIn.RefreshToken, refreshed.RefreshToken)
	require.Equal(t, "access-1-desktop-1", refreshed.AccessToken)

	_, err = NewRefreshTokenLogic(ctx, svcCtx).RefreshToken(&pb.RefreshTokenReq{RefreshToken: loggedIn.RefreshToken})
	require.Error(t, err)

	loggedOut, err := NewLogoutLogic(ctx, svcCtx).Logout(&pb.LogoutReq{UserId: 1, DeviceId: "desktop-1"})
	require.NoError(t, err)
	require.True(t, loggedOut.Success)

	_, err = NewRefreshTokenLogic(ctx, svcCtx).RefreshToken(&pb.RefreshTokenReq{RefreshToken: refreshed.RefreshToken})
	require.Error(t, err)
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{}, nil)

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.NoError(t, err)

	_, err = NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "wrong-password", DeviceId: "desktop-1"})
	require.Error(t, err)
}

func TestRepeatedLoginCleansUpOldRefreshToken(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{}, nil)

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "ada@example.com",
		Password: "password123",
		DeviceId: "desktop-1",
	})
	require.NoError(t, err)

	first, err := NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{
		Email:    "ada@example.com",
		Password: "password123",
		DeviceId: "desktop-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, first.RefreshToken)

	second, err := NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{
		Email:    "ada@example.com",
		Password: "password123",
		DeviceId: "desktop-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, second.RefreshToken)
	require.NotEqual(t, first.RefreshToken, second.RefreshToken)

	// Old refresh token must be invalidated
	_, err = NewRefreshTokenLogic(ctx, svcCtx).RefreshToken(&pb.RefreshTokenReq{RefreshToken: first.RefreshToken})
	require.Error(t, err, "old refresh token should be invalid after re-login")

	// New refresh token must work
	refreshed, err := NewRefreshTokenLogic(ctx, svcCtx).RefreshToken(&pb.RefreshTokenReq{RefreshToken: second.RefreshToken})
	require.NoError(t, err)
	require.NotEmpty(t, refreshed.RefreshToken)
}

func TestAuthValidationErrors(t *testing.T) {
	ctx := context.Background()
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, newMemoryUserStore(t), newMemorySessionStore(), fixedIssuer{}, nil)

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{Email: "ada@example.com", Password: "password123"})
	require.Error(t, err)

	_, err = NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.Error(t, err)

	_, err = NewRefreshTokenLogic(ctx, svcCtx).RefreshToken(&pb.RefreshTokenReq{})
	require.Error(t, err)

	_, err = NewLogoutLogic(ctx, svcCtx).Logout(&pb.LogoutReq{UserId: 1})
	require.Error(t, err)
}

func TestAuthLogicInternalErrorBranches(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{}, nil)

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.NoError(t, err)

	users.err = errors.New("database down")
	_, err = NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.Error(t, err)
	users.err = nil

	svcCtx.TokenIssuer = failingIssuer{}
	_, err = NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.Error(t, err)

	svcCtx.TokenIssuer = fixedIssuer{}
	sessions.err = errors.New("redis down")
	_, err = NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.Error(t, err)
	sessions.err = nil

	loginResp, err := NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "password123", DeviceId: "desktop-1"})
	require.NoError(t, err)

	svcCtx.TokenIssuer = failingIssuer{}
	_, err = NewRefreshTokenLogic(ctx, svcCtx).RefreshToken(&pb.RefreshTokenReq{RefreshToken: loginResp.RefreshToken})
	require.Error(t, err)

	svcCtx.TokenIssuer = fixedIssuer{}
	sessions.err = errors.New("redis down")
	_, err = NewLogoutLogic(ctx, svcCtx).Logout(&pb.LogoutReq{UserId: 1, DeviceId: "desktop-1"})
	require.Error(t, err)
}

type memoryUserStore struct {
	nextID  int64
	byEmail map[string]authsvc.UserCredential
	err     error
}

func newMemoryUserStore(t *testing.T) *memoryUserStore {
	t.Helper()
	return &memoryUserStore{nextID: 1, byEmail: map[string]authsvc.UserCredential{}}
}

func (s *memoryUserStore) CreateUser(_ context.Context, email, passwordHash string) (authsvc.UserCredential, error) {
	if s.err != nil {
		return authsvc.UserCredential{}, s.err
	}

	// Simulate duplicate email detection
	if _, exists := s.byEmail[email]; exists {
		return authsvc.UserCredential{}, errorx.NewCodeError(authsvc.CodeConflict, "email already registered")
	}

	user := authsvc.UserCredential{ID: s.nextID, Email: email, PasswordHash: passwordHash, Status: authsvc.StatusNormal}
	s.nextID++
	s.byEmail[email] = user

	return user, nil
}

func (s *memoryUserStore) GetUserByEmail(_ context.Context, email string) (authsvc.UserCredential, error) {
	if s.err != nil {
		return authsvc.UserCredential{}, s.err
	}

	user, ok := s.byEmail[email]
	if !ok {
		return authsvc.UserCredential{}, errorx.NewCodeError(authsvc.CodeUnauthorized, "not found")
	}

	return user, nil
}

type memorySessionStore struct {
	next     int
	rt       map[string][2]string
	byDevice map[[2]string]string
	err      error
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{rt: map[string][2]string{}, byDevice: map[[2]string]string{}}
}

func (s *memorySessionStore) Create(_ context.Context, userID int64, deviceID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}

	uid := strconv.FormatInt(userID, 10)
	deviceKey := [2]string{uid, deviceID}

	// Clean up old refresh token if this device already has a session
	if oldToken, ok := s.byDevice[deviceKey]; ok {
		delete(s.rt, oldToken)
	}

	s.next++
	token := "refresh-" + strconv.Itoa(s.next)
	s.rt[token] = [2]string{uid, deviceID}
	s.byDevice[deviceKey] = token

	return token, nil
}

func (s *memorySessionStore) Rotate(ctx context.Context, token string) (int64, string, string, error) {
	if s.err != nil {
		return 0, "", "", s.err
	}

	entry, ok := s.rt[token]
	if !ok {
		return 0, "", "", errorx.NewCodeError(authsvc.CodeUnauthorized, "invalid refresh token")
	}

	delete(s.rt, token)

	return 1, entry[1], must(s.Create(ctx, 1, entry[1])), nil
}

func (s *memorySessionStore) RevokeDevice(_ context.Context, userID int64, deviceID string) error {
	if s.err != nil {
		return s.err
	}

	key := [2]string{strconv.FormatInt(userID, 10), deviceID}
	token := s.byDevice[key]
	delete(s.byDevice, key)
	delete(s.rt, token)

	return nil
}

type fixedIssuer struct{}

func (fixedIssuer) Issue(_ context.Context, userID int64, deviceID string) (string, int64, error) {
	return "access-1-" + deviceID, 123, nil
}

type failingIssuer struct{}

func (failingIssuer) Issue(context.Context, int64, string) (string, int64, error) {
	return "", 0, errors.New("issuer failed")
}

func must(token string, err error) string {
	if err != nil {
		panic(err)
	}

	return token
}

// --- User event publisher tests ---

func TestRegister_PublishesCorrectEvent(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)

	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	publisher := &fakePublisher{}
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{}, publisher)

	_, err = NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "Ada@Example.COM",
		Password: "password123",
		DeviceId: "desktop-1",
		Username: "Ada",
		Avatar:   "https://example.com/avatar.png",
	})
	require.NoError(t, err)
	require.Len(t, publisher.calls, 1)
	require.Equal(t, "1", publisher.calls[0].key)

	var event events.UserCreatedEvent

	err = json.Unmarshal(publisher.calls[0].value, &event)
	require.NoError(t, err)
	require.Equal(t, int64(1), event.UserID)
	require.Equal(t, "ada@example.com", event.Email)
	require.Equal(t, "Ada", event.Nickname)
	require.Equal(t, "https://example.com/avatar.png", event.Avatar)
	require.NotZero(t, event.CreatedAt)
	require.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", event.TraceParent)
}

func TestRegister_DoesNotPublishWhenCreateFails(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	publisher := &fakePublisher{}
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{}, publisher)

	users.err = errors.New("database error")
	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "ada@example.com",
		Password: "password123",
		DeviceId: "desktop-1",
	})
	require.Error(t, err)
	require.Empty(t, publisher.calls)
}

func TestRegister_DoesNotPublishOnDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	publisher := &fakePublisher{}
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{}, publisher)

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "ada@example.com",
		Password: "password123",
		DeviceId: "desktop-1",
	})
	require.NoError(t, err)
	require.Len(t, publisher.calls, 1)

	// Duplicate registration
	_, err = NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "ada@example.com",
		Password: "password456",
		DeviceId: "desktop-2",
	})
	require.Error(t, err)
	require.Len(t, publisher.calls, 1) // Should not publish again
}

func TestRegister_ReturnsErrorWhenPublisherFails(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	publisher := &fakePublisher{err: errors.New("kafka error")}
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{}, publisher)

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "ada@example.com",
		Password: "password123",
		DeviceId: "desktop-1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "publish user created event failed")
}
