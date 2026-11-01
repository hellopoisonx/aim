package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/auth/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	sharedjwt "github.com/hellopoisonx/aim/app/shared/jwt"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	StatusDisabled = 0
	StatusNormal   = 1

	CodeInvalidArgument = 40000
	CodeUnauthorized    = 40100
	CodeConflict        = 40900
	CodeInternal        = 50000
)

type UserStore interface {
	CreateUser(ctx context.Context, email, passwordHash, name string) (UserCredential, error)
	CreateBotCredential(ctx context.Context, email, passwordHash, name string) (UserCredential, error)
	GetUserByEmail(ctx context.Context, email string) (UserCredential, error)
}

type SessionStore interface {
	Create(ctx context.Context, userID int64, deviceID string) (string, error)
	Rotate(ctx context.Context, refreshToken string) (userID int64, deviceID string, nextToken string, err error)
	RevokeDevice(ctx context.Context, userID int64, deviceID string) error
}

type TokenIssuer interface {
	Issue(ctx context.Context, userID int64, deviceID string) (accessToken string, expiresAt int64, err error)
}

type UserCredential struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	Status       int16
}

// userDAO is the DAO slice used by DAOUserStore.
type userDAO interface {
	CreateUser(ctx context.Context, id int64, email, passwordHash, name string) (orm.UserCredential, error)
	CreateBotCredential(ctx context.Context, id int64, email, passwordHash, name string) (orm.UserCredential, error)
	GetUserByEmail(ctx context.Context, email string) (orm.UserCredential, error)
}

type DAOUserStore struct {
	store userDAO
	ids   *tools.Snowflake
}

func NewDAOUserStore(store userDAO) *DAOUserStore {
	return NewDAOUserStoreWithMachineID(store, 1)
}

func NewDAOUserStoreWithMachineID(store userDAO, machineID int64) *DAOUserStore {
	ids, err := tools.NewSnowflake(machineID)
	if err != nil {
		panic(err)
	}

	return NewDAOUserStoreWithIDGenerator(store, ids)
}

func NewDAOUserStoreWithIDGenerator(store userDAO, ids *tools.Snowflake) *DAOUserStore {
	return &DAOUserStore{store: store, ids: ids}
}

func (s *DAOUserStore) CreateUser(ctx context.Context, email, passwordHash, name string) (UserCredential, error) {
	id, err := s.ids.NextID()
	if err != nil {
		return UserCredential{}, err
	}

	user, err := s.store.CreateUser(ctx, id, email, passwordHash, name)
	if err != nil {
		return UserCredential{}, err
	}

	return userCredentialFromORM(user), nil
}

func (s *DAOUserStore) CreateBotCredential(ctx context.Context, email, passwordHash, name string) (UserCredential, error) {
	id, err := s.ids.NextID()
	if err != nil {
		return UserCredential{}, err
	}

	email = NormalizeEmail(email)
	if email == "" {
		email = DefaultBotEmail(id)
	}

	user, err := s.store.CreateBotCredential(ctx, id, email, passwordHash, name)
	if err != nil {
		return UserCredential{}, err
	}

	return userCredentialFromORM(user), nil
}

func (s *DAOUserStore) GetUserByEmail(ctx context.Context, email string) (UserCredential, error) {
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return UserCredential{}, err
	}

	return userCredentialFromORM(user), nil
}

func userCredentialFromORM(user orm.UserCredential) UserCredential {
	return UserCredential{ID: user.ID, Email: user.Email, PasswordHash: user.PasswordHash, Name: user.Name, Status: user.Status}
}

type RedisSessionStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisSessionStore(client *redis.Client, ttl time.Duration) *RedisSessionStore {
	return &RedisSessionStore{client: client, ttl: ttl}
}

func (s *RedisSessionStore) Create(ctx context.Context, userID int64, deviceID string) (string, error) {
	token := uuid.NewString()
	pipe := s.client.TxPipeline()

	// If a session already exists for this device, revoke the old refresh token
	if oldToken, err := s.client.Get(ctx, deviceKey(userID, deviceID)).Result(); err == nil && oldToken != "" {
		pipe.Del(ctx, refreshKey(oldToken))
	}

	pipe.Set(ctx, refreshKey(token), sessionValue(userID, deviceID), s.ttl)
	pipe.Set(ctx, deviceKey(userID, deviceID), token, 0)

	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}

	return token, nil
}

func (s *RedisSessionStore) Rotate(ctx context.Context, refreshToken string) (int64, string, string, error) {
	value, err := s.client.Get(ctx, refreshKey(refreshToken)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, "", "", errorx.NewCodeError(CodeUnauthorized, "invalid refresh token")
		}

		return 0, "", "", err
	}

	userID, deviceID, err := parseSessionValue(value)
	if err != nil {
		return 0, "", "", err
	}

	nextToken := uuid.NewString()
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, refreshKey(refreshToken))
	pipe.Set(ctx, refreshKey(nextToken), sessionValue(userID, deviceID), s.ttl)
	pipe.Set(ctx, deviceKey(userID, deviceID), nextToken, 0)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, "", "", err
	}

	return userID, deviceID, nextToken, nil
}

func (s *RedisSessionStore) RevokeDevice(ctx context.Context, userID int64, deviceID string) error {
	key := deviceKey(userID, deviceID)

	token, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}

		return err
	}

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, key)
	pipe.Del(ctx, refreshKey(token))
	_, err = pipe.Exec(ctx)

	return err
}

type JWTIssuer struct {
	secret string
	ttl    time.Duration
}

func NewJWTIssuer(secret string, ttl time.Duration) *JWTIssuer {
	return &JWTIssuer{secret: secret, ttl: ttl}
}

func (i *JWTIssuer) Issue(_ context.Context, userID int64, deviceID string) (string, int64, error) {
	return sharedjwt.NewManagerWithTTL(i.secret, i.ttl).GenerateAccessToken(userID, deviceID)
}

func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hashed), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func DefaultBotEmail(userID int64) string {
	return fmt.Sprintf("bot-%d@bots.aim.local", userID)
}

func IsDuplicateEmail(err error) bool {
	return errors.Is(err, dao.ErrDuplicateKey)
}

func IsNotFound(err error) bool {
	return errors.Is(err, dao.ErrNotFound)
}

func refreshKey(token string) string {
	return "auth:rt:" + token
}

func deviceKey(userID int64, deviceID string) string {
	return "auth:device:" + strconv.FormatInt(userID, 10) + ":" + deviceID
}

func sessionValue(userID int64, deviceID string) string {
	return strconv.FormatInt(userID, 10) + ":" + deviceID
}

func parseSessionValue(value string) (int64, string, error) {
	userPart, deviceID, ok := strings.Cut(value, ":")
	if !ok || deviceID == "" {
		return 0, "", errorx.NewCodeError(CodeInternal, "invalid refresh session")
	}

	userID, err := strconv.ParseInt(userPart, 10, 64)
	if err != nil || userID == 0 {
		return 0, "", errorx.NewCodeError(CodeInternal, "invalid refresh session")
	}

	return userID, deviceID, nil
}
