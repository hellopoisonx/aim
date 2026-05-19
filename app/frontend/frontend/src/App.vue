<script lang="ts" setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  ConnectWS,
  DeviceID,
  DisconnectWS,
  Login,
  Logout,
  Register,
  SendMessage,
  SendTyping,
  SessionState,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'
import type { client, main } from '../wailsjs/go/models'
import type { ChatMessage, Conversation } from './components/types'
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

// ─── Wails event handlers ─────────────────────────────────────────────────────

const offHandlers: Array<() => void> = []

interface FramePayload {
  conversation_id?: number
  sender_id?: number
  sender_name?: string
  content?: string
  timestamp?: string
  msg_id?: string
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
    msg_id: typeof source.msg_id === 'string' ? source.msg_id : undefined,
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
      const msgId = payload.msg_id ? parseInt(payload.msg_id, 10) : Date.now()
      const senderId = payload.sender_id ?? 0
      const senderName = payload.sender_name ?? '未知用户'
      const timestamp = payload.timestamp ?? new Date().toISOString()

      // Avoid duplicates from optimistic updates
      const existing = messagesMap.value.get(conversationId)
      if (existing?.some((m) => m.id === msgId)) return

      const newMsg: ChatMessage = {
        id: msgId,
        conversationId,
        senderId,
        senderName,
        senderAvatar: '',
        content: payload.content,
        timestamp,
        isMine: senderId === currentUserId.value,
      }

      if (!messagesMap.value.has(conversationId)) {
        messagesMap.value.set(conversationId, [])
      }
      messagesMap.value.get(conversationId)!.push(newMsg)

      // Update conversation preview
      const conv = conversations.value.find((c) => c.id === conversationId)
      if (conv) {
        conv.lastMessage = payload.content
        conv.lastMessageAt = timestamp
        if (newMsg.isMine) conv.unreadCount = 0
        else conv.unreadCount++
      }
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
        @select="handleSelectConversation"
        @logout="handleLogout"
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
