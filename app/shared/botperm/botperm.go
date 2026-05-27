// Package botperm defines action-based permission helpers for Bot OpenAPI.
package botperm

import (
	"strings"
	"unicode"

	"github.com/hellopoisonx/aim/app/shared/errorx"
)

const (
	ActionAll                            = "*"
	ActionBotAll                         = "bot.*"
	ActionSelfRead                       = "bot.self.read"
	ActionConversationList               = "bot.conversation.list"
	ActionMessageAll                     = "bot.message.*"
	ActionMessageSend                    = "bot.message.send"
	ActionWebhookRead                    = "bot.webhook.read"
	ActionWebhookWrite                   = "bot.webhook.write"
	ActionWebhookDelete                  = "bot.webhook.delete"
	ActionWebhookSubscribeAll            = "bot.webhook.subscribe.*"
	ActionWebhookSubscribeMessageCreated = "bot.webhook.subscribe.message_created"
	ActionAttachmentDownload             = "bot.attachment.download"
	WebhookEventMessageCreated           = "message.created"
)

// BuiltinActions mirrors the seed data in migration 009_bot_actions.sql.
var BuiltinActions = []string{
	ActionSelfRead,
	ActionConversationList,
	ActionMessageSend,
	ActionWebhookRead,
	ActionWebhookWrite,
	ActionWebhookDelete,
	ActionWebhookSubscribeMessageCreated,
	ActionAttachmentDownload,
	ActionAll,
	ActionBotAll,
	ActionMessageAll,
	ActionWebhookSubscribeAll,
}

// NormalizeGrant trims whitespace. Actions are case-sensitive; callers should
// store and compare the canonical lowercase action names defined here.
func NormalizeGrant(grant string) string {
	return strings.TrimSpace(grant)
}

// IsValidAction reports whether action is a syntactically valid Bot action or
// supported wildcard grant. Existence/enabled checks are database concerns.
func IsValidAction(action string) bool {
	action = NormalizeGrant(action)

	if action == ActionAll || action == ActionBotAll {
		return true
	}

	if action == "" || !strings.HasPrefix(action, "bot.") {
		return false
	}

	if strings.Contains(action, "..") || strings.HasSuffix(action, ".") {
		return false
	}

	parts := strings.Split(action, ".")
	if len(parts) < 3 {
		return false
	}

	for i, part := range parts {
		if part == "" {
			return false
		}

		if part == "*" {
			return i == len(parts)-1
		}

		for _, r := range part {
			if !unicode.IsLower(r) && !unicode.IsDigit(r) && r != '_' {
				return false
			}
		}
	}

	return true
}

// HasAction returns true if grants include action exactly or via supported
// wildcard grants such as *, bot.* or bot.message.*.
func HasAction(grants []string, action string) bool {
	action = NormalizeGrant(action)

	if !IsValidAction(action) || strings.HasSuffix(action, ".*") || action == ActionAll {
		return false
	}

	for _, grant := range grants {
		grant = NormalizeGrant(grant)
		if !IsValidAction(grant) {
			continue
		}

		if grant == ActionAll || grant == action {
			return true
		}

		if strings.HasSuffix(grant, ".*") {
			prefix := strings.TrimSuffix(grant, "*")
			if strings.HasPrefix(action, prefix) {
				return true
			}
		}
	}

	return false
}

// RequireAction returns CodeBotScopeDenied when grants do not authorize action.
func RequireAction(grants []string, action string) error {
	if HasAction(grants, action) {
		return nil
	}

	return errorx.NewCodeError(errorx.CodeBotScopeDenied, "token missing required action: "+action)
}
