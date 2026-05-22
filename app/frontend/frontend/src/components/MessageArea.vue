<script lang="ts" setup>
import { computed, nextTick, ref, watch } from 'vue'
import { ElAvatar, ElEmpty, ElScrollbar } from 'element-plus'
import type { ChatMessage, Conversation } from './types'

interface Props {
  conversation: Conversation | null
  messages: ChatMessage[]
  typingUserId?: string | null
}

const props = withDefaults(defineProps<Props>(), {
  typingUserId: null,
})

const emit = defineEmits<{
  'load-more': [conversationId: string]
  'open-settings': []
}>()

const scrollbarRef = ref<InstanceType<typeof ElScrollbar> | null>(null)
const isLoadingMore = ref(false)

const hasMessages = computed(() => props.messages.length > 0)

const typingDisplayName = computed(() => {
  if (!props.typingUserId || !props.conversation) return ''
  // If the conversation has memberIds, check if typing user is in it
  if (props.conversation.memberIds?.includes(props.typingUserId)) {
    // Try to derive name from messages
    const msg = props.messages.find((m) => m.senderId === props.typingUserId && !m.isMine)
    return msg?.senderName ?? `用户 ${props.typingUserId}`
  }
  return ''
})

function formatTime(isoString: string): string {
  const date = new Date(isoString)
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function formatDateGroup(isoString: string): string {
  const date = new Date(isoString)
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const yesterday = new Date(today.getTime() - 86400000)
  const msgDate = new Date(date.getFullYear(), date.getMonth(), date.getDate())
  if (msgDate.getTime() === today.getTime()) return '今天'
  if (msgDate.getTime() === yesterday.getTime()) return '昨天'
  return date.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric', year: 'numeric' })
}

// Group messages by date
interface MessageGroup {
  label: string
  messages: ChatMessage[]
}

const groupedMessages = computed<MessageGroup[]>(() => {
  const groups: MessageGroup[] = []
  let currentLabel = ''

  for (const msg of props.messages) {
    const label = formatDateGroup(msg.timestamp)
    if (label !== currentLabel) {
      currentLabel = label
      groups.push({ label, messages: [] })
    }
    groups[groups.length - 1].messages.push(msg)
  }

  return groups
})

// ─── 滚动到顶部时加载更多历史消息 ──────────────────────────────────────
function handleScroll({ scrollTop }: { scrollTop: number }) {
  if (scrollTop <= 20 && !isLoadingMore.value && props.conversation?.historyCursor?.hasMore) {
    isLoadingMore.value = true
    emit('load-more', props.conversation.id)
  }
}

// Auto-scroll to bottom when new messages arrive (非加载更多场景)
watch(
  () => props.messages.length,
  async () => {
    if (isLoadingMore.value) {
      isLoadingMore.value = false
      return // 加载更早历史时不滚动到底部
    }
    await nextTick()
    scrollbarRef.value?.wrapRef?.scrollTo({ top: scrollbarRef.value.wrapRef.scrollHeight, behavior: 'smooth' })
  },
)
</script>

<template>
  <section class="message-area">
    <!-- No conversation selected -->
    <ElEmpty
      v-if="!conversation"
      description="选择一个会话开始聊天"
      :image-size="100"
      class="ma-empty"
    />

    <!-- No messages in conversation -->
    <template v-else-if="!hasMessages">
      <div class="ma-header">
        <div class="ma-conv-info">
          <ElAvatar :src="conversation.avatar" :size="40">{{ conversation.title.slice(0, 1) }}</ElAvatar>
          <div>
            <div class="ma-conv-title">
              {{ conversation.title }}
              <span v-if="conversation.conversationType === 'group'" class="ma-group-tag">群</span>
            </div>
            <div class="ma-conv-sub">暂无消息记录</div>
          </div>
        </div>
        <button
          v-if="conversation.conversationType === 'group'"
          class="ma-settings-btn"
          title="群聊设置"
          @click="emit('open-settings')"
        >
          设置
        </button>
      </div>
      <ElEmpty description="还没有任何消息，快说点什么吧" :image-size="90" class="ma-empty" />
    </template>

    <!-- Message list -->
    <template v-else>
      <div class="ma-header">
        <div class="ma-conv-info">
          <ElAvatar :src="conversation.avatar" :size="40">{{ conversation.title.slice(0, 1) }}</ElAvatar>
          <div>
            <div class="ma-conv-title">
              {{ conversation.title }}
              <span v-if="conversation.conversationType === 'group'" class="ma-group-tag">群</span>
            </div>
            <div v-if="typingDisplayName" class="ma-typing">{{ typingDisplayName }} 正在输入…</div>
            <div v-else-if="conversation.conversationType === 'group'" class="ma-conv-sub">
              {{ conversation.memberIds?.length ?? 0 }} 名成员
            </div>
            <div
              v-else
              class="ma-conv-sub"
              :class="conversation.isOnline ? 'ma-conv-sub--online' : ''"
            >
              {{ conversation.isOnline ? '在线' : '离线' }}
            </div>
          </div>
        </div>
        <button
          v-if="conversation.conversationType === 'group'"
          class="ma-settings-btn"
          title="群聊设置"
          @click="emit('open-settings')"
        >
          设置
        </button>
      </div>

      <ElScrollbar ref="scrollbarRef" class="ma-scroll" @scroll="handleScroll">
        <div class="ma-messages">
          <template v-for="group in groupedMessages" :key="group.label">
            <div class="ma-date-divider">
              <span>{{ group.label }}</span>
            </div>

            <template v-for="msg in group.messages" :key="msg.id">
              <div v-if="msg.isSystem" class="ma-system-msg">
                {{ msg.content }}
              </div>

              <div
                v-else
                class="ma-bubble-row"
                :class="msg.isMine ? 'row-mine' : 'row-theirs'"
              >
                <!-- Avatar (only for others) -->
                <ElAvatar
                  v-if="!msg.isMine"
                  :src="msg.senderAvatar"
                  :size="34"
                  class="ma-avatar"
                >
                  {{ msg.senderName.slice(0, 1) }}
                </ElAvatar>

                <!-- Bubble -->
                <div class="ma-bubble-wrap">
                  <div class="ma-sender" :class="msg.isMine ? 'sender-mine' : 'sender-theirs'">
                    {{ msg.isMine ? '我' : msg.senderName }}
                  </div>
                  <div class="ma-bubble" :class="msg.isMine ? 'bubble-mine' : 'bubble-theirs'">
                    {{ msg.content }}
                  </div>
                  <div class="ma-meta" :class="msg.isMine ? 'meta-mine' : 'meta-theirs'">
                    <!-- ACK 状态图标（仅自己的消息） -->
                    <span v-if="msg.isMine && msg.ackStatus === 'pending'" class="ma-ack ma-ack-pending" title="发送中">◌</span>
                    <span v-else-if="msg.isMine && msg.ackStatus === 'failed'" class="ma-ack ma-ack-failed" title="发送失败">⚠</span>
                    <span class="ma-time" :class="msg.isMine ? 'time-mine' : 'time-theirs'">
                      {{ formatTime(msg.timestamp) }}
                    </span>
                  </div>
                  <!-- 发送失败提示 -->
                  <div v-if="msg.isMine && msg.ackStatus === 'failed'" class="ma-failed-hint">
                    发送失败
                  </div>
                </div>
              </div>
            </template>
          </template>
        </div>
      </ElScrollbar>
    </template>
  </section>
</template>

<style scoped>
.message-area {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--aim-bg);
}

/* Header */
.ma-header {
  flex-shrink: 0;
  padding: var(--space-4);
  border-bottom: 1px solid var(--aim-border);
  background: var(--aim-surface);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.ma-conv-info {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
  flex: 1;
}

.ma-settings-btn {
  background: none;
  border: 1px solid var(--aim-border);
  color: var(--aim-text-muted);
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  flex-shrink: 0;
}

.ma-settings-btn:hover {
  border-color: var(--aim-primary);
  color: var(--aim-primary);
}

.ma-group-tag {
  display: inline-block;
  margin-left: 6px;
  font-size: 10px;
  font-weight: 600;
  color: var(--aim-primary);
  background: rgba(0, 212, 170, 0.12);
  border: 1px solid rgba(0, 212, 170, 0.35);
  border-radius: 4px;
  padding: 1px 5px;
  vertical-align: middle;
}

.ma-conv-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--aim-text);
}

.ma-conv-sub {
  font-size: 11px;
  color: var(--aim-text-muted);
  margin-top: 2px;
}

.ma-conv-sub--online {
  color: var(--aim-online);
  font-weight: 500;
}

.ma-typing {
  font-size: 11px;
  color: var(--aim-primary);
  margin-top: 2px;
  animation: typing-blink 1.2s ease-in-out infinite;
}

@keyframes typing-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* Empty */
.ma-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* Scroll area */
.ma-scroll {
  flex: 1;
  overflow: hidden;
}

.ma-messages {
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.ma-system-msg {
  text-align: center;
  font-size: 11px;
  color: var(--aim-text-muted);
  background: var(--aim-surface-2);
  border: 1px solid var(--aim-border);
  border-radius: 10px;
  padding: 6px 12px;
  margin: var(--space-1) auto;
  max-width: 85%;
}

/* Date divider */
.ma-date-divider {
  display: flex;
  align-items: center;
  justify-content: center;
  margin: var(--space-2) 0;
}

.ma-date-divider span {
  font-size: 10px;
  color: var(--aim-text-muted);
  background: var(--aim-surface-2);
  padding: 3px 10px;
  border-radius: 10px;
  border: 1px solid var(--aim-border);
}

/* Bubble rows */
.ma-bubble-row {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
}

.row-mine {
  flex-direction: row-reverse;
}

.row-theirs {
  flex-direction: row;
}

.ma-avatar {
  flex-shrink: 0;
  background: var(--aim-surface-2);
  color: var(--aim-primary);
  font-weight: 700;
  border: 1px solid var(--aim-border);
}

.ma-bubble-wrap {
  display: flex;
  flex-direction: column;
  gap: 3px;
  max-width: 68%;
}

.row-mine .ma-bubble-wrap {
  align-items: flex-end;
}

.row-theirs .ma-bubble-wrap {
  align-items: flex-start;
}

.ma-sender {
  font-size: 10px;
  color: var(--aim-text-muted);
  padding: 0 var(--space-2);
}

.sender-mine { text-align: right; }
.sender-theirs { text-align: left; }

.ma-bubble {
  padding: var(--space-3) var(--space-4);
  border-radius: 12px;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
}

.bubble-mine {
  background: var(--aim-primary);
  color: #0f1419;
  border-bottom-right-radius: 4px;
}

.bubble-theirs {
  background: var(--aim-surface-2);
  color: var(--aim-text);
  border: 1px solid var(--aim-border);
  border-bottom-left-radius: 4px;
}

.ma-meta {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 var(--space-2);
}

.meta-mine {
  justify-content: flex-end;
}

.meta-theirs {
  justify-content: flex-start;
}

.ma-time {
  font-size: 10px;
  color: var(--aim-text-muted);
}

.time-mine { text-align: right; }
.time-theirs { text-align: left; }

/* ACK 状态图标 */
.ma-ack {
  font-size: 12px;
  line-height: 1;
}

.ma-ack-pending {
  color: var(--aim-text-muted);
  opacity: 0.5;
}

.ma-ack-failed {
  color: #f56c6c;
}

.ma-failed-hint {
  font-size: 10px;
  color: #f56c6c;
  padding: 0 var(--space-2);
  text-align: right;
}
</style>