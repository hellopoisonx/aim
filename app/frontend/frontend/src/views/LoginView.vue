<script setup lang="ts">
import { reactive } from 'vue'
import { ElForm, ElFormItem, ElInput, ElButton, ElLink } from 'element-plus'

interface LoginPayload {
  email: string
  password: string
  device_id: string
}

const props = defineProps<{
  loading?: boolean
  deviceId: string
}>()

const emit = defineEmits<{
  login: [payload: LoginPayload]
  'switch-register': []
}>()

const form = reactive({
  email: '',
  password: '',
})

const errors = reactive({
  email: '',
  password: '',
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

function handleSubmit() {
  errors.email = validateEmail(form.email)
  errors.password = validatePassword(form.password)

  if (errors.email || errors.password) {
    return
  }

  emit('login', {
    email: form.email.trim(),
    password: form.password,
    device_id: props.deviceId,
  })
}

function handleSwitchRegister() {
  emit('switch-register')
}
</script>

<template>
  <div class="auth-card">
    <div class="auth-brand">
      <div class="brand-logo">AIM</div>
      <h2 class="brand-title">欢迎回来</h2>
      <p class="brand-subtitle">登录您的 AIM 账号</p>
    </div>

    <ElForm class="auth-form" @submit.prevent="handleSubmit">
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
          placeholder="请输入密码"
          show-password
          autocomplete="current-password"
          @blur="errors.password = validatePassword(form.password)"
        />
      </ElFormItem>

      <ElFormItem>
        <ElButton
          type="primary"
          class="submit-btn"
          :loading="loading"
          @click="handleSubmit"
        >
          登录
        </ElButton>
      </ElFormItem>
    </ElForm>

    <div class="auth-footer">
      <span class="footer-text">还没有账号？</span>
      <ElLink type="primary" @click="handleSwitchRegister">
        立即注册
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