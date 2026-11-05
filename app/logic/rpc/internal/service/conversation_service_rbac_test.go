package service

import (
	"context"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/service/rbac"
	"github.com/hellopoisonx/aim/app/logic/rpc/internal/servicefakes"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	shareduser "github.com/hellopoisonx/aim/app/shared/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRBACFixture builds a group conversation with creator 1 (owner), member 2
// and member 3, authorized by a real in-memory Casbin enforcer.
func newRBACFixture(t *testing.T) (*ConversationService, *servicefakes.ConversationStore, *rbac.Enforcer, int64) {
	t.Helper()

	ctx := context.Background()
	store := servicefakes.NewConversationStore()
	authz, err := rbac.New(nil)
	require.NoError(t, err)

	svc := NewConversationService(store, testSnowflake, plainTxRunner{store: store}, WithRBAC(authz))

	conv, err := svc.CreateConversation(ctx, sharedconversation.TypeGroup, 1, []int64{1, 2, 3}, "group", "")
	require.NoError(t, err)

	return svc, store, authz, conv.ID
}

func mustSubject(t *testing.T, userType shareduser.Type, userID int64) string {
	t.Helper()
	sub, err := rbac.Subject(userType, userID)
	require.NoError(t, err)

	return sub
}

func TestConversationService_RBACRoleAssignmentsOnCreate(t *testing.T) {
	_, _, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	ownerRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 1), rbac.Domain(convID), rbac.RoleOwner)
	require.NoError(t, err)
	assert.True(t, ownerRole, "creator must be assigned the owner role")

	memberRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 2), rbac.Domain(convID), rbac.RoleMember)
	require.NoError(t, err)
	assert.True(t, memberRole, "invited member must be assigned the member role")
}

func TestConversationService_RBACRejectsMemberManagement(t *testing.T) {
	svc, _, _, convID := newRBACFixture(t)
	ctx := context.Background()

	_, err := svc.AddGroupMembers(ctx, convID, 2, "bob", []int64{4})
	require.ErrorIs(t, err, ErrNotAdmin)

	require.ErrorIs(t, svc.GrantGroupAdmin(ctx, convID, 2, 3), ErrNotOwner)
	_, err = svc.UpdateGroupInfo(ctx, convID, 2, "bob", nil, nil)
	require.ErrorIs(t, err, ErrNotAdmin)
	require.ErrorIs(t, svc.DismissGroup(ctx, convID, 2), ErrNotOwner)
	require.ErrorIs(t, svc.RevokeGroupAdmin(ctx, convID, 2, 3), ErrNotOwner)

	name := "renamed"
	_, err = svc.UpdateGroupInfo(ctx, convID, 1, "alice", &name, nil)
	require.NoError(t, err)
	require.NoError(t, svc.GrantGroupAdmin(ctx, convID, 1, 2))

	adminRole, err := svc.authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 2), rbac.Domain(convID), rbac.RoleAdmin)
	require.NoError(t, err)
	assert.True(t, adminRole)

	memberRole, err := svc.authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 2), rbac.Domain(convID), rbac.RoleMember)
	require.NoError(t, err)
	assert.False(t, memberRole, "member role must be replaced by admin")
}

func TestConversationService_RBACAdminCannotOwn(t *testing.T) {
	svc, _, _, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, svc.GrantGroupAdmin(ctx, convID, 1, 2))

	_, err := svc.AddGroupMembers(ctx, convID, 2, "bob", []int64{4})
	require.NoError(t, err)

	_, err = svc.TransferGroupOwner(ctx, convID, 2, 3)
	require.ErrorIs(t, err, ErrNotOwner)
	require.ErrorIs(t, svc.DismissGroup(ctx, convID, 2), ErrNotOwner)
}

func TestConversationService_TransferGroupOwnerRejectsBot(t *testing.T) {
	svc, store, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	store.BotUserIDs[3] = struct{}{}

	_, err := svc.TransferGroupOwner(ctx, convID, 1, 3)
	require.Error(t, err)

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, errorx.CodeBadInput, codeErr.Code)

	ownerRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 1), rbac.Domain(convID), rbac.RoleOwner)
	require.NoError(t, err)
	assert.True(t, ownerRole)
}

func TestConversationService_RemoveMemberRevokesRole(t *testing.T) {
	svc, _, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, svc.RemoveGroupMembers(ctx, convID, 1, "alice", []int64{3}))

	hasRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 3), rbac.Domain(convID), rbac.RoleMember)
	require.NoError(t, err)
	assert.False(t, hasRole, "removed member must lose its role")

	_, err = svc.AddGroupMembers(ctx, convID, 3, "carol", []int64{4})
	require.ErrorIs(t, err, ErrNotAdmin)
}

func TestConversationService_DismissClearsRoles(t *testing.T) {
	svc, _, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, svc.DismissGroup(ctx, convID, 1))

	for _, userID := range []int64{1, 2, 3} {
		role, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, userID), rbac.Domain(convID), rbac.RoleOwner)
		require.NoError(t, err)
		assert.False(t, role, "user %d must have no role after dismissal", userID)
	}
}
