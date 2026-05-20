<script lang="ts" setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  ConnectWS,
  CreateDirectConversation,
  DeviceID,
  DisconnectWS,
  GetConversationHistory,
  Login,
  Logout,
  Register,
  SearchUsersByName,
  SendMessage,
  SendTyping,
  SessionState,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import type { client, main } from '../wailsjs/go/models'
import type { ChatMessage, Conversation, SearchUserItem } from './components/types'
import LoginView from './views/LoginView.vue'
import RegisterView from './views/RegisterView.vue'
import ConversationList from './components/ConversationList.vue'
import MessageArea from './components/MessageArea.vue'
import MessageInput from './components/MessageInput.vue'

// ─── State ───────────────────────────────────────────────────────────────────

type AuthView = 'login' | 'register'
type ConnectionState = 'connecting' | 'connected' | 'disconnected'

const authView = ref<AuthView>('login')
const authLoading = ref(false)
const deviceId = ref('')
const currentUserId = ref<number>(0)
const currentUserLabel = ref('')
const connectionState = ref<ConnectionState>('disconnected')

const conversations = ref<Conversation[]>([])
const activeConversationId = ref<number | null>(null)
const messagesMap = ref<Map<number, ChatMessage[]>>(new Map())
const historyLoadedSet = ref<Set<number>>(new Set())
const historyLoading = ref(false)

// ─── User search / direct conversation ────────────────────────────────────────
const searchKeyword = ref('')
const searchResults = ref<SearchUserItem[]>([])
const searchLoading = ref(false)
const createLoading = ref(false)
const creatingUserId = ref<number | null>(null)
const searchError = ref('')

// ─── Derived helpers ──────────────────────────────────────────────────────────

const activeConversation = computed<Conversation | null>(() => {
  if (activeConversationId.value === null) return null
  return conversations.value.find((c) => c.id === activeConversationId.value) ?? null
})

const activeMessages = computed<ChatMessage[]>(() => {
  if (activeConversationId.value === null) return []
  return messagesMap.value.get(activeConversationId.value) ?? []
})

const isAuthenticated = computed(() => currentUserId.value > 0 && deviceId.value !== '')
const isConnected = computed(() => connectionState.value === 'connected')

// ─── Conversation history loader ──────────────────────────────────────────────

async function loadConversationHistory(conversationId: number) {
  // Skip if already loaded or currently loading
  if (historyLoadedSet.value.has(conversationId) || historyLoading.value) return
  // Skip if messages already exist for this conversation (already populated)
  if ((messagesMap.value.get(conversationId)?.length ?? 0) > 0) return

  historyLoading.value = true
  try {
    const resp = await GetConversationHistory(conversationId, 0, 0, 50)
    if (!resp?.messages?.length) {
      historyLoadedSet.value.add(conversationId)
      return
    }

    const conv = conversations.value.find((c) => c.id === conversationId)
    const remoteName = conv?.title ?? '未知用户'

    const hist: ChatMessage[] = resp.messages.map((m: client.MessageItem) => ({
      id: m.id,
      conversationId: m.conversation_id,
      senderId: m.sender_id,
      senderName: m.sender_id === currentUserId.value ? currentUserLabel.value : remoteName,
      senderAvatar: '',
      content: m.content,
      timestamp: new Date(m.created_at).toISOString(),
      isMine: m.sender_id === currentUserId.value,
    }))

    if (!messagesMap.value.has(conversationId)) {
      messagesMap.value.set(conversationId, [])
    }
    // Prepend history (oldest first) before optimistic/real-time messages
    const existing = messagesMap.value.get(conversationId)!
    messagesMap.value.set(conversationId, [...hist, ...existing])
    historyLoadedSet.value.add(conversationId)
  } catch {
    ElMessage.error('加载历史消息失败')
  } finally {
    historyLoading.value = false
  }
}

// ─── Wails event handlers ─────────────────────────────────────────────────────

const offHandlers: Array<() => void> = []

interface FramePayload {
  conversation_id?: number
  sender_id?: number
  sender_name?: string
  content?: string
  timestamp?: string
  message_id?: number
  sent_at?: number
  client_msg_id?: string
  conversation_type?: string
  message_type?: string
}

function toRecord(val: unknown): Record<string, unknown> | null {
  if (val === null || val === undefined) return null
  if (typeof val !== 'object') return null
  return val as Record<string, unknown>
}

function toFramePayload(val: unknown): FramePayload | null {
  const obj = toRecord(val)
  if (!obj) return null

  const nested = toRecord(obj.payload)
  const source = nested ?? obj
  return (
    typeof source.conversation_id === 'number' ||
    typeof source.message_type === 'string'
  ) ? {
    conversation_id: typeof source.conversation_id === 'number' ? source.conversation_id : undefined,
    sender_id: typeof source.sender_id === 'number' ? source.sender_id : undefined,
    sender_name: typeof source.sender_name === 'string' ? source.sender_name : undefined,
    content: typeof source.content === 'string' ? source.content : undefined,
    timestamp: typeof source.timestamp === 'string' ? source.timestamp : undefined,
    message_id: typeof source.message_id === 'number' ? source.message_id : undefined,
    sent_at: typeof source.sent_at === 'number' ? source.sent_at : undefined,
    client_msg_id: typeof source.client_msg_id === 'string' ? source.client_msg_id : undefined,
    conversation_type: typeof source.conversation_type === 'string' ? source.conversation_type : undefined,
    message_type: typeof source.message_type === 'string' ? source.message_type : undefined,
  } : null
}

onMounted(async () => {
  // Load device ID immediately
  try {
    const id = await DeviceID()
    if (id) deviceId.value = id
  } catch {
    // device ID load failed — proceed without it
  }

  offHandlers.push(
    EventsOn('aim:connection_state', (state: main.SessionState) => {
      if (state.ws_connected) {
        connectionState.value = 'connected'
      } else {
        connectionState.value = 'disconnected'
      }
    }),
    EventsOn('aim:frame_received', (frame: unknown) => {
      const payload = toFramePayload(frame)
      if (!payload?.conversation_id || !payload.message_type) return

      // Only process text messages for now
      if (payload.message_type !== 'text' && payload.message_type !== 'message') return
      if (!payload.content) return

      const conversationId = payload.conversation_id
      const msgId = payload.message_id ?? Date.now()
      const senderId = payload.sender_id ?? 0
      const senderName = payload.sender_name ?? (senderId === currentUserId.value ? currentUserLabel.value : `用户 ${senderId}`)
      const timestamp = payload.timestamp ?? (payload.sent_at ? new Date(payload.sent_at).toISOString() : new Date().toISOString())

      const existing = messagesMap.value.get(conversationId)
      const existingIndex = existing?.findIndex((m) =>
        (payload.client_msg_id && m.clientMsgId === payload.client_msg_id) || m.id === msgId
      ) ?? -1

      const newMsg: ChatMessage = {
        id: msgId,
        conversationId,
        senderId,
        senderName,
        senderAvatar: '',
        content: payload.content,
        timestamp,
        isMine: senderId === currentUserId.value,
        clientMsgId: payload.client_msg_id,
      }

      if (!messagesMap.value.has(conversationId)) {
        messagesMap.value.set(conversationId, [])
      }
      if (existingIndex >= 0) {
        messagesMap.value.get(conversationId)!.splice(existingIndex, 1, newMsg)
      } else {
        messagesMap.value.get(conversationId)!.push(newMsg)
      }

      let conv = conversations.value.find((c) => c.id === conversationId)
      if (!conv) {
        conv = {
          id: conversationId,
          title: senderId === currentUserId.value ? '我' : senderName,
          avatar: '',
          lastMessage: '',
          lastMessageAt: '',
          unreadCount: 0,
          isOnline: false,
        }
        conversations.value.unshift(conv)
      }
      conv.lastMessage = payload.content
      conv.lastMessageAt = timestamp
      if (newMsg.isMine || activeConversationId.value === conversationId) conv.unreadCount = 0
      else conv.unreadCount++
    }),
    EventsOn('aim:error', (_error: unknown) => {
      // Surface errors as user-friendly messages, not raw logs
      ElMessage.error('连接异常，请检查网络')
    }),
  )
})

onUnmounted(() => {
  offHandlers.forEach((off) => off())
})

// ─── Auth handlers ────────────────────────────────────────────────────────────

async function handleLogin(payload: { email: string; password: string; device_id: string }) {
  authLoading.value = true
  try {
    const resp = await Login({ email: payload.email, password: payload.password, device_id: payload.device_id })
    if (resp) {
      const state = await SessionState()
      currentUserId.value = state.user_id ?? 0
      currentUserLabel.value = payload.email.split('@')[0]
      conversations.value = []
      messagesMap.value = new Map()
      activeConversationId.value = null
      try {
        await ConnectWS()
        connectionState.value = 'connected'
      } catch {
        ElMessage.warning('已登录，但无法建立实时连接')
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
  try {
    await Logout()
  } catch {
    // best effort
  }
  currentUserId.value = 0
  currentUserLabel.value = ''
  conversations.value = []
  messagesMap.value.clear()
  historyLoadedSet.value.clear()
  activeConversationId.value = null
  connectionState.value = 'disconnected'
  authView.value = 'login'
}

// ─── Conversation handlers ───────────────────────────────────────────────────

function handleSelectConversation(id: number) {
  activeConversationId.value = id
  // Clear unread for selected conversation
  const conv = conversations.value.find((c) => c.id === id)
  if (conv) conv.unreadCount = 0
  // Load history if not yet loaded and no messages present
  loadConversationHistory(id)
}

async function handleSendMessage(content: string) {
  if (activeConversationId.value === null) return
  if (!isConnected.value) {
    ElMessage.warning('未连接，消息发送失败')
    return
  }

  const conversationId = activeConversationId.value
  const clientMsgId = Date.now().toString()

  const optimisticMsg: ChatMessage = {
    id: Date.now(),
    conversationId,
    senderId: currentUserId.value,
    senderName: currentUserLabel.value,
    senderAvatar: '',
    content,
    timestamp: new Date().toISOString(),
    isMine: true,
    clientMsgId,
  }

  try {
    await SendMessage({
      conversation_id: conversationId,
      message_type: 'text',
      content,
      client_msg_id: clientMsgId,
      mentions: [],
    })
  } catch {
    ElMessage.error('消息发送失败')
    return
  }

  if (!messagesMap.value.has(conversationId)) {
    messagesMap.value.set(conversationId, [])
  }
  messagesMap.value.get(conversationId)!.push(optimisticMsg)

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
  SendTyping(activeConversationId.value).catch(() => {
    // typing errors should not surface as debug logs
  })
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

async function handleStartDirect(userId: number) {
  const user = searchResults.value.find((item) => item.id === userId)
  createLoading.value = true
  creatingUserId.value = userId
  try {
    const resp = await CreateDirectConversation(userId)
    const title = user ? displayUserName(user) : `用户 ${userId}`
    const newConv: Conversation = {
      id: resp.conversation_id,
      title,
      avatar: user?.avatar ?? '',
      lastMessage: '',
      lastMessageAt: '',
      unreadCount: 0,
      isOnline: false,
    }
    // upsert: avoid duplicates if already exists
    const idx = conversations.value.findIndex((c) => c.id === resp.conversation_id)
    if (idx >= 0) {
      conversations.value.splice(idx, 1, newConv)
    } else {
      conversations.value.unshift(newConv)
    }
    messagesMap.value.set(resp.conversation_id, [])
    activeConversationId.value = resp.conversation_id
    searchResults.value = []
    searchKeyword.value = ''
    // Load history for the newly created conversation
    loadConversationHistory(resp.conversation_id)
  } catch (err) {
    const msg = err instanceof Error ? err.message : '创建会话失败'
    ElMessage.error(msg)
  } finally {
    createLoading.value = false
    creatingUserId.value = null
  }
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
        :conversations="conversations"
        :active-conversation-id="activeConversationId"
        :current-user-label="currentUserLabel"
        :connected="isConnected"
        :search-keyword="searchKeyword"
        :search-results="searchResults"
        :search-loading="searchLoading"
        :create-loading="createLoading"
        :creating-user-id="creatingUserId"
        :search-error="searchError"
        @select="handleSelectConversation"
        @logout="handleLogout"
        @search-user="handleSearchUsers"
        @start-direct="handleStartDirect"
      />
    </div>
    <div class="chat-main">
      <MessageArea
        :conversation="activeConversation"
        :messages="activeMessages"
        :typing-label="isConnected && activeConversationId !== null ? activeConversation?.title : ''"
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
