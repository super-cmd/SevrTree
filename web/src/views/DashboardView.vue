<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NProgress,
  NSpace,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { api, ApiError } from '@/api/client'
import type { ServiceNode, Snapshot, SystemPayload, TaskListPayload, Task } from '@/types'
import { formatBytes, formatClock, formatDuration, formatTime, serviceKindType, statusMeta } from '@/utils'

const message = useMessage()
const dialog = useDialog()
const router = useRouter()

const system = ref<SystemPayload | null>(null)
const snapshot = ref<Snapshot | null>(null)
const tasks = ref<Task[]>([])
const loading = ref(true)
const busy = ref(false)
const lastError = ref('')

let timer: number | undefined
let refreshing = false

const runningTasks = computed(() => tasks.value.filter((t) => t.runtime && ['running', 'starting', 'stopping'].includes(t.runtime.status)))
const autostartCount = computed(() => tasks.value.filter((t) => t.enabled && t.autoStart).length)
const detectedServices = computed(() => snapshot.value?.services ?? [])
const sequence = computed(() => system.value?.sequence)

async function refresh() {
  if (refreshing) {
    return
  }
  refreshing = true
  try {
    const [sys, procs, taskPayload] = await Promise.all([
      api.get<SystemPayload>('/api/system'),
      api.get<Snapshot>('/api/processes'),
      api.get<TaskListPayload>('/api/tasks'),
    ])
    system.value = sys
    snapshot.value = procs
    tasks.value = taskPayload.tasks
    lastError.value = ''
  } catch (err) {
    lastError.value = err instanceof ApiError ? err.message : '数据刷新失败'
  } finally {
    loading.value = false
    refreshing = false
  }
}

async function runSequence(action: 'start-all' | 'stop-all') {
  busy.value = true
  try {
    await api.post(`/api/tasks/${action}`, { boot: false })
    message.success(action === 'start-all' ? '已开始按顺序启动' : '已开始按顺序停止')
    await refresh()
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    busy.value = false
  }
}

function confirmStopAll() {
  dialog.warning({
    title: '停止全部任务',
    content: '将按反向依赖顺序停止所有运行中的任务，是否继续？',
    positiveText: '停止',
    negativeText: '取消',
    onPositiveClick: () => runSequence('stop-all'),
  })
}

const serviceColumns: DataTableColumns<ServiceNode> = [
  {
    title: '服务',
    key: 'name',
    minWidth: 160,
    render: (row) =>
      h('div', { style: 'display:flex;align-items:center;gap:8px' }, [
        h('span', { style: 'font-weight:600' }, row.name),
        row.managed ? h(NTag, { size: 'small', type: 'success', bordered: false }, { default: () => '托管' }) : null,
      ]),
  },
  {
    title: '类别',
    key: 'kind',
    width: 120,
    render: (row) => h(NTag, { size: 'small', type: serviceKindType(row.kind), bordered: false }, { default: () => row.kind }),
  },
  {
    title: '端口',
    key: 'ports',
    minWidth: 120,
    render: (row) => (row.ports.length ? row.ports.join(', ') : '—'),
  },
  {
    title: 'PID',
    key: 'pids',
    minWidth: 110,
    render: (row) => row.pids.join(', '),
  },
  {
    title: '依赖方',
    key: 'clients',
    width: 100,
    render: (row) => `${row.clients} 个进程`,
  },
]

const taskColumns: DataTableColumns<Task> = [
  {
    title: '任务',
    key: 'name',
    minWidth: 160,
    render: (row) => h('span', { style: 'font-weight:600' }, row.name),
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) => {
      const meta = statusMeta(row.runtime?.status)
      return h(NTag, { size: 'small', type: meta.type, bordered: false }, { default: () => meta.text })
    },
  },
  {
    title: 'PID',
    key: 'pid',
    width: 90,
    render: (row) => (row.runtime?.pid ? String(row.runtime.pid) : '—'),
  },
  {
    title: '延迟',
    key: 'delaySeconds',
    width: 90,
    render: (row) => `${row.delaySeconds}s`,
  },
  {
    title: '开机自启',
    key: 'autoStart',
    width: 100,
    render: (row) => (row.autoStart ? '是' : '否'),
  },
  {
    title: '启动入口',
    key: 'exe',
    minWidth: 260,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'mono' }, row.exe + (row.args.length ? ' ' + row.args.join(' ') : '')),
  },
]

onMounted(async () => {
  await refresh()
  timer = window.setInterval(refresh, 4000)
})

onUnmounted(() => {
  if (timer) {
    window.clearInterval(timer)
  }
})
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h2 class="page-title">运行总览</h2>
        <p class="page-subtitle">实时掌握系统进程、依赖服务与托管任务状态</p>
      </div>
      <n-space>
        <n-button :loading="loading" @click="refresh">刷新</n-button>
        <n-button type="primary" :loading="busy" :disabled="sequence?.running" @click="runSequence('start-all')">
          按序启动全部
        </n-button>
        <n-button type="error" ghost :disabled="sequence?.running" @click="confirmStopAll">按序停止全部</n-button>
      </n-space>
    </div>

    <n-alert v-if="lastError" type="error" :bordered="false" style="margin-bottom: 16px">{{ lastError }}</n-alert>
    <n-alert v-if="system && !system.system.elevated" type="warning" :bordered="false" style="margin-bottom: 16px">
      当前以普通权限运行，部分高权限进程的命令行可能无法读取；安装开机自启服务需要管理员权限。
    </n-alert>
    <n-alert
      v-if="sequence?.running"
      type="info"
      :bordered="false"
      style="margin-bottom: 16px"
      :title="sequence.message"
    >
      {{ sequence.current || '编排执行中' }} · 成功 {{ sequence.started }} · 失败/跳过 {{ sequence.failed }}
      <n-progress type="line" :percentage="100" :processing="true" :show-indicator="false" style="margin-top: 10px" />
    </n-alert>

    <div class="stats">
      <div class="stat-card">
        <div class="stat-label">进程总数</div>
        <div class="stat-value">{{ snapshot?.total ?? '—' }}</div>
        <div class="stat-foot">采集于 {{ formatClock(snapshot?.collectedAt) }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">识别到的依赖服务</div>
        <div class="stat-value">{{ detectedServices.length }}</div>
        <div class="stat-foot">nginx / MySQL / Redis 等</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">运行中的托管任务</div>
        <div class="stat-value">{{ runningTasks.length }} / {{ tasks.length }}</div>
        <div class="stat-foot">开机自启 {{ autostartCount }} 个</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">内存使用</div>
        <div class="stat-value">{{ system ? system.system.memPercent.toFixed(1) + '%' : '—' }}</div>
        <div class="stat-foot">
          {{ system ? formatBytes(system.system.memUsed) + ' / ' + formatBytes(system.system.memTotal) : '—' }}
        </div>
      </div>
    </div>

    <div class="panels">
      <n-card title="识别到的依赖服务" :bordered="false" class="panel">
        <n-data-table
          size="small"
          :columns="serviceColumns"
          :data="detectedServices"
          :bordered="false"
          :max-height="320"
          :row-key="(row: ServiceNode) => row.key"
          :locale="{ emptyText: '暂未识别到数据库、缓存或 Web 服务' }"
        />
      </n-card>

      <n-card title="托管任务" :bordered="false" class="panel">
        <template #header-extra>
          <n-button size="small" quaternary type="primary" @click="router.push('/tasks')">管理任务</n-button>
        </template>
        <n-data-table
          size="small"
          :columns="taskColumns"
          :data="tasks"
          :bordered="false"
          :max-height="320"
          :row-key="(row: Task) => row.id"
          :locale="{ emptyText: '尚未配置托管任务' }"
        />
      </n-card>
    </div>

    <n-card v-if="system" title="运行环境" :bordered="false" style="margin-top: 16px">
      <div class="env-grid">
        <div>
          <div class="env-label">主机</div>
          <div class="env-value">{{ system.system.hostname }} · {{ system.system.platform }} {{ system.system.platformVersion }}</div>
        </div>
        <div>
          <div class="env-label">运行时长</div>
          <div class="env-value">{{ formatDuration(system.system.uptimeSeconds) }}</div>
        </div>
        <div>
          <div class="env-label">CPU 核心</div>
          <div class="env-value">{{ system.system.cpuCount }} 核</div>
        </div>
        <div>
          <div class="env-label">开机自启服务</div>
          <div class="env-value">
            <n-tag size="small" :type="system.autostart.installed ? 'success' : 'default'" :bordered="false">
              {{ system.autostart.installed ? system.autostart.state : '未安装' }}
            </n-tag>
          </div>
        </div>
        <div>
          <div class="env-label">数据目录</div>
          <div class="env-value mono ellipsis">{{ system.app.dataDir }}</div>
        </div>
        <div>
          <div class="env-label">服务启动时间</div>
          <div class="env-value">{{ formatTime(system.app.startedAt) }}</div>
        </div>
      </div>
    </n-card>
  </div>
</template>

<style scoped>
.stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.panels {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr);
  gap: 16px;
  margin-top: 16px;
}

.panel {
  min-width: 0;
}

.env-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px 24px;
}

.env-label {
  font-size: 12px;
  color: var(--app-text-muted);
  margin-bottom: 4px;
}

.env-value {
  font-size: 13px;
}

@media (max-width: 1180px) {
  .stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .panels,
  .env-grid {
    grid-template-columns: 1fr;
  }
}
</style>
