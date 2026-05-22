<script lang="ts" setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  AddFriend,
  ConnectWS,
  CreateDirectConversation,
  DeviceID,
  DisconnectWS,
  GetConversationHistory,
  GetUserById,
  ListConversations,
  ListFriendApplications,
  Login,
  Logout,
  Refresh,
  Register,
  SearchUsersByName,
  SendAck,
  SendHeartbeat,
  SendMessage,
  SendReadReceipt,
  SendTyping,
  SessionState,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import type { client, main, vueapi } from '../wailsjs/go/models'
import type { ChatMessage, Conversation, SearchUserItem } from './components/types'
import LoginView from './views/LoginView.vue'
import RegisterView from './views/RegisterView.vue'
import FriendsView from './views/FriendsView.vue'
import ConversationList from './components/ConversationList.vue'
import GroupSettingsPanel from './components/GroupSettingsPanel.vue'
import MessageArea from './components/MessageArea.vue'
import MessageInput from './components/MessageInput.vue'
import { formatSystemMessage, isSystemMessageType } from './utils/systemMessage'

// ─── State ───────────────────────────────────────────────────────────────────

type AuthView = 'login' | 'register'
type ConnectionState = 'connecting' | 'connected' | 'disconnected'

const authView = ref<AuthView>('login')
const authLoading = ref(false)
const deviceId = ref('')
const currentUserId = ref<string>('')
const currentUserLabel = ref('')
const connectionState = ref<ConnectionState>('disconnected')

const conversations = ref<Conversation[]>([])
const activeConversationId = ref<string | null>(null)
const messagesMap = ref<Map<string, ChatMessage[]>>(new Map())
const historyLoadedSet = ref<Set<string>>(new Set())
const historyLoading = ref(false)

// ─── User search / direct conversation ────────────────────────────────────────
const searchKeyword = ref('')
const searchResults = ref<SearchUserItem[]>([])
const searchLoading = ref(false)
const createLoading = ref(false)
const creatingUserId = ref<string | null>(null)
const addFriendLoading = ref(false)
const addingFriendUserId = ref<string | null>(null)
const searchError = ref('')

// ─── Friends view ─────────────────────────────────────────────────────────────
const friendsViewVisible = ref(false)
const friendsInitialTab = ref<'friends' | 'applications' | 'create-group'>('friends')
const pendingFriendAppCount = ref(0)
const groupSettingsVisible = ref(false)

// ─── Presence / Typing / Heartbeat ────────────────────────────────────────────
const onlineUserIds = ref<Set<string>>(new Set())
// `typingInfo` is now a Map keyed by conversationId to support multiple simultaneous typing indicators.
const typingInfo = ref<Map<string, { userId: string; timer: ReturnType<typeof setTimeout> }>>(new Map())
const lastReadSeq = ref(0)
let heartbeatTimer: ReturnType<typeof setInterval> | null = null
// TYPING_TIMEOUT_MS mirrors the 4s clear timeout used on receipt of PUSH_TYPING.
const TYPING_TIMEOUT_MS = 4000

// ─── Auto-reconnect ───────────────────────────────────────────────────────────
const intentionalDisconnect = ref(false)
let reconnectAttempt = 0
const MAX_RECONNECT_ATTEMPTS = 5
let reconnectTimer: ReturnType<typeof setTimeout> | null = null

// ─── Derived helpers ──────────────────────────────────────────────────────────

const activeConversation = computed<Conversation | null>(() => {
  if (activeConversationId.value === null) return null
  return conversations.value.find((c) => c.id === activeConversationId.value) ?? null
})

const activeMessages = computed<ChatMessage[]>(() => {
  if (activeConversationId.value === null) return []
  return messagesMap.value.get(activeConversationId.value) ?? []
})

/** The user ID currently typing in the active conversation, or null */
const typingUserId = computed<string | null>(() => {
  if (activeConversationId.value === null) return null
  const entry = typingInfo.value.get(activeConversationId.value)
  return entry?.userId ?? null
})

const isAuthenticated = computed(() => currentUserId.value !== '' && deviceId.value !== '')
const isConnected = computed(() => connectionState.value === 'connected')

function mapHistoryMessage(
  m: vueapi.MessageItem,
  conv: Conversation | undefined,
): ChatMessage {
  const isSystem = isSystemMessageType(m.message_type, false)
  const remoteName = conv?.title ?? '未知用户'
  return {
    id: m.id,
    conversationId: m.conversation_id,
    senderId: m.sender_id,
    senderName: isSystem ? '系统' : (m.sender_id === currentUserId.value ? currentUserLabel.value : remoteName),
    senderAvatar: '',
    content: isSystem ? formatSystemMessage(m.content) : m.content,
    timestamp: new Date(m.created_at).toISOString(),
    isMine: !isSystem && m.sender_id === currentUserId.value,
    isSystem,
  }
}

function displayNameFromUser(user: vueapi.UserInfo | undefined, fallback: string): string {
  if (!user) return fallback
  const nickname = user.nickname?.trim()
  if (nickname) return nickname
  const atIdx = user.email.indexOf('@')
  return atIdx > 0 ? user.email.slice(0, atIdx) : user.email
}

async function buildConversationFromItem(item: vueapi.ConversationItem): Promise<Conversation> {
  const convType = (item.conversation_type === 'group' ? 'group' : 'direct') as 'direct' | 'group'
  const serverName = item.name?.trim() ?? ''
  let title = serverName
  let avatar = item.avatar ?? ''
  const otherIds = (item.member_ids ?? []).filter((id: string) => id !== currentUserId.value)
  const otherId = otherIds.length > 0 ? otherIds[0] : ''

  if (convType === 'direct') {
    if (!title && otherId !== '') {
      try {
        const userResp = await GetUserById(otherId)
        if (userResp?.user) {
          title = displayNameFromUser(userResp.user, `用户 ${otherId}`)
          if (!avatar) avatar = userResp.user.avatar ?? ''
        }
      } catch { /* use placeholder */ }
    }
    if (!title) title = otherId !== '' ? `用户 ${otherId}` : `会话 ${item.conversation_id}`
    return {
      id: item.conversation_id,
      title,
      avatar,
      lastMessage: '',
      lastMessageAt: '',
      unreadCount: 0,
      isOnline: onlineUserIds.value.has(otherId),
      conversationType: convType,
      creatorId: item.creator_id,
      memberIds: item.member_ids ?? [],
    }
  }

  if (!title) title = `群聊 ${item.conversation_id}`

  return {
    id: item.conversation_id,
    title,
    avatar,
    lastMessage: '',
    lastMessageAt: '',
    unreadCount: 0,
    isOnline: false,
    conversationType: convType,
    creatorId: item.creator_id,
    memberIds: item.member_ids ?? [],
  }
}

function upsertConversation(conv: Conversation) {
  const idx = conversations.value.findIndex((c) => c.id === conv.id)
  if (idx >= 0) {
    conversations.value.splice(idx, 1, conv)
  } else {
    conversations.value.unshift(conv)
  }
}

// ─── Conversation history loader ──────────────────────────────────────────────

async function loadConversationHistory(conversationId: string) {
  if (historyLoadedSet.value.has(conversationId) || historyLoading.value) return
  if ((messagesMap.value.get(conversationId)?.length ?? 0) > 0) return

  historyLoading.value = true
  try {
    const resp = await GetConversationHistory(conversationId, '', 0, 50)
    if (!resp?.messages?.length) {
      historyLoadedSet.value.add(conversationId)
      return
    }

    const conv = conversations.value.find((c) => c.id === conversationId)

    const hist: ChatMessage[] = resp.messages.map((m: vueapi.MessageItem) => mapHistoryMessage(m, conv))

    if (!messagesMap.value.has(conversationId)) {
      messagesMap.value.set(conversationId, [])
    }
    const existing = messagesMap.value.get(conversationId)!
    messagesMap.value.set(conversationId, [...hist, ...existing])

    // 保存游标信息用于分页加载更多历史
    const conv2 = conversations.value.find((c) => c.id === conversationId)
    if (conv2) {
      conv2.historyCursor = {
        cursorCreatedAt: resp.next_cursor_created_at,
        cursorId: resp.next_cursor_id,
        hasMore: resp.has_more,
      }
    }
    historyLoadedSet.value.add(conversationId)
  } catch {
    ElMessage.error('加载历史消息失败')
  } finally {
    historyLoading.value = false
  }
}

// ─── Helpers for WS frame decoding ────────────────────────────────────────────

function toRecord(val: unknown): Record<string, unknown> | null {
  if (val === null || val === undefined) return null
  if (typeof val !== 'object') return null
  return val as Record<string, unknown>
}

// Frame types from ws.proto
const WS_FRAME = {
  PUSH_MESSAGE: 101,
  PUSH_PRESENCE: 102,
  PUSH_NOTIFICATION: 103,
  PUSH_TYPING: 104,
  RECONNECT: 105,
  SERVER_ACK: 106,
  TOKEN_EXPIRED: 107,
  PUSH_FRIEND_APPLICATION: 108,
} as const

interface WsFrameEnvelope {
  frame?: { type?: number; seq?: number }
  payload?: Record<string, unknown>
}

function parseFrameEnvelope(val: unknown): WsFrameEnvelope | null {
  const obj = toRecord(val)
  if (!obj) return null
  const frame = toRecord(obj.frame)
  const payload = toRecord(obj.payload)
  return {
    frame: frame ? { type: frame.type as number | undefined, seq: frame.seq as number | undefined } : undefined,
    payload: payload ?? undefined,
  }
}

// ─── Create or get conversation from WS push ──────────────────────────────────

async function ensureConversationForPush(
  conversationId: string,
  senderId: string,
  conversationType?: string,
): Promise<Conversation | null> {
  let conv = conversations.value.find((c) => c.id === conversationId)
  if (conv) return conv

  const convType = conversationType === 'group' ? 'group' : 'direct'
  let title = convType === 'group' ? `群聊 ${conversationId}` : `用户 ${senderId}`
  let avatar = ''

  if (convType === 'direct' && senderId !== '') {
    try {
      const info = await resolveSenderInfo(senderId)
      title = info.name
      avatar = info.avatar
    } catch {
      // Use placeholder title
    }
  }

  // Create local conversation using the push's conversationId (NOT a new one)
  conv = {
    id: conversationId,
    title,
    avatar,
    lastMessage: '',
    lastMessageAt: '',
    unreadCount: 0,
    isOnline: convType === 'direct' ? onlineUserIds.value.has(senderId) : false,
    conversationType: convType,
    memberIds: [senderId, currentUserId.value],
  }
  conversations.value.unshift(conv)
  messagesMap.value.set(conversationId, [])

  // Load history in background
  loadConversationHistory(conversationId)

  return conv
}

async function resolveSenderInfo(senderId: string): Promise<{ name: string; avatar: string }> {
  try {
    const resp = await GetUserById(senderId)
    if (resp?.user) {
      return {
        name: displayNameFromUser(resp.user, `用户 ${senderId}`),
        avatar: resp.user.avatar ?? '',
      }
    }
  } catch { /* ignore */ }
  return { name: `用户 ${senderId}`, avatar: '' }
}

async function refreshPendingFriendApplications() {
  if (currentUserId.value === '') return
  try {
    const resp = await ListFriendApplications()
    const pending = (resp?.applications ?? []).filter(
      (item: { status: string }) => item.status === 'pending',
    )
    pendingFriendAppCount.value = pending.length
  } catch {
    // Best-effort
  }
}

// ─── Wails event handlers ─────────────────────────────────────────────────────

const offHandlers: Array<() => void> = []

onMounted(async () => {
  try {
    const id = await DeviceID()
    if (id) deviceId.value = id
  } catch { /* ignore */ }

  offHandlers.push(
    EventsOn('aim:connection_state', async (state: vueapi.SessionState) => {
      if (state.ws_connected) {
        connectionState.value = 'connected'
        reconnectAttempt = 0
        startHeartbeat()
        refreshPresenceSnapshot()
        refreshPendingFriendApplications()
      } else {
        connectionState.value = 'disconnected'
        stopHeartbeat()
        // 非主动断线时自动重连（指数退避）
        if (!intentionalDisconnect.value) {
          scheduleReconnect()
        }
      }
    }),

    EventsOn('aim:frame_received', async (raw: unknown) => {
      const env = parseFrameEnvelope(raw)
      const frameType = env?.frame?.type
      const seq = env?.frame?.seq
      const payload = env?.payload

      if (seq != null && seq > lastReadSeq.value) {
        lastReadSeq.value = seq
      }

      switch (frameType) {
        // ── PUSH_MESSAGE (101) ──────────────────────────────────────────
        case WS_FRAME.PUSH_MESSAGE: {
          const conversationId = payload?.conversation_id as string | undefined
          const senderId = (payload?.sender_id as string) ?? ''
          const rawContent = (payload?.content as string) ?? ''
          const msgType = (payload?.message_type as string) ?? 'text'
          const isSystem = isSystemMessageType(msgType, payload?.is_system as boolean | undefined)
          const msgId = (payload?.message_id as string) ?? String(Date.now())
          const sentAt = payload?.sent_at as number | undefined
          const clientMsgId = payload?.client_msg_id as string | undefined

          if (!conversationId) break

          // Skip own messages that we already have optimistically (non-system only)
          if (!isSystem && senderId === currentUserId.value) break

          const convTypeFromPush = (payload?.conversation_type as string) ?? ''
          const conv = await ensureConversationForPush(conversationId, senderId, convTypeFromPush)
          if (!conv) break

          let senderName = '系统'
          let senderAvatar = ''
          if (!isSystem) {
            const senderInfo = await resolveSenderInfo(senderId)
            senderName = senderInfo.name
            senderAvatar = senderInfo.avatar
            if (conv.conversationType !== 'group' && conv.title.startsWith('用户 ')) {
              conv.title = senderInfo.name
              conv.avatar = senderInfo.avatar
              conv.memberIds = conv.memberIds ?? [senderId, currentUserId.value]
            }
          }

          const content = isSystem ? formatSystemMessage(rawContent) : rawContent
          const timestamp = sentAt ? new Date(sentAt).toISOString() : new Date().toISOString()

          // Check for duplicate
          const existing = messagesMap.value.get(conversationId)
          const dupIdx = existing?.findIndex((m) =>
            (clientMsgId && m.clientMsgId === clientMsgId) || m.id === msgId
          ) ?? -1

          const newMsg: ChatMessage = {
            id: msgId,
            conversationId,
            senderId,
            senderName,
            senderAvatar,
            content,
            timestamp,
            isMine: false,
            isSystem,
            clientMsgId,
          }

          if (!messagesMap.value.has(conversationId)) {
            messagesMap.value.set(conversationId, [])
          }
          if (dupIdx >= 0) {
            messagesMap.value.get(conversationId)!.splice(dupIdx, 1, newMsg)
          } else {
            messagesMap.value.get(conversationId)!.push(newMsg)
          }

          let unreadCount = conv.unreadCount ?? 0
          if (activeConversationId.value === null) {
            activeConversationId.value = conversationId
            unreadCount = 0
            loadConversationHistory(conversationId)
          } else if (activeConversationId.value === conversationId) {
            unreadCount = 0
            SendReadReceipt(conversationId, msgId).catch(() => {})
          } else {
            unreadCount++
          }

          const convIdx = conversations.value.findIndex((c) => c.id === conversationId)
          if (convIdx >= 0) {
            conversations.value.splice(convIdx, 1, {
              ...conversations.value[convIdx],
              lastMessage: content,
              lastMessageAt: timestamp,
              unreadCount,
            })
          }

          // Send ACK
          if (seq != null) {
            SendAck(seq).catch(() => {})
          }
          break
        }

        // ── PUSH_PRESENCE (102) ────────────────────────────────────────
        case WS_FRAME.PUSH_PRESENCE: {
          const userId = payload?.user_id as string | undefined
          const status = (payload?.status as string) ?? ''
          if (userId == null) break

          // Only accept strict online/offline states (aggregated by server).
          if (status === 'online') {
            onlineUserIds.value = new Set([...onlineUserIds.value, userId])
          } else if (status === 'offline') {
            const next = new Set(onlineUserIds.value)
            next.delete(userId)
            onlineUserIds.value = next
          } else {
            break // ignore unknown statuses
          }

          // Update conversation online status
          const conv = conversations.value.find((c) =>
            c.memberIds?.includes(userId)
          )
          if (conv) {
            conv.isOnline = status === 'online'
          }
          break
        }

        // ── PUSH_NOTIFICATION (103) ──────────────────────────────────
        case WS_FRAME.PUSH_NOTIFICATION: {
          const notificationType = (payload?.notification_type as string) ?? ''
          const title = (payload?.title as string) ?? ''
          const body = (payload?.body as string) ?? ''
          const relatedId = payload?.related_id as string | undefined
          const displayText = [title, body].filter(Boolean).join('：')
          if (displayText) {
            ElMessage.info(displayText)
          }
          // relatedId may be used to navigate to the related conversation/object
          break
        }

        // ── PUSH_TYPING (104) ──────────────────────────────────────────
        case WS_FRAME.PUSH_TYPING: {
          const conversationId = payload?.conversation_id as string | undefined
          const userId = payload?.user_id as string | undefined
          if (!conversationId || !userId) break

          // Clear any previous timer for this conversation.
          const prev = typingInfo.value.get(conversationId)
          if (prev?.timer) clearTimeout(prev.timer)

          // Set new typing entry with auto-clear timer.
          const timer = setTimeout(() => {
            typingInfo.value.delete(conversationId)
            // Trigger reactivity.
            typingInfo.value = new Map(typingInfo.value)
          }, TYPING_TIMEOUT_MS)

          typingInfo.value.set(conversationId, { userId, timer })
          // Trigger Vue reactivity for Map.
          typingInfo.value = new Map(typingInfo.value)
          break
        }

        // ── PUSH_FRIEND_APPLICATION (108) ──────────────────────────────
        case WS_FRAME.PUSH_FRIEND_APPLICATION: {
          const status = (payload?.status as string) ?? ''
          const userEmail = (payload?.user_id != null)
            ? (await resolveSenderInfo(payload.user_id as string)).name
            : '有人'

          if (status === 'pending') {
            await refreshPendingFriendApplications()
            ElMessage.info(`${userEmail} 向你发送了好友申请`)
          } else if (status === 'accepted') {
            ElMessage.success(`${userEmail} 已接受你的好友申请`)
          } else if (status === 'rejected') {
            ElMessage.warning(`${userEmail} 已拒绝你的好友申请`)
          }
          break
        }

        // ── SERVER_ACK (106) ───────────────────────────────────────────
        case WS_FRAME.SERVER_ACK: {
          const ackSeq = payload?.ack_seq as number | undefined
          const clientMsgId = payload?.client_msg_id as string | undefined
          const ackStatus = payload?.status as number | undefined
          // status: 1=ACCEPTED, 2=REJECTED, 3=RETRYABLE

          if (clientMsgId) {
            // Find and update the message with matching clientMsgId
            for (const [, msgs] of messagesMap.value) {
              const msg = msgs.find((m) => m.clientMsgId === clientMsgId)
              if (msg) {
                if (ackStatus === 1) {
                  msg.ackStatus = 'delivered'
                } else if (ackStatus === 2) {
                  msg.ackStatus = 'failed'
                }
                break
              }
            }
          }

          // Send client ack for the server ack
          if (ackSeq != null) {
            SendAck(ackSeq).catch(() => {})
          }
          break
        }

        // ── TOKEN_EXPIRED (107) ────────────────────────────────────────
        case WS_FRAME.TOKEN_EXPIRED: {
          try {
            await Refresh({ refresh_token: '' })
            // 刷新成功后重新建立 WebSocket 连接
            try {
              await ConnectWS()
            } catch {
              ElMessage.error('重新连接失败，请重新登录')
              handleLogout()
            }
          } catch {
            ElMessage.error('登录已过期，请重新登录')
            handleLogout()
          }
          break
        }

        // ── RECONNECT (105) ────────────────────────────────────────────
        case WS_FRAME.RECONNECT: {
          const delayMs = (payload?.reconnect_delay_ms as number) ?? 3000
          connectionState.value = 'connecting'
          setTimeout(async () => {
            try {
              await ConnectWS()
              connectionState.value = 'connected'
            } catch {
              connectionState.value = 'disconnected'
            }
          }, delayMs)
          break
        }

        default:
          break
      }
    }),

    EventsOn('aim:error', (_error: unknown) => {
      ElMessage.error('连接异常，请检查网络')
    }),
  )
})

onUnmounted(() => {
  offHandlers.forEach((off) => off())
  stopHeartbeat()
  clearReconnectTimer()
})

// ─── Presence snapshot ────────────────────────────────────────────────────────

async function refreshPresenceSnapshot() {
  if (currentUserId.value === '') return
  try {
    // Dynamic import; GetFriendsPresence is exposed via wails bindings (step 15).
    const { GetFriendsPresence } = await import('../wailsjs/go/main/App')
    const resp = await GetFriendsPresence()
    if (!resp?.presences?.length) return
    const next = new Set<string>()
    for (const item of resp.presences) {
      if (item.status === 'online') {
        next.add(item.user_id)
      }
    }
    onlineUserIds.value = next
    // Also refresh conversation isOnline flags.
    for (const conv of conversations.value) {
      const otherIds = (conv.memberIds ?? []).filter((id) => id !== currentUserId.value)
      conv.isOnline = otherIds.some((id) => next.has(id))
    }
  } catch {
    // Best-effort; presence will be updated via PUSH_PRESENCE events.
  }
}

// ─── Heartbeat ────────────────────────────────────────────────────────────────

function startHeartbeat() {
  if (heartbeatTimer) return
  heartbeatTimer = setInterval(() => {
    if (connectionState.value === 'connected') {
      SendHeartbeat(lastReadSeq.value).catch(() => {})
    }
  }, 20_000)
}

function stopHeartbeat() {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

function clearReconnectTimer() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

function scheduleReconnect() {
  if (reconnectAttempt >= MAX_RECONNECT_ATTEMPTS) {
    ElMessage.error('无法重新连接，请检查网络后手动登录')
    return
  }
  clearReconnectTimer()
  // 指数退避: 1s → 2s → 4s → 8s, 最多 30s
  const delay = Math.min(1000 * Math.pow(2, reconnectAttempt), 30000)
  reconnectAttempt++
  reconnectTimer = setTimeout(async () => {
    try {
      await ConnectWS()
    } catch {
      // 重连失败，if 还有机会则继续
      if (reconnectAttempt < MAX_RECONNECT_ATTEMPTS) {
        scheduleReconnect()
      } else {
        ElMessage.error('重连次数已达上限，请检查网络后手动登录')
      }
    }
  }, delay)
}

// ─── Auth handlers ────────────────────────────────────────────────────────────

async function handleLogin(payload: { email: string; password: string; device_id: string }) {
  authLoading.value = true
  try {
    const resp = await Login({ email: payload.email, password: payload.password, device_id: payload.device_id })
    if (resp) {
      const state = await SessionState()
      currentUserId.value = state.user_id ?? ''
      currentUserLabel.value = payload.email.split('@')[0]
      intentionalDisconnect.value = false
      reconnectAttempt = 0
      clearReconnectTimer()
      conversations.value = []
      messagesMap.value = new Map()
      historyLoadedSet.value.clear()
      try {
        await ConnectWS()
        connectionState.value = 'connected'
        // Pull presence snapshot after connect.
        await refreshPresenceSnapshot()
        await refreshPendingFriendApplications()
      } catch {
        ElMessage.warning('已登录，但无法建立实时连接')
      }

      await refreshPendingFriendApplications()

      // Load existing conversations from server
      try {
        const listResp = await ListConversations()
        const items = listResp?.conversations ?? []
        if (items.length > 0) {
          const loadedConvs = await Promise.all(items.map((item) => buildConversationFromItem(item)))
          conversations.value = loadedConvs
          // Load history for each conversation
          for (const conv of loadedConvs) {
            loadConversationHistory(conv.id)
          }
        }
      } catch {
        // Non-critical: conversations will populate via WS pushes
      }
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : '登录失败，请检查邮箱和密码'
    ElMessage.error(msg)
  } finally {
    authLoading.value = false
  }
}

async function handleRegister(payload: { email: string; password: string; username: string; avatar: string; device_id: string }) {
  authLoading.value = true
  try {
    await Register({
      email: payload.email,
      password: payload.password,
      username: payload.username,
      avatar: payload.avatar,
      device_id: payload.device_id,
    })
    ElMessage.success('注册成功，请登录')
    authView.value = 'login'
  } catch (err) {
    const msg = err instanceof Error ? err.message : '注册失败，请稍后重试'
    ElMessage.error(msg)
  } finally {
    authLoading.value = false
  }
}

async function handleLogout() {
  intentionalDisconnect.value = true
  clearReconnectTimer()
  stopHeartbeat()
  try {
    await Logout()
  } catch {
    // best effort
  }
  currentUserId.value = ''
  currentUserLabel.value = ''
  conversations.value = []
  messagesMap.value.clear()
  historyLoadedSet.value.clear()
  // Clear typing timers.
  for (const [, entry] of typingInfo.value) {
    if (entry.timer) clearTimeout(entry.timer)
  }
  typingInfo.value.clear()
  typingInfo.value = new Map()
  friendsViewVisible.value = false
  pendingFriendAppCount.value = 0
  connectionState.value = 'disconnected'
  authView.value = 'login'
}

// ─── Conversation handlers ───────────────────────────────────────────────────

// ─── History pagination: 加载更早的历史消息 ──────────────────────────────
async function handleLoadMoreHistory(conversationId: string) {
  if (historyLoading.value) return
  const conv = conversations.value.find((c) => c.id === conversationId)
  if (!conv?.historyCursor?.hasMore) return

  const cursor = conv.historyCursor
  historyLoading.value = true
  try {
    const resp = await GetConversationHistory(
      conversationId,
      cursor.cursorId,
      cursor.cursorCreatedAt,
      50,
    )
    if (!resp?.messages?.length) {
      conv.historyCursor = { ...cursor, hasMore: false }
      return
    }

    const olderMsgs: ChatMessage[] = resp.messages.map((m: vueapi.MessageItem) => mapHistoryMessage(m, conv))

    // 将更早的消息追加到列表头部（保持时间顺序）
    const existing = messagesMap.value.get(conversationId) ?? []
    messagesMap.value.set(conversationId, [...olderMsgs, ...existing])

    // 更新游标
    conv.historyCursor = {
      cursorCreatedAt: resp.next_cursor_created_at,
      cursorId: resp.next_cursor_id,
      hasMore: resp.has_more,
    }
  } catch {
    ElMessage.error('加载更多历史消息失败')
  } finally {
    historyLoading.value = false
  }
}

function handleSelectConversation(id: string) {
  friendsViewVisible.value = false
  activeConversationId.value = id
  const conv = conversations.value.find((c) => c.id === id)
  if (conv) conv.unreadCount = 0
  loadConversationHistory(id)
  // 发送已读回执：标记该会话最后一条消息为已读
  const msgs = messagesMap.value.get(id)
  if (msgs && msgs.length > 0) {
    const lastMsg = msgs[msgs.length - 1]
    SendReadReceipt(id, lastMsg.id).catch(() => {})
  }
}

async function handleSendMessage(content: string) {
  if (activeConversationId.value === null) return
  if (!isConnected.value) {
    ElMessage.warning('未连接，消息发送失败')
    return
  }

  const conversationId = activeConversationId.value
  const clientMsgId = Date.now().toString() + '-' + Math.random().toString(36).slice(2, 8)

  const optimisticMsg: ChatMessage = {
    id: String(Date.now()),
    conversationId,
    senderId: currentUserId.value,
    senderName: currentUserLabel.value,
    senderAvatar: '',
    content,
    timestamp: new Date().toISOString(),
    isMine: true,
    clientMsgId,
    ackStatus: 'pending',
  }

  // 乐观更新：先将消息加入 UI
  if (!messagesMap.value.has(conversationId)) {
    messagesMap.value.set(conversationId, [])
  }
  messagesMap.value.get(conversationId)!.push(optimisticMsg)

  try {
    await SendMessage({
      conversation_id: conversationId,
      message_type: 'text',
      content,
      client_msg_id: clientMsgId,
      mentions: [],
    })
  } catch {
    // 发送失败，将乐观消息标记为 failed
    optimisticMsg.ackStatus = 'failed'
    ElMessage.error('消息发送失败')
    return
  }

  const conv = conversations.value.find((c) => c.id === conversationId)
  if (conv) {
    conv.lastMessage = content
    conv.lastMessageAt = optimisticMsg.timestamp
    conv.unreadCount = 0
  }
}

function handleTyping() {
  if (activeConversationId.value === null) return
  if (!isConnected.value) return
  SendTyping(activeConversationId.value).catch(() => {})
}

// ─── Search / start direct conversation ───────────────────────────────────────

async function handleSearchUsers(keyword: string) {
  searchKeyword.value = keyword
  searchError.value = ''
  if (!keyword.trim()) {
    searchResults.value = []
    return
  }
  searchLoading.value = true
  try {
    searchResults.value = await SearchUsersByName(keyword.trim())
  } catch {
    searchError.value = '搜索失败，请稍后重试'
    searchResults.value = []
  } finally {
    searchLoading.value = false
  }
}

async function handleStartDirect(userId: string) {
  const user = searchResults.value.find((item) => item.id === userId)
  createLoading.value = true
  creatingUserId.value = userId
  try {
    const resp = await CreateDirectConversation(userId)
    const title = resp.name?.trim() || (user ? displayUserName(user) : `用户 ${userId}`)
    const newConv: Conversation = {
      id: resp.conversation_id,
      title,
      avatar: user?.avatar ?? '',
      lastMessage: '',
      lastMessageAt: '',
      unreadCount: 0,
      isOnline: onlineUserIds.value.has(userId),
      conversationType: 'direct',
      memberIds: resp.member_ids ?? [userId, currentUserId.value],
    }
    const idx = conversations.value.findIndex((c) => c.id === resp.conversation_id)
    if (idx >= 0) {
      conversations.value.splice(idx, 1, newConv)
    } else {
      conversations.value.unshift(newConv)
    }
    messagesMap.value.set(resp.conversation_id, [])
    activeConversationId.value = resp.conversation_id
    friendsViewVisible.value = false
    searchResults.value = []
    searchKeyword.value = ''
    loadConversationHistory(resp.conversation_id)
  } catch (err) {
    const msg = err instanceof Error ? err.message : '创建会话失败'
    ElMessage.error(msg)
  } finally {
    createLoading.value = false
    creatingUserId.value = null
  }
}

// ─── Friends view handlers ────────────────────────────────────────────────────

function handleOpenFriends() {
  friendsInitialTab.value = pendingFriendAppCount.value > 0 ? 'applications' : 'friends'
  friendsViewVisible.value = true
}

async function handleAddFriend(userId: string) {
  if (userId === currentUserId.value) return
  if (addFriendLoading.value) return
  addFriendLoading.value = true
  addingFriendUserId.value = userId
  // #region agent log
  fetch('http://127.0.0.1:7448/ingest/f55c4ddd-4668-4f70-bd8a-b6b233ad8684',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'0bb7cd'},body:JSON.stringify({sessionId:'0bb7cd',hypothesisId:'A',location:'App.vue:handleAddFriend',message:'add friend user id',data:{userId,jsNumber:Number(userId)},timestamp:Date.now()})}).catch(()=>{});
  // #endregion
  try {
    await AddFriend(userId)
    ElMessage.success('好友申请已发送')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '添加好友失败'
    ElMessage.error(msg)
  } finally {
    addFriendLoading.value = false
    addingFriendUserId.value = null
  }
}

function handleApplicationsUpdated() {
  refreshPendingFriendApplications()
}

function handleFriendsStartConversation(
  conversationId: string,
  friendId: string,
  title: string,
  avatar: string,
) {
  const newConv: Conversation = {
    id: conversationId,
    title,
    avatar,
    lastMessage: '',
    lastMessageAt: '',
    unreadCount: 0,
    isOnline: onlineUserIds.value.has(friendId),
    conversationType: 'direct',
    memberIds: [friendId, currentUserId.value],
  }
  upsertConversation(newConv)
  if (!messagesMap.value.has(conversationId)) {
    messagesMap.value.set(conversationId, [])
  }
  activeConversationId.value = conversationId
  friendsViewVisible.value = false
  loadConversationHistory(conversationId)
}

function handleFriendsStartGroup(
  conversationId: string,
  title: string,
  avatar: string,
  memberIds: string[],
) {
  const newConv: Conversation = {
    id: conversationId,
    title,
    avatar,
    lastMessage: '',
    lastMessageAt: '',
    unreadCount: 0,
    isOnline: false,
    conversationType: 'group',
    creatorId: currentUserId.value,
    memberIds,
  }
  upsertConversation(newConv)
  if (!messagesMap.value.has(conversationId)) {
    messagesMap.value.set(conversationId, [])
  }
  activeConversationId.value = conversationId
  friendsViewVisible.value = false
  loadConversationHistory(conversationId)
}

function handleGroupUpdated(updated: Conversation) {
  upsertConversation(updated)
}

function handleGroupLeft(conversationId: string) {
  conversations.value = conversations.value.filter((c) => c.id !== conversationId)
  messagesMap.value.delete(conversationId)
  if (activeConversationId.value === conversationId) {
    activeConversationId.value = null
  }
  groupSettingsVisible.value = false
}

function handleGroupDismissed(conversationId: string) {
  handleGroupLeft(conversationId)
}

function displayUserName(user: SearchUserItem): string {
  const atIndex = user.email.indexOf('@')
  return atIndex > 0 ? user.email.slice(0, atIndex) : user.email
}
</script>

<template>
  <!-- Auth pages -->
  <div v-if="!isAuthenticated" class="auth-page">
    <Transition name="auth-fade" mode="out-in">
      <LoginView
        v-if="authView === 'login'"
        :loading="authLoading"
        :device-id="deviceId"
        @login="handleLogin"
        @switch-register="authView = 'register'"
      />
      <RegisterView
        v-else
        :loading="authLoading"
        :device-id="deviceId"
        @register="handleRegister"
        @switch-login="authView = 'login'"
      />
    </Transition>
  </div>

  <!-- Chat workspace -->
  <div v-else class="app-shell">
    <div class="chat-sidebar">
      <ConversationList
        v-if="!friendsViewVisible"
        :conversations="conversations"
        :active-conversation-id="activeConversationId"
        :current-user-label="currentUserLabel"
        :current-user-id="currentUserId"
        :connected="isConnected"
        :pending-friend-count="pendingFriendAppCount"
        :search-keyword="searchKeyword"
        :search-results="searchResults"
        :search-loading="searchLoading"
        :create-loading="createLoading"
        :creating-user-id="creatingUserId"
        :add-friend-loading="addFriendLoading"
        :adding-friend-user-id="addingFriendUserId"
        :search-error="searchError"
        @select="handleSelectConversation"
        @logout="handleLogout"
        @open-friends="handleOpenFriends"
        @search-user="handleSearchUsers"
        @start-direct="handleStartDirect"
        @add-friend="handleAddFriend"
      />
      <FriendsView
        v-else
        :current-user-id="currentUserId"
        :online-user-ids="onlineUserIds"
        :initial-tab="friendsInitialTab"
        @start-conversation="handleFriendsStartConversation"
        @start-group="handleFriendsStartGroup"
        @applications-updated="handleApplicationsUpdated"
        @back="friendsViewVisible = false"
      />
    </div>
    <div class="chat-main">
      <MessageArea
        :conversation="activeConversation"
        :messages="activeMessages"
        :typing-user-id="typingUserId"
        @load-more="handleLoadMoreHistory"
        @open-settings="groupSettingsVisible = true"
      />
      <GroupSettingsPanel
        v-model:visible="groupSettingsVisible"
        :conversation="activeConversation"
        :current-user-id="currentUserId"
        @updated="handleGroupUpdated"
        @left="handleGroupLeft"
        @dismissed="handleGroupDismissed"
      />
      <div class="input-area">
        <MessageInput
          :disabled="!isConnected || activeConversationId === null"
          placeholder="输入消息…"
          @send="handleSendMessage"
          @typing="handleTyping"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.auth-fade-enter-active,
.auth-fade-leave-active {
  transition: opacity 0.2s ease;
}
.auth-fade-enter-from,
.auth-fade-leave-to {
  opacity: 0;
}
</style>
