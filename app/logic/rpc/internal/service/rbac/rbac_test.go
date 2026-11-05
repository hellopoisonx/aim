package rbac

import (
	"context"
	"testing"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
)

type memoryAdapter struct {
	rules [][]string // {ptype, sub, role, dom}
}

func (a *memoryAdapter) LoadPolicy(m casbinmodel.Model) error {
	for _, rule := range a.rules {
		if err := persist.LoadPolicyArray(rule, m); err != nil {
			return err
		}
	}
	return nil
}

func (a *memoryAdapter) SavePolicy(casbinmodel.Model) error { return nil }

func (a *memoryAdapter) AddPolicy(_ string, ptype string, rule []string) error {
	a.rules = append(a.rules, append([]string{ptype}, rule...))
	return nil
}

func (a *memoryAdapter) RemovePolicy(_ string, ptype string, rule []string) error {
	return a.RemoveFilteredPolicy("", ptype, 0, append([]string{""}, rule...)[1:]...)
}

func (a *memoryAdapter) RemoveFilteredPolicy(_ string, ptype string, fieldIndex int, fieldValues ...string) error {
	kept := a.rules[:0]
	for _, row := range a.rules {
		if row[0] != ptype || !matchesFilter(row[1:], fieldIndex, fieldValues) {
			kept = append(kept, row)
		}
	}
	a.rules = kept
	return nil
}

func matchesFilter(rule []string, fieldIndex int, fieldValues []string) bool {
	for i, value := range fieldValues {
		column := fieldIndex + i
		if value == "" {
			continue
		}
		if column >= len(rule) || rule[column] != value {
			return false
		}
	}
	return true
}

func newTestEnforcer(t *testing.T, adapter persist.Adapter) *Enforcer {
	t.Helper()

	enforcer, err := New(adapter)
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}
	return enforcer
}

func TestAuthorizeMatrix(t *testing.T) {
	ctx := context.Background()
	enforcer := newTestEnforcer(t, nil)

	conv := Domain(42)
	owner := Subject("human", 1)
	admin := Subject("human", 2)
	member := Subject("human", 3)
	stranger := Subject("human", 4)

	for sub, role := range map[string]string{owner: RoleOwner, admin: RoleAdmin, member: RoleMember} {
		if err := enforcer.AssignRole(ctx, sub, conv, role); err != nil {
			t.Fatalf("assign %s: %v", role, err)
		}
	}

	cases := []struct {
		sub  string
		act  string
		want bool
	}{
		{owner, ActionGroupOwn, true},
		{owner, ActionGroupManage, true},
		{owner, ActionMessageSend, true},
		{admin, ActionGroupOwn, false},
		{admin, ActionGroupManage, true},
		{admin, ActionMessageSend, true},
		{member, ActionGroupOwn, false},
		{member, ActionGroupManage, false},
		{member, ActionMessageSend, true},
		{stranger, ActionMessageSend, false},
		{stranger, ActionGroupManage, false},
	}

	for _, tc := range cases {
		allowed, err := enforcer.Authorize(ctx, tc.sub, conv, ObjectConversation, tc.act)
		if err != nil {
			t.Fatalf("authorize %s %s: %v", tc.sub, tc.act, err)
		}
		if allowed != tc.want {
			t.Errorf("authorize %s %s = %v, want %v", tc.sub, tc.act, allowed, tc.want)
		}
	}
}

func TestAuthorizeIsDomainScoped(t *testing.T) {
	ctx := context.Background()
	enforcer := newTestEnforcer(t, nil)

	member := Subject("human", 7)
	if err := enforcer.AssignRole(ctx, member, Domain(1), RoleMember); err != nil {
		t.Fatalf("assign: %v", err)
	}

	allowed, err := enforcer.Authorize(ctx, member, Domain(2), ObjectConversation, ActionMessageSend)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if allowed {
		t.Fatal("role in conv:1 must not grant anything in conv:2")
	}
}

func TestBotSubjectUsesSameMatrix(t *testing.T) {
	ctx := context.Background()
	enforcer := newTestEnforcer(t, nil)

	bot := Subject("bot", 9)
	if err := enforcer.AssignRole(ctx, bot, Domain(5), RoleAdmin); err != nil {
		t.Fatalf("assign: %v", err)
	}

	manage, err := enforcer.Authorize(ctx, bot, Domain(5), ObjectConversation, ActionGroupManage)
	if err != nil {
		t.Fatalf("authorize admin: %v", err)
	}
	if !manage {
		t.Error("bot admin must be able to group.manage")
	}

	own, err := enforcer.Authorize(ctx, bot, Domain(5), ObjectConversation, ActionGroupOwn)
	if err != nil {
		t.Fatalf("authorize own: %v", err)
	}
	if own {
		t.Error("bot admin must not be able to group.own")
	}
}

func TestRoleMutationPersistsThroughAdapter(t *testing.T) {
	ctx := context.Background()
	adapter := &memoryAdapter{}
	enforcer := newTestEnforcer(t, adapter)

	conv := Domain(11)
	sub := Subject("human", 3)

	if err := enforcer.AssignRole(ctx, sub, conv, RoleMember); err != nil {
		t.Fatalf("assign member: %v", err)
	}
	if err := enforcer.ChangeRole(ctx, sub, conv, RoleMember, RoleAdmin); err != nil {
		t.Fatalf("change role: %v", err)
	}

	hasAdmin, err := enforcer.HasRole(ctx, sub, conv, RoleAdmin)
	if err != nil {
		t.Fatalf("has admin: %v", err)
	}
	if !hasAdmin {
		t.Error("expected admin role after change")
	}
	hasMember, err := enforcer.HasRole(ctx, sub, conv, RoleMember)
	if err != nil {
		t.Fatalf("has member: %v", err)
	}
	if hasMember {
		t.Error("member role must be gone after change")
	}

	// A fresh enforcer over the same store must see the same assignment.
	reloaded := newTestEnforcer(t, adapter)
	allowed, err := reloaded.Authorize(ctx, sub, conv, ObjectConversation, ActionGroupManage)
	if err != nil {
		t.Fatalf("authorize reloaded: %v", err)
	}
	if !allowed {
		t.Error("reloaded enforcer lost the persisted role")
	}

	if err := enforcer.Unassign(ctx, sub, conv); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if len(adapter.rules) != 0 {
		t.Errorf("adapter still holds %d rules after unassign", len(adapter.rules))
	}

	if err := enforcer.AssignRole(ctx, sub, conv, RoleOwner); err != nil {
		t.Fatalf("re-assign owner: %v", err)
	}
	if err := enforcer.ClearConversation(ctx, conv); err != nil {
		t.Fatalf("clear conversation: %v", err)
	}
	if len(adapter.rules) != 0 {
		t.Errorf("adapter still holds %d rules after clear", len(adapter.rules))
	}
}
