package logic

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/hellopoisonx/aim/app/auth/rpc/internal/config"
	authsvc "github.com/hellopoisonx/aim/app/auth/rpc/internal/service"
	"github.com/hellopoisonx/aim/app/auth/rpc/internal/svc"
	"github.com/hellopoisonx/aim/app/auth/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
)

func TestAuthClosedLoop(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	issuer := fixedIssuer{}
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, issuer)

	registered, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "Ada@Example.COM",
		Password: "password123",
		Username: "Ada",
		DeviceId: "desktop-1",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), registered.UserId)
	require.NotEqual(t, "password123", users.byEmail["ada@example.com"].PasswordHash)
	require.Equal(t, "Ada", users.byEmail["ada@example.com"].Name)

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
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{})

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{Email: "ada@example.com", Password: "password123", Username: "Ada", DeviceId: "desktop-1"})
	require.NoError(t, err)

	_, err = NewLoginLogic(ctx, svcCtx).Login(&pb.LoginReq{Email: "ada@example.com", Password: "wrong-password", DeviceId: "desktop-1"})
	require.Error(t, err)
}

func TestRepeatedLoginCleansUpOldRefreshToken(t *testing.T) {
	ctx := context.Background()
	users := newMemoryUserStore(t)
	sessions := newMemorySessionStore()
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{})

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{
		Email:    "ada@example.com",
		Password: "password123",
		Username: "Ada",
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
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, newMemoryUserStore(t), newMemorySessionStore(), fixedIssuer{})

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{Email: "ada@example.com", Password: "password123"})
	require.Error(t, err)

	_, err = NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{Email: "ada@example.com", Password: "password123", Username: "   ", DeviceId: "desktop-1"})
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
	svcCtx := svc.NewServiceContextWithStores(config.Config{}, users, sessions, fixedIssuer{})

	_, err := NewRegisterLogic(ctx, svcCtx).Register(&pb.RegisterReq{Email: "ada@example.com", Password: "password123", Username: "Ada", DeviceId: "desktop-1"})
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

func (s *memoryUserStore) CreateUser(_ context.Context, email, passwordHash, name string) (authsvc.UserCredential, error) {
	if s.err != nil {
		return authsvc.UserCredential{}, s.err
	}

	// Simulate duplicate email detection
	if _, exists := s.byEmail[email]; exists {
		return authsvc.UserCredential{}, errorx.NewCodeError(authsvc.CodeConflict, "email already registered")
	}

	user := authsvc.UserCredential{ID: s.nextID, Email: email, PasswordHash: passwordHash, Name: name, Status: authsvc.StatusNormal}
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

// CreateBotCredential is a stub to satisfy the UserStore interface;
// bot-specific tests can override this.
func (s *memoryUserStore) CreateBotCredential(ctx context.Context, email, passwordHash, name string) (authsvc.UserCredential, error) {
	return s.CreateUser(ctx, email, passwordHash, name)
}

type memorySessionStore struct {
	next     int
	rt       map[string][2]string //nolint:unused
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
