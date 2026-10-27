<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NCard, NForm, NFormItem, NInput } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/client'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  if (!password.value) {
    error.value = '请输入管理口令'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await auth.login(password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '登录失败，请稍后重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-shell">
    <div class="login-visual">
      <div class="brand-mark">SevrTree</div>
      <h1>进程依赖与启动管理</h1>
      <p>开机自动编排启动顺序，还原每个进程的真实启动者与依赖服务。</p>
      <ul>
        <li>进程树 / 启动者 / 命令行全量采集</li>
        <li>nginx、MySQL、Redis 等依赖自动识别</li>
        <li>启动顺序、延迟、参数与重启策略可配置</li>
      </ul>
    </div>

    <div class="login-form">
      <n-card :bordered="false" class="login-card">
        <div class="login-head">
          <h2>管理员登录</h2>
          <p class="muted">仅限本机访问，请使用管理员口令登录</p>
        </div>
        <n-form @submit.prevent>
          <n-form-item label="管理口令">
            <n-input
              v-model:value="password"
              type="password"
              show-password-on="click"
              placeholder="请输入口令"
              size="large"
              :disabled="loading"
              @keyup.enter="submit"
            />
          </n-form-item>
        </n-form>
        <p v-if="error" class="login-error">{{ error }}</p>
        <n-button type="primary" size="large" block :loading="loading" @click="submit">
          登录
        </n-button>
        <p class="login-tip muted">
          首次启动生成的初始口令保存在数据目录的 initial-admin-password.txt
        </p>
      </n-card>
    </div>
  </div>
</template>

<style scoped>
.login-shell {
  display: grid;
  grid-template-columns: 1.15fr 1fr;
  min-height: 100vh;
}

.login-visual {
  padding: 64px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  color: #eaf2ff;
  background:
    radial-gradient(circle at 20% 20%, rgba(79, 156, 249, 0.45), transparent 45%),
    radial-gradient(circle at 80% 80%, rgba(71, 214, 178, 0.28), transparent 45%),
    #0d1524;
}

.brand-mark {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 0.08em;
  color: #4f9cf9;
}

.login-visual h1 {
  font-size: 32px;
  margin: 24px 0 12px;
}

.login-visual p {
  max-width: 420px;
  line-height: 1.7;
  color: #a9bddb;
}

.login-visual ul {
  margin: 24px 0 0;
  padding-left: 18px;
  line-height: 2;
  color: #c7d7f0;
  font-size: 14px;
}

.login-form {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
}

.login-card {
  width: 100%;
  max-width: 380px;
  background: var(--app-bg-soft);
  box-shadow: 0 18px 50px rgba(10, 22, 40, 0.12);
}

.login-head h2 {
  margin: 0 0 6px;
  font-size: 22px;
}

.login-head p {
  margin: 0 0 24px;
  font-size: 13px;
}

.login-error {
  margin: 0 0 12px;
  color: #e5484d;
  font-size: 13px;
}

.login-tip {
  margin: 16px 0 0;
  font-size: 12px;
  line-height: 1.6;
}

@media (max-width: 900px) {
  .login-shell {
    grid-template-columns: 1fr;
  }

  .login-visual {
    display: none;
  }
}
</style>
