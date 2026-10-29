package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/auth/rpc/model/dao"
	"github.com/hellopoisonx/aim/app/auth/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/shared/jwt"
	"github.com/hellopoisonx/aim/app/shared/tools"
	"github.com/stretchr/testify/require"
)

var errQuery = errors.New("query failed")

func TestPasswordAndEmailHelpers(t *testing.T) {
	require.Equal(t, "ada@example.com", NormalizeEmail(" Ada@Example.COM "))

	hash, err := HashPassword("password123")
	require.NoError(t, err)
	require.NotEqual(t, "password123", hash)
	require.True(t, CheckPassword(hash, "password123"))
	require.False(t, CheckPassword(hash, "wrong-password"))
}

func TestJWTIssuerIssuesValidToken(t *testing.T) {
	issuer := NewJWTIssuer("test-secret", 5*time.Minute)
	token, expiresAt, err := issuer.Issue(context.Background(), 9, "device-1")
	require.NoError(t, err)
	require.NotZero(t, expiresAt)

	claims, err := jwt.NewManager("test-secret").ValidateAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, int64(9), claims.UserID)
	require.Equal(t, "device-1", claims.DeviceID)
}

func TestParseSessionValueRejectsInvalidData(t *testing.T) {
	_, _, err := parseSessionValue("broken")
	require.Error(t, err)

	_, _, err = parseSessionValue("x:device")
	require.Error(t, err)
}

func TestDAOUserStore(t *testing.T) {
	fake := &fakeUserDAO{user: orm.UserCredential{ID: 11, Email: "ada@example.com", PasswordHash: "hash", Name: "Ada", Status: StatusNormal}}
	ids, err := tools.NewSnowflake(1)
	require.NoError(t, err)
	store := NewDAOUserStoreWithIDGenerator(fake, ids)

	created, err := store.CreateUser(context.Background(), "ada@example.com", "hash", "Ada")
	require.NoError(t, err)
	require.Positive(t, fake.createdID)
	require.Equal(t, "ada@example.com", fake.email)
	require.Equal(t, "hash", fake.passwordHash)
	require.Equal(t, "Ada", fake.name)
	require.Equal(t, int64(11), created.ID)
	require.Equal(t, "ada@example.com", created.Email)
	require.Equal(t, "Ada", created.Name)

	found, err := store.GetUserByEmail(context.Background(), "ada@example.com")
	require.NoError(t, err)
	require.Equal(t, int64(11), found.ID)
}

func TestDAOUserStorePropagatesQueryErrors(t *testing.T) {
	fake := &fakeUserDAO{err: errQuery}
	ids, err := tools.NewSnowflake(1)
	require.NoError(t, err)
	store := NewDAOUserStoreWithIDGenerator(fake, ids)

	_, err = store.CreateUser(context.Background(), "ada@example.com", "hash", "Ada")
	require.ErrorIs(t, err, errQuery)

	_, err = store.GetUserByEmail(context.Background(), "ada@example.com")
	require.ErrorIs(t, err, errQuery)
}

func TestDatabaseErrorHelpers(t *testing.T) {
	require.True(t, IsDuplicateEmail(dao.ErrDuplicateKey))
	require.False(t, IsDuplicateEmail(errors.New("other")))
	require.True(t, IsNotFound(dao.ErrNotFound))
	require.False(t, IsNotFound(errors.New("other")))
}

type fakeUserDAO struct {
	user         orm.UserCredential
	err          error
	createdID    int64
	email        string
	passwordHash string
	name         string
}

func (f *fakeUserDAO) CreateUser(_ context.Context, id int64, email, passwordHash, name string) (orm.UserCredential, error) {
	f.createdID = id
	f.email = email
	f.passwordHash = passwordHash
	f.name = name
	if f.err != nil {
		return orm.UserCredential{}, f.err
	}
	return f.user, nil
}

func (f *fakeUserDAO) CreateBotCredential(_ context.Context, id int64, email, passwordHash, name string) (orm.UserCredential, error) {
	f.createdID = id
	f.email = email
	f.passwordHash = passwordHash
	f.name = name
	if f.err != nil {
		return orm.UserCredential{}, f.err
	}
	return f.user, nil
}

func (f *fakeUserDAO) GetUserByEmail(_ context.Context, email string) (orm.UserCredential, error) {
	f.email = email
	if f.err != nil {
		return orm.UserCredential{}, f.err
	}
	return f.user, nil
}
