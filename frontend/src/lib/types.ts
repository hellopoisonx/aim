export type EntityId = string

export interface SenderInfo {
  name: string
  email: string
}

export interface UserInfo {
  id: EntityId
  email: string
  status: number
  nickname: string
  avatar: string
  created_at: number
  updated_at: number
}

export interface UserListItem {
  id: EntityId
  nickname: string
  email: string
  avatar: string
}

export interface FriendTagItem {
  id: EntityId
  user_id: EntityId
  name: string
  created_at: number
  updated_at: number
}

export interface FriendshipItem {
  user_id: EntityId
  friend_id: EntityId
  status: string
  created_at: number
  updated_at: number
  name?: string
  email?: string
  avatar?: string
  tags?: FriendTagItem[]
}

export interface MessageReadDetailItem {
  user_id: EntityId
  is_read: boolean
  last_read_message_id: EntityId
  updated_at: number
  email: string
  avatar: string
  name: string
}

export interface ReadStateItem {
  user_id: EntityId
  last_read_message_id: EntityId
  updated_at: number
  email?: string
  avatar?: string
  name?: string
}

export interface MessageItem {
  id: EntityId
  conversation_id: EntityId
  sender_id: EntityId
  sender_info: SenderInfo
  message_type: string
  content: string
  client_msg_id: string
  created_at: number
  is_system?: boolean
  mentions?: string[]
  read_details: MessageReadDetailItem[]
}

export type LocalDeliveryStatus = "received" | "sending" | "sent" | "failed"

export interface ChatMessage extends MessageItem {
  local_status: LocalDeliveryStatus
  error_msg?: string
}

export interface ConversationItem {
  conversation_id: EntityId
  conversation_type: "direct" | "group" | string
  is_active: boolean
  created_at: number
  member_ids: EntityId[]
  name: string
  avatar: string
  creator_id: EntityId
}

export interface MemberDetailItem {
  user_id: EntityId
  email: string
  avatar: string
  role: "owner" | "admin" | "member" | string
  joined_at: number
  name?: string
}

export interface PresenceItem {
  user_id: EntityId
  status: "online" | "offline" | string
  updated_at: number
  name?: string
}

export interface InitAttachmentUploadRequest {
  conversation_id: EntityId
  kind: "image" | "video" | "audio" | "file"
  original_name: string
  mime: string
  size: number
  sha256?: string
}

export interface InitAttachmentUploadResponse {
  file_id: string
  bucket: string
  object_key: string
  upload_url: string
  upload_method: string
  headers?: Record<string, string>
  expires_at: number
}

export interface AttachmentFileInfo {
  file_id: string
  owner_id: EntityId
  conversation_id: EntityId
  kind: "image" | "video" | "audio" | "file" | string
  original_name: string
  mime: string
  size: number
  sha256?: string
  status: string
  parse_status: string
  bucket: string
  object_key: string
  thumbnail_object_key?: string
  duration_ms?: number
  width?: number
  height?: number
  metadata?: Record<string, unknown>
}

export interface DownloadAttachmentResponse {
  url: string
  headers?: Record<string, string>
  expires_at: number
}

export interface AttachmentContentV1 {
  schema: "aim.attachment.v1"
  file_id: string
  kind: "image" | "video" | "audio" | "file" | string
  original: {
    name: string
    mime: string
    size: number
    sha256?: string
  }
  thumbnail_file_id?: string
  parse_status?: string
  duration_ms?: number
  width?: number
  height?: number
  metadata?: Record<string, unknown>
}

export interface SystemMessageEvent {
  event: string
  operator_id?: EntityId
  target_user_ids?: EntityId[]
  name?: string
  avatar?: string
}

export interface SearchUserResultItem {
  user: UserInfo
  snippet: string
}

export interface SearchFriendResultItem {
  friendship: FriendshipItem
  user: UserInfo
  snippet: string
}

export interface SearchConversationResultItem {
  conversation: ConversationItem
  snippet: string
}

export interface SearchMessageResultItem {
  message: MessageItem
  snippet: string
}

export interface SearchResponse {
  users: SearchUserResultItem[]
  friends: SearchFriendResultItem[]
  conversations: SearchConversationResultItem[]
  messages: SearchMessageResultItem[]
  next_cursor_created_at: number
  next_cursor_id: EntityId
  has_more: boolean
}

export interface AuthSession {
  userId: EntityId
  accessToken: string
  refreshToken: string
  expiresAt: number
  deviceId: string
  email?: string
}

export interface SystemNotificationItem {
  id: string
  notification_type: string
  title: string
  body: string
  related_id: EntityId
  received_at: number
}
