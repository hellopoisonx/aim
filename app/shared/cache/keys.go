package cache

import (
	"fmt"
	"strings"
)

const (
	NameConversation        = "conv"
	NameConversationMembers = "conv_members"
	NameUser                = "user"
	NameFriendship          = "friendship"
	NameUserType            = "user_type"
	NameBotToken            = "bot_token"
	NamePresence            = "presence"
)

func ConvKey(id int64) string {
	return fmt.Sprintf("conv:%d", id)
}

func ConvMembersKey(conversationID int64) string {
	return fmt.Sprintf("conv:members:%d", conversationID)
}

func UserKey(id int64) string {
	return fmt.Sprintf("user:%d", id)
}

func UserTypeKey(id int64) string {
	return fmt.Sprintf("user:type:%d", id)
}

func UserEmailKey(email string) string {
	return fmt.Sprintf("user:email:%s", strings.ToLower(strings.TrimSpace(email)))
}

func FriendshipKey(uid1, uid2 int64) string {
	if uid1 > uid2 {
		uid1, uid2 = uid2, uid1
	}

	return fmt.Sprintf("friend:%d:%d", uid1, uid2)
}

func BotTokenKey(hash string) string {
	return fmt.Sprintf("bot:token:%s", hash)
}

func PresenceKey(userID int64) string {
	return fmt.Sprintf("presence:%d", userID)
}
