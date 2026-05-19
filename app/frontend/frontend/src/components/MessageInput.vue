<script lang="ts" setup>
import { onUnmounted, ref } from 'vue'
import { ElButton, ElInput } from 'element-plus'

interface Props {
  disabled?: boolean
  placeholder?: string
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  placeholder: '输入消息…',
})

const emit = defineEmits<{
  send: [content: string]
  typing: []
}>()

const inputValue = ref('')

let typingTimer: ReturnType<typeof setTimeout> | null = null

function onInput() {
  emit('typing')
  if (typingTimer) clearTimeout(typingTimer)
  typingTimer = setTimeout(() => {
    // typing stopped
  }, 2000)
}

function onSend() {
  const trimmed = inputValue.value.trim()
  if (!trimmed || props.disabled) return
  emit('send', trimmed)
  inputValue.value = ''
  if (typingTimer) clearTimeout(typingTimer)
}

function onKeydown(e: Event | KeyboardEvent) {
  if (!(e instanceof KeyboardEvent)) return
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    onSend()
  }
}

onUnmounted(() => {
  if (typingTimer) clearTimeout(typingTimer)
})
</script>

<template>
  <div class="message-input">
    <ElInput
      v-model="inputValue"
      type="textarea"
      :placeholder="placeholder"
      :disabled="disabled"
      :rows="3"
      resize="none"
      class="mi-textarea"
      @input="onInput"
      @keydown="onKeydown"
    />
    <div class="mi-actions">
      <span class="mi-hint">Enter 发送，Shift+Enter 换行</span>
      <ElButton
        type="primary"
        :disabled="disabled || !inputValue.trim()"
        class="mi-send-btn"
        @click="onSend"
      >
        发送
      </ElButton>
    </div>
  </div>
</template>

<style scoped>
.message-input {
  flex-shrink: 0;
  padding: var(--space-4);
  border-top: 1px solid var(--aim-border);
  background: var(--aim-surface);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.mi-textarea :deep(.el-textarea__inner) {
  background: var(--aim-bg) !important;
  border: 1px solid var(--aim-border) !important;
  color: var(--aim-text) !important;
  font-family: var(--font-ui) !important;
  font-size: 13px !important;
  border-radius: 8px !important;
  padding: var(--space-3) !important;
  line-height: 1.5 !important;
}

.mi-textarea :deep(.el-textarea__inner:focus) {
  border-color: var(--aim-primary) !important;
  box-shadow: 0 0 0 2px rgba(0, 212, 170, 0.15) !important;
}

.mi-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.mi-hint {
  font-size: 10px;
  color: var(--aim-text-muted);
}

.mi-send-btn {
  min-width: 72px;
}
</style>
