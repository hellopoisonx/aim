package service

import (
	"context"
	"errors"
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

	svc := NewConversationService(store, authz, testSnowflake, plainTxRunner{store: store})

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

	ownerRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 1), rbac.Domain(convID), sharedconversation.RoleOwner)
	require.NoError(t, err)
	assert.True(t, ownerRole, "creator must be assigned the owner role")

	memberRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 2), rbac.Domain(convID), sharedconversation.RoleMember)
	require.NoError(t, err)
	assert.True(t, memberRole, "invited member must be assigned the member role")
}

func TestConversationService_RBACRejectsMemberManagement(t *testing.T) {
	svc, _, _, convID := newRBACFixture(t)
	ctx := context.Background()

	_, err := svc.AddGroupMembers(ctx, convID, 2, []int64{4})
	require.ErrorIs(t, err, ErrNotAdmin)

	require.ErrorIs(t, svc.GrantGroupAdmin(ctx, convID, 2, 3), ErrNotOwner)
	_, err = svc.UpdateGroupInfo(ctx, convID, 2, nil, nil)
	require.ErrorIs(t, err, ErrNotAdmin)
	require.ErrorIs(t, svc.DismissGroup(ctx, convID, 2), ErrNotOwner)
	require.ErrorIs(t, svc.RevokeGroupAdmin(ctx, convID, 2, 3), ErrNotOwner)

	name := "renamed"
	_, err = svc.UpdateGroupInfo(ctx, convID, 1, &name, nil)
	require.NoError(t, err)
	require.NoError(t, svc.GrantGroupAdmin(ctx, convID, 1, 2))

	adminRole, err := svc.authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 2), rbac.Domain(convID), sharedconversation.RoleAdmin)
	require.NoError(t, err)
	assert.True(t, adminRole)

	memberRole, err := svc.authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 2), rbac.Domain(convID), sharedconversation.RoleMember)
	require.NoError(t, err)
	assert.False(t, memberRole, "member role must be replaced by admin")
}

func TestConversationService_RBACAdminCannotOwn(t *testing.T) {
	svc, _, _, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, svc.GrantGroupAdmin(ctx, convID, 1, 2))

	_, err := svc.AddGroupMembers(ctx, convID, 2, []int64{4})
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

	ownerRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 1), rbac.Domain(convID), sharedconversation.RoleOwner)
	require.NoError(t, err)
	assert.True(t, ownerRole)
}

func TestConversationService_RemoveMemberRevokesRole(t *testing.T) {
	svc, _, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, svc.RemoveGroupMembers(ctx, convID, 1, []int64{3}))

	hasRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 3), rbac.Domain(convID), sharedconversation.RoleMember)
	require.NoError(t, err)
	assert.False(t, hasRole, "removed member must lose its role")

	_, err = svc.AddGroupMembers(ctx, convID, 3, []int64{4})
	require.ErrorIs(t, err, ErrNotAdmin)
}

func TestConversationService_DismissClearsRoles(t *testing.T) {
	svc, _, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, svc.DismissGroup(ctx, convID, 1))

	for _, userID := range []int64{1, 2, 3} {
		role, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, userID), rbac.Domain(convID), sharedconversation.RoleOwner)
		require.NoError(t, err)
		assert.False(t, role, "user %d must have no role after dismissal", userID)
	}
}

// A lost assignment denies the owner (fail closed) and the next rebuild from
// conversation_members restores every member's rule.
func TestConversationService_LostRolesFailClosedThenRebuild(t *testing.T) {
	svc, _, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, authz.ClearConversation(ctx, rbac.Domain(convID)))

	name := "renamed"
	_, err := svc.UpdateGroupInfo(ctx, convID, 1, &name, nil)
	require.ErrorIs(t, err, ErrNotAdmin, "the owner lost its rule, so the guard rejects it")

	require.NoError(t, svc.syncRoles(ctx, convID))
	_, err = svc.UpdateGroupInfo(ctx, convID, 1, &name, nil)
	require.NoError(t, err)

	memberRole, err := authz.HasRole(ctx, mustSubject(t, shareduser.TypeHuman, 3), rbac.Domain(convID), sharedconversation.RoleMember)
	require.NoError(t, err)
	assert.True(t, memberRole, "post-commit rebuild restores every member's role")
}

// assignFailingAuthorizer delegates every read to a real enforcer but cannot
// persist assignments, the failure the role rebuild has to report.
type assignFailingAuthorizer struct {
	rbac.Authorizer
	err error
}

func (a assignFailingAuthorizer) AssignRole(context.Context, string, string, sharedconversation.Role) error {
	return a.err
}

// A rebuild that cannot write rules must deny the conversation instead of
// leaving the revoked role in force.
func TestConversationService_RoleSyncFailureFailsClosed(t *testing.T) {
	svc, _, authz, convID := newRBACFixture(t)
	ctx := context.Background()

	require.NoError(t, svc.GrantGroupAdmin(ctx, convID, 1, 3))
	svc.authz = assignFailingAuthorizer{Authorizer: authz, err: errors.New("authorizer unavailable")}

	require.Error(t, svc.RevokeGroupAdmin(ctx, convID, 1, 3))

	dom := rbac.Domain(convID)
	allowed, err := authz.Authorize(ctx, mustSubject(t, shareduser.TypeHuman, 3), dom, rbac.ObjectConversation, rbac.ActionGroupManage)
	require.NoError(t, err)
	assert.False(t, allowed, "a failed rebuild must not keep the revoked admin authorized")

	ownerAllowed, err := authz.Authorize(ctx, mustSubject(t, shareduser.TypeHuman, 1), dom, rbac.ObjectConversation, rbac.ActionGroupManage)
	require.NoError(t, err)
	assert.False(t, ownerAllowed, "clear-then-assign leaves no rule when every write fails")
}
