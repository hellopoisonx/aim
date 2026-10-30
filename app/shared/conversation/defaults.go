package conversation

const (
	DefaultAvatar = "http://implement.me"

	SystemSenderName  = "system"
	SystemSenderEmail = "system@no.reply"
)

// Type is the closed set of conversations.conversation_type values.
type Type string

// Conversation types stored in conversations.conversation_type.
const (
	TypeDirect Type = "direct"
	TypeGroup  Type = "group"
)

// Role is a member role stored in conversation_members.role.
type Role string

// Member roles stored in conversation_members.role.
const (
	RoleMember Role = "member"
	RoleAdmin  Role = "admin"
	RoleOwner  Role = "owner"
)

// MessageTypeSystem marks messages authored by the system rather than a user.
// Message types are an open set (clients send text/image/... ), so only the
// system-authored marker is a named constant.
const MessageTypeSystem = "system"
