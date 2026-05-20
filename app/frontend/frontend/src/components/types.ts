/**
 * Local TypeScript interfaces for AIM desktop chat workspace components.
 * These are decoupled from Wails generated types — components never call
 * Go bindings directly; they receive data via props and emit events.
 */

export interface Conversation {
  id: number
  title: string
  avatar?: string
  lastMessage?: string
  lastMessageAt?: string
  unreadCount: number
  isOnline: boolean
}

export interface ChatMessage {
  id: number
  conversationId: number
  senderId: number
  senderName: string
  senderAvatar?: string
  content: string
  timestamp: string
  isMine: boolean
  clientMsgId?: string
}

/** Search user result item, derived from client.UserListItem */
export interface SearchUserItem {
  id: number
  email: string
  avatar?: string
}
