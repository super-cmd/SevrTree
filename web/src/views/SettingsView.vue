<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDescriptions,
  NDescriptionsItem,
  NForm,
  NFormItem,
  NInput,
  NSpace,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'
import { api, ApiError } from '@/api/client'
import type { AuditLog, SystemPayload } from '@/types'
import { formatBytes, formatDuration, formatTime } from '@/utils'
import { useAuthStore } from '@/stores/auth'

const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()

const system = ref<SystemPayload | null>(null)
const audits = ref<AuditLog[]>([])
const loading = ref(true)
const autostartBusy = ref(false)
const lastError = ref('')

const passwordForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' })
const passwordRef = ref<FormInst | null>(null)
const passwordBusy = ref(false)

const passwordRules: FormRules = {
  oldPassword: { required: true, message: '请输入当前口令', trigger: ['input', 'blur'] },
  newPassword: {
    required: true,
    min: 8,
    message: '新口令至少 8 位',
    trigger: ['input', 'blur'],
  },
  confirmPassword: {
    validator: (_rule, value: string) => {
      if (value !== passwordForm.value.newPassword) {
        return new Error('两次输入的口令不一致')
      }
      return true
    },
    trigger: ['input', 'blur'],
  },
}

async function refresh() {
  try {
    const [sys, audit] = await Promise.all([
      api.get<SystemPayload>('/api/system'),
      api.get<{ logs: AuditLog[] }>('/api/audit?limit=100'),
    ])
    system.value = sys
    audits.value = audit.logs
    lastError.value = ''
  } catch (err) {
    lastError.value = err instanceof ApiError ? err.message : '读取设置信息失败'
  } finally {
    loading.value = false
  }
}

async function autostartAction(action: 'install' | 'uninstall' | 'start' | 'stop') {
  autostartBusy.value = true
  try {
    await api.post('/api/system/autostart', { action })
    message.success('操作已执行')
    await refresh()
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    autostartBusy.value = false
  }
}

function confirmUninstall() {
  dialog.warning({
    title: '卸载开机自启',
    content: '将停止并移除 SevrTree 的 Windows 服务，之后系统重启不再自动拉起本程序。',
    positiveText: '卸载',
    negativeText: '取消',
    onPositiveClick: () => autostartAction('uninstall'),
  })
}

async function changePassword() {
  try {
    await passwordRef.value?.validate()
  } catch {
    return
  }
  passwordBusy.value = true
  try {
    await api.post('/api/auth/password', {
      oldPassword: passwordForm.value.oldPassword,
      newPassword: passwordForm.value.newPassword,
    })
    passwordForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
    message.success('口令已修改，请重新登录')
    auth.authenticated = false
    await auth.logout()
    window.location.href = '/login'
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '修改口令失败')
  } finally {
    passwordBusy.value = false
  }
}

const auditColumns: DataTableColumns<AuditLog> = [
  { title: '时间', key: 'createdAt', width: 170, render: (row) => formatTime(row.createdAt) },
  {
    title: '操作',
    key: 'action',
    width: 110,
    render: (row) => h(NTag, { size: 'small', bordered: false, type: 'info' }, { default: () => row.action }),
  },
  { title: '对象', key: 'target', minWidth: 140, ellipsis: { tooltip: true }, render: (row) => row.target || '—' },
  { title: '操作人', key: 'username', width: 100 },
  { title: '来源 IP', key: 'ip', width: 130, render: (row) => h('span', { class: 'mono' }, row.ip || '—') },
  { title: '说明', key: 'detail', minWidth: 180, ellipsis: { tooltip: true }, render: (row) => row.detail || '—' },
]

onMounted(refresh)
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h2 class="page-title">设置与审计</h2>
        <p class="page-subtitle">开机自启、管理员口令与操作审计</p>
      </div>
      <n-button :loading="loading" @click="refresh">刷新</n-button>
    </div>

    <n-alert v-if="lastError" type="error" :bordered="false" style="margin-bottom: 16px">{{ lastError }}</n-alert>
    <n-alert
      v-if="system && !system.system.elevated"
      type="warning"
      :bordered="false"
      style="margin-bottom: 16px"
    >
      安装或卸载开机自启服务需要以管理员身份运行 SevrTree，否则系统会拒绝写入服务配置。
    </n-alert>

    <div class="settings-grid">
      <n-card title="开机自启服务" :bordered="false">
        <p class="tip muted">
          SevrTree 通过 nssm 注册为 Windows 服务，随系统启动后自动按配置拉起托管任务。
          请将 nssm.exe 放在程序同目录，或加入系统 PATH。
        </p>
        <n-descriptions :column="2" size="small" bordered label-placement="left" style="margin-bottom: 16px">
          <n-descriptions-item label="服务名">{{ system?.autostart.serviceName ?? 'SevrTree' }}</n-descriptions-item>
          <n-descriptions-item label="安装状态">
            <n-tag size="small" :type="system?.autostart.installed ? 'success' : 'default'" :bordered="false">
              {{ system?.autostart.installed ? '已安装' : '未安装' }}
            </n-tag>
          </n-descriptions-item>
          <n-descriptions-item label="运行状态">{{ system?.autostart.state || '—' }}</n-descriptions-item>
          <n-descriptions-item label="启动类型">SERVICE_AUTO_START</n-descriptions-item>
        </n-descriptions>
        <n-space>
          <n-button
            v-if="!system?.autostart.installed"
            type="primary"
            :loading="autostartBusy"
            @click="autostartAction('install')"
          >
            安装为开机自启
          </n-button>
          <template v-else>
            <n-button :loading="autostartBusy" @click="autostartAction('start')">启动服务</n-button>
            <n-button :loading="autostartBusy" @click="autostartAction('stop')">停止服务</n-button>
            <n-button type="error" ghost :loading="autostartBusy" @click="confirmUninstall">卸载自启</n-button>
          </template>
        </n-space>
      </n-card>

      <n-card title="修改管理员口令" :bordered="false">
        <n-form ref="passwordRef" :model="passwordForm" :rules="passwordRules" label-placement="top">
          <n-form-item label="当前口令" path="oldPassword">
            <n-input v-model:value="passwordForm.oldPassword" type="password" show-password-on="click" />
          </n-form-item>
          <n-form-item label="新口令" path="newPassword">
            <n-input v-model:value="passwordForm.newPassword" type="password" show-password-on="click" />
          </n-form-item>
          <n-form-item label="确认新口令" path="confirmPassword">
            <n-input v-model:value="passwordForm.confirmPassword" type="password" show-password-on="click" />
          </n-form-item>
        </n-form>
        <n-button type="primary" :loading="passwordBusy" @click="changePassword">修改口令</n-button>
        <p class="tip muted">修改成功后全部会话失效，需要使用新口令重新登录。</p>
      </n-card>
    </div>

    <n-card v-if="system" title="运行信息" :bordered="false" style="margin-top: 16px">
      <n-descriptions :column="3" size="small" bordered label-placement="left">
        <n-descriptions-item label="程序版本">{{ system.app.version }}</n-descriptions-item>
        <n-descriptions-item label="监听地址">{{ system.app.addr }}</n-descriptions-item>
        <n-descriptions-item label="已运行">{{ formatDuration(system.app.uptimeSeconds) }}</n-descriptions-item>
        <n-descriptions-item label="启动时间">{{ formatTime(system.app.startedAt) }}</n-descriptions-item>
        <n-descriptions-item label="数据目录" :span="2">{{ system.app.dataDir }}</n-descriptions-item>
        <n-descriptions-item label="日志目录" :span="2">{{ system.app.logDir }}</n-descriptions-item>
        <n-descriptions-item label="系统">{{ system.system.platform }} {{ system.system.platformVersion }}</n-descriptions-item>
        <n-descriptions-item label="内存">
          {{ formatBytes(system.system.memUsed) }} / {{ formatBytes(system.system.memTotal) }}
        </n-descriptions-item>
        <n-descriptions-item label="主机名">{{ system.system.hostname }}</n-descriptions-item>
      </n-descriptions>
    </n-card>

    <n-card title="操作审计" :bordered="false" style="margin-top: 16px">
      <n-data-table
        size="small"
        :columns="auditColumns"
        :data="audits"
        :bordered="false"
        :max-height="360"
        :row-key="(row: AuditLog) => row.id"
        :locale="{ emptyText: '暂无操作记录' }"
      />
    </n-card>
  </div>
</template>

<style scoped>
.settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.tip {
  font-size: 12px;
  line-height: 1.7;
  margin: 0 0 16px;
}

@media (max-width: 1080px) {
  .settings-grid {
    grid-template-columns: 1fr;
  }
}
</style>
