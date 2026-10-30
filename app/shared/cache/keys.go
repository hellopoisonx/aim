package cache

import (
	"fmt"
	"strings"
)

// Key prefixes. Every builder below composes "<prefix>:<id>".
const (
	prefixConversation = "conv"
	prefixUser        = "user"
	prefixFriendship  = "friend"
	prefixBot         = "bot"
	prefixPresence    = "presence"
	prefixWsReplay    = "ws"
)

const (
	NameConversation        = "conv"
	NameConversationMembers = "conv_members"
	NameUser                = "user"
	NameFriendship          = "friendship"
	NameUserType            = "user_type"
	NameBotToken            = "bot_token"
	NamePresence            = "presence"
	NameWsReplay            = "ws_replay"
)

// ConversationKey is the Redis/L1 key of a conversation row.
func ConversationKey(conversationID int64) string {
	return fmt.Sprintf("%s:%d", prefixConversation, conversationID)
}

// ConversationMembersKey is the Redis/L1 key of a conversation's member list.
func ConversationMembersKey(conversationID int64) string {
	return fmt.Sprintf("%s:members:%d", prefixConversation, conversationID)
}

// UserKey is the Redis/L1 key of a user profile.
func UserKey(userID int64) string {
	return fmt.Sprintf("%s:%d", prefixUser, userID)
}

// UserTypeKey is the Redis/L1 key of a user's type (human/bot/system).
func UserTypeKey(userID int64) string {
	return fmt.Sprintf("%s:type:%d", prefixUser, userID)
}

// UserEmailKey is the Redis/L1 key of the email → user lookup.
func UserEmailKey(email string) string {
	return fmt.Sprintf("%s:email:%s", prefixUser, strings.ToLower(strings.TrimSpace(email)))
}

// FriendshipKey is the order-independent key of a friendship pair.
func FriendshipKey(userID1, userID2 int64) string {
	if userID1 > userID2 {
		userID1, userID2 = userID2, userID1
	}

	return fmt.Sprintf("%s:%d:%d", prefixFriendship, userID1, userID2)
}

// BotTokenKey is the Redis/L1 key of a bot token lookup by hash.
func BotTokenKey(tokenHash string) string {
	return fmt.Sprintf("%s:token:%s", prefixBot, tokenHash)
}

// PresenceKey is the Redis/L1 key of a user's presence set.
func PresenceKey(userID int64) string {
	return fmt.Sprintf("%s:%d", prefixPresence, userID)
}

// WsReplayKey locates the gateway WebSocket replay queue of one device.
func WsReplayKey(userID int64, deviceID string) string {
	return fmt.Sprintf("%s:replay:%d:%s", prefixWsReplay, userID, deviceID)
}
