package conversation

const (
	DefaultAvatar = "http://implement.me"

	SystemSenderName  = "system"
	SystemSenderEmail = "system@no.reply"
)

// Conversation types stored in conversations.conversation_type.
const (
	TypeDirect = "direct"
	TypeGroup  = "group"
)

// Member roles stored in conversation_members.role.
const (
	RoleMember = "member"
	RoleAdmin  = "admin"
	RoleOwner  = "owner"
)

// MessageTypeSystem marks messages authored by the system rather than a user.
const MessageTypeSystem = "system"
