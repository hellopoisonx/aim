// Package rbac decides conversation permissions with Casbin.
//
// Identity model:
//
//	subject  = "user:<id>" | "bot:<id>"   (a bot is never an owner)
//	domain   = "conv:<conversation id>"
//	object   = "conversation"
//	actions  = group.own | group.manage | message.send
//
// Role assignments (g rules) are persisted; the static policy (p) rules live in
// memory and are reseeded on every start, so the store only ever holds
// assignments.
package rbac

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"

	shareduser "github.com/hellopoisonx/aim/app/shared/user"
)

const rolePrefix = "role:"

const (
	subjectOwner  = rolePrefix + "owner"
	subjectAdmin  = rolePrefix + "admin"
	subjectMember = rolePrefix + "member"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"

	ObjectConversation = "conversation"
	ActionGroupOwn     = "group.own"
	ActionGroupManage  = "group.manage"
	ActionMessageSend  = "message.send"
)

// modelDefinition is the Casbin model: roles are scoped per conversation
// domain, and a policy rule applies when its domain is the requested one or
// the wildcard "*".
const modelDefinition = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && (p.dom == "*" || p.dom == r.dom) && r.obj == p.obj && r.act == p.act
`

// staticRules is the permission matrix. The default is deny, so an admin
// simply has no group.own rule.
var staticRules = [][]string{
	{subjectOwner, "*", ObjectConversation, ActionGroupOwn},
	{subjectOwner, "*", ObjectConversation, ActionGroupManage},
	{subjectAdmin, "*", ObjectConversation, ActionGroupManage},
	{subjectOwner, "*", ObjectConversation, ActionMessageSend},
	{subjectAdmin, "*", ObjectConversation, ActionMessageSend},
	{subjectMember, "*", ObjectConversation, ActionMessageSend},
}

// Authorizer is the conversation permission surface used by the logic services.
type Authorizer interface {
	// Authorize reports whether sub may perform act on obj in dom.
	Authorize(ctx context.Context, sub, dom, obj, act string) (bool, error)
	// AssignRole grants role to sub in dom.
	AssignRole(ctx context.Context, sub, dom, role string) error
	// Unassign removes every role of sub in dom.
	Unassign(ctx context.Context, sub, dom string) error
	// ChangeRole moves sub from one role to another in dom.
	ChangeRole(ctx context.Context, sub, dom, fromRole, toRole string) error
	// ClearConversation removes every assignment in dom.
	ClearConversation(ctx context.Context, dom string) error
	// HasRole reports whether sub holds role in dom.
	HasRole(ctx context.Context, sub, dom, role string) (bool, error)
}

// Subject maps a validated user type to its Casbin subject, rejecting values
// outside the closed set instead of silently treating them as human.
func Subject(userType shareduser.Type, userID int64) (string, error) {
	switch userType {
	case shareduser.TypeHuman:
		return "user:" + strconv.FormatInt(userID, 10), nil
	case shareduser.TypeBot:
		return "bot:" + strconv.FormatInt(userID, 10), nil
	}

	return "", fmt.Errorf("rbac: unknown user type %q", userType)
}

func Domain(conversationID int64) string {
	return "conv:" + strconv.FormatInt(conversationID, 10)
}

func RoleSubject(role string) string {
	return rolePrefix + role
}

// Enforcer is the Casbin-backed Authorizer.
//
// Enforce holds a read lock: rule mutations rewrite model slices that a
// concurrent Enforce iterates.
type Enforcer struct {
	enforcer *casbin.Enforcer
	mu       sync.RWMutex
}

// New builds the enforcer and loads the persisted assignments through adapter.
func New(adapter persist.Adapter) (*Enforcer, error) {
	m, err := casbinmodel.NewModelFromString(modelDefinition)
	if err != nil {
		return nil, fmt.Errorf("rbac: build model: %w", err)
	}

	params := []any{m}
	if adapter != nil {
		params = append(params, adapter)
	}

	enforcer, err := casbin.NewEnforcer(params...)
	if err != nil {
		return nil, fmt.Errorf("rbac: create enforcer: %w", err)
	}

	// The static matrix is memory-only: persisting it would duplicate what every
	// start reseeds and would let a stale row outlive a policy change.
	enforcer.EnableAutoSave(false)
	if _, err := enforcer.AddPolicies(staticRules); err != nil {
		return nil, fmt.Errorf("rbac: seed static policy: %w", err)
	}
	enforcer.EnableAutoSave(true)

	return &Enforcer{enforcer: enforcer}, nil
}

func (a *Enforcer) Authorize(_ context.Context, sub, dom, obj, act string) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	allowed, err := a.enforcer.Enforce(sub, dom, obj, act)
	if err != nil {
		return false, fmt.Errorf("rbac: enforce %s %s %s %s: %w", sub, dom, obj, act, err)
	}

	return allowed, nil
}

func (a *Enforcer) AssignRole(_ context.Context, sub, dom, role string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.enforcer.AddGroupingPolicy(sub, RoleSubject(role), dom); err != nil {
		return fmt.Errorf("rbac: assign %s to %s in %s: %w", role, sub, dom, err)
	}

	return nil
}

func (a *Enforcer) Unassign(_ context.Context, sub, dom string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Field order of a g rule is sub, role, domain; an empty value matches any.
	if _, err := a.enforcer.RemoveFilteredGroupingPolicy(0, sub, "", dom); err != nil {
		return fmt.Errorf("rbac: unassign %s in %s: %w", sub, dom, err)
	}

	return nil
}

func (a *Enforcer) ChangeRole(ctx context.Context, sub, dom, fromRole, toRole string) error {
	if fromRole == toRole {
		return nil
	}
	if err := a.Unassign(ctx, sub, dom); err != nil {
		return err
	}

	return a.AssignRole(ctx, sub, dom, toRole)
}

func (a *Enforcer) ClearConversation(_ context.Context, dom string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := a.enforcer.RemoveFilteredGroupingPolicy(2, dom); err != nil {
		return fmt.Errorf("rbac: clear domain %s: %w", dom, err)
	}

	return nil
}

func (a *Enforcer) HasRole(_ context.Context, sub, dom, role string) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ok, err := a.enforcer.HasGroupingPolicy(sub, RoleSubject(role), dom)
	if err != nil {
		return false, fmt.Errorf("rbac: has role %s for %s in %s: %w", role, sub, dom, err)
	}

	return ok, nil
}
