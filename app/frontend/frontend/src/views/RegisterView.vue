<script setup lang="ts">
import { reactive } from 'vue'
import { ElForm, ElFormItem, ElInput, ElButton, ElLink } from 'element-plus'

interface RegisterPayload {
  email: string
  password: string
  username: string
  avatar: string
  device_id: string
}

const props = defineProps<{
  loading?: boolean
  deviceId: string
}>()

const emit = defineEmits<{
  register: [payload: RegisterPayload]
  'switch-login': []
}>()

const form = reactive({
  email: '',
  password: '',
  username: '',
  avatar: '',
})

const errors = reactive({
  email: '',
  password: '',
  username: '',
})

function validateEmail(value: string): string {
  if (!value.trim()) {
    return '请输入邮箱地址'
  }
  if (!value.includes('@')) {
    return '邮箱格式不正确'
  }
  return ''
}

function validatePassword(value: string): string {
  if (!value) {
    return '请输入密码'
  }
  if (value.length < 8) {
    return '密码长度至少为 8 个字符'
  }
  return ''
}

function validateUsername(value: string): string {
  if (!value.trim()) {
    return '请输入用户名'
  }
  return ''
}

function handleSubmit() {
  errors.email = validateEmail(form.email)
  errors.password = validatePassword(form.password)
  errors.username = validateUsername(form.username)

  if (errors.email || errors.password || errors.username) {
    return
  }

  emit('register', {
    email: form.email.trim(),
    password: form.password,
    username: form.username.trim(),
    avatar: form.avatar.trim(),
    device_id: props.deviceId,
  })
}

function handleSwitchLogin() {
  emit('switch-login')
}
</script>

<template>
  <div class="auth-card">
    <div class="auth-brand">
      <div class="brand-logo">AIM</div>
      <h2 class="brand-title">创建账号</h2>
      <p class="brand-subtitle">加入 AIM，开始智能通讯之旅</p>
    </div>

    <ElForm class="auth-form" @submit.prevent="handleSubmit">
      <ElFormItem :label="'用户名'" :error="errors.username">
        <ElInput
          v-model="form.username"
          type="text"
          placeholder="请输入用户名"
          autocomplete="username"
          @blur="errors.username = validateUsername(form.username)"
        />
      </ElFormItem>

      <ElFormItem :label="'邮箱'" :error="errors.email">
        <ElInput
          v-model="form.email"
          type="email"
          placeholder="请输入邮箱地址"
          autocomplete="email"
          @blur="errors.email = validateEmail(form.email)"
        />
      </ElFormItem>

      <ElFormItem :label="'密码'" :error="errors.password">
        <ElInput
          v-model="form.password"
          type="password"
          placeholder="请输入密码（至少 8 个字符）"
          show-password
          autocomplete="new-password"
          @blur="errors.password = validatePassword(form.password)"
        />
      </ElFormItem>

      <ElFormItem :label="'头像 URL（可选）'">
        <ElInput
          v-model="form.avatar"
          type="text"
          placeholder="请输入头像图片地址"
          autocomplete="photo"
        />
      </ElFormItem>

      <ElFormItem>
        <ElButton
          type="primary"
          class="submit-btn"
          :loading="loading"
          @click="handleSubmit"
        >
          注册
        </ElButton>
      </ElFormItem>
    </ElForm>

    <div class="auth-footer">
      <span class="footer-text">已有账号？</span>
      <ElLink type="primary" @click="handleSwitchLogin">
        立即登录
      </ElLink>
    </div>
  </div>
</template>

<style scoped>
.auth-card {
  width: 100%;
  max-width: 380px;
  margin: 0 auto;
  padding: 40px 32px;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.auth-brand {
  text-align: center;
  margin-bottom: 32px;
}

.brand-logo {
  width: 56px;
  height: 56px;
  display: grid;
  place-items: center;
  border: 1px solid var(--aim-primary);
  color: var(--aim-primary);
  font-weight: 900;
  letter-spacing: 0.12em;
  background: rgba(0, 212, 170, 0.08);
  margin: 0 auto 16px;
  font-size: 18px;
}

.brand-title {
  font-size: 22px;
  font-weight: 700;
  color: var(--aim-text);
  margin: 0 0 6px;
}

.brand-subtitle {
  font-size: 13px;
  color: var(--aim-text-muted);
  margin: 0;
}

.auth-form {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.auth-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.auth-form :deep(.el-form-item__label) {
  color: var(--aim-text-muted);
  font-size: 12px;
  font-weight: 500;
  padding-bottom: 6px;
}

.auth-form :deep(.el-input__wrapper) {
  padding: 8px 12px;
}

.auth-form :deep(.el-input__inner) {
  font-size: 13px;
}

.submit-btn {
  width: 100%;
  height: 40px;
  font-size: 14px;
  margin-top: 8px;
}

.auth-footer {
  margin-top: 24px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.footer-text {
  font-size: 13px;
  color: var(--aim-text-muted);
}
</style>