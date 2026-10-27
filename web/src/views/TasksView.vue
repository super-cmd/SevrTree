<script setup lang="ts">
import { computed, h, nextTick, onMounted, onUnmounted, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
  NGrid,
  NGi,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'
import { api, ApiError } from '@/api/client'
import type { LogLine, SequenceState, Task, TaskListPayload } from '@/types'
import { formatTime, statusMeta } from '@/utils'

interface TaskForm {
  id: number | null
  name: string
  exe: string
  argsText: string
  workDir: string
  envText: string
  delaySeconds: number
  sortOrder: number
  enabled: boolean
  autoStart: boolean
  dependsOn: number[]
  healthPort: number
  stopTimeoutSeconds: number
  restartOnExit: boolean
  maxRestarts: number
}

const message = useMessage()
const dialog = useDialog()

const tasks = ref<Task[]>([])
const sequence = ref<SequenceState | null>(null)
const loading = ref(true)
const saving = ref(false)
const lastError = ref('')
const busyTaskIds = ref<number[]>([])

const showForm = ref(false)
const formRef = ref<FormInst | null>(null)
const form = ref<TaskForm>(emptyForm())

const showLogs = ref(false)
const logTask = ref<Task | null>(null)
const logLines = ref<LogLine[]>([])
const logBox = ref<HTMLElement | null>(null)

let timer: number | undefined
let logTimer: number | undefined
let inFlight = false

const rules: FormRules = {
  name: { required: true, message: '请输入任务名称', trigger: ['input', 'blur'] },
  exe: { required: true, message: '请输入可执行文件或脚本路径', trigger: ['input', 'blur'] },
}

const dependencyOptions = computed(() =>
  tasks.value
    .filter((t) => t.id !== form.value.id)
    .map((t) => ({ label: `${t.name}（#${t.id}）`, value: t.id })),
)

function emptyForm(): TaskForm {
  return {
    id: null,
    name: '',
    exe: '',
    argsText: '',
    workDir: '',
    envText: '',
    delaySeconds: 0,
    sortOrder: 0,
    enabled: true,
    autoStart: true,
    dependsOn: [],
    healthPort: 0,
    stopTimeoutSeconds: 15,
    restartOnExit: false,
    maxRestarts: 3,
  }
}

async function refresh() {
  if (inFlight) {
    return
  }
  inFlight = true
  try {
    const payload = await api.get<TaskListPayload>('/api/tasks')
    tasks.value = payload.tasks
    sequence.value = payload.sequence
    lastError.value = ''
  } catch (err) {
    lastError.value = err instanceof ApiError ? err.message : '读取任务失败'
  } finally {
    loading.value = false
    inFlight = false
  }
}

function openCreate() {
  form.value = emptyForm()
  form.value.sortOrder = tasks.value.length
  showForm.value = true
}

function openEdit(task: Task) {
  form.value = {
    id: task.id,
    name: task.name,
    exe: task.exe,
    argsText: task.args.join('\n'),
    workDir: task.workDir,
    envText: Object.entries(task.env)
      .map(([k, v]) => `${k}=${v}`)
      .join('\n'),
    delaySeconds: task.delaySeconds,
    sortOrder: task.sortOrder,
    enabled: task.enabled,
    autoStart: task.autoStart,
    dependsOn: [...task.dependsOn],
    healthPort: task.healthPort,
    stopTimeoutSeconds: task.stopTimeoutSeconds,
    restartOnExit: task.restartOnExit,
    maxRestarts: task.maxRestarts,
  }
  showForm.value = true
}

async function save() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  const args = form.value.argsText
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)

  const env: Record<string, string> = {}
  for (const line of form.value.envText.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed) {
      continue
    }
    const index = trimmed.indexOf('=')
    if (index <= 0) {
      message.error(`环境变量格式错误：${trimmed}`)
      return
    }
    env[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1)
  }

  const payload = {
    name: form.value.name.trim(),
    exe: form.value.exe.trim(),
    args,
    workDir: form.value.workDir.trim(),
    env,
    delaySeconds: form.value.delaySeconds,
    sortOrder: form.value.sortOrder,
    enabled: form.value.enabled,
    autoStart: form.value.autoStart,
    dependsOn: form.value.dependsOn,
    healthPort: form.value.healthPort,
    stopTimeoutSeconds: form.value.stopTimeoutSeconds,
    restartOnExit: form.value.restartOnExit,
    maxRestarts: form.value.maxRestarts,
  }

  saving.value = true
  try {
    if (form.value.id) {
      await api.put(`/api/tasks/${form.value.id}`, payload)
      message.success('任务已更新')
    } else {
      await api.post('/api/tasks', payload)
      message.success('任务已创建')
    }
    showForm.value = false
    await refresh()
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

function markBusy(id: number, busy: boolean) {
  if (busy) {
    if (!busyTaskIds.value.includes(id)) {
      busyTaskIds.value = [...busyTaskIds.value, id]
    }
  } else {
    busyTaskIds.value = busyTaskIds.value.filter((item) => item !== id)
  }
}

async function taskAction(task: Task, action: 'start' | 'stop' | 'restart') {
  markBusy(task.id, true)
  try {
    await api.post(`/api/tasks/${task.id}/${action}`)
    message.success(`${task.name} ${action === 'start' ? '已启动' : action === 'stop' ? '已停止' : '已重启'}`)
    await refresh()
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    markBusy(task.id, false)
  }
}

function confirmDelete(task: Task) {
  dialog.warning({
    title: '删除任务',
    content: `确定删除「${task.name}」？运行中的任务会先被停止。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.del(`/api/tasks/${task.id}`)
        message.success('任务已删除')
        await refresh()
      } catch (err) {
        message.error(err instanceof ApiError ? err.message : '删除失败')
      }
    },
  })
}

async function move(task: Task, direction: -1 | 1) {
  const list = [...tasks.value]
  const index = list.findIndex((item) => item.id === task.id)
  const target = index + direction
  if (index < 0 || target < 0 || target >= list.length) {
    return
  }
  const [moved] = list.splice(index, 1)
  list.splice(target, 0, moved)
  tasks.value = list
  try {
    await api.post('/api/tasks/reorder', { ids: list.map((item) => item.id) })
    await refresh()
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '调整顺序失败')
  }
}

async function runSequence(action: 'start-all' | 'stop-all') {
  try {
    await api.post(`/api/tasks/${action}`, { boot: false })
    message.success(action === 'start-all' ? '已开始按依赖顺序启动' : '已开始按反向顺序停止')
    await refresh()
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '操作失败')
  }
}

async function loadLogs() {
  if (!logTask.value) {
    return
  }
  try {
    const data = await api.get<{ lines: LogLine[] }>(`/api/tasks/${logTask.value.id}/logs?lines=400`)
    logLines.value = data.lines
    await nextTick()
    if (logBox.value) {
      logBox.value.scrollTop = logBox.value.scrollHeight
    }
  } catch {
    // 轮询失败不打断用户操作
  }
}

function openLogs(task: Task) {
  logTask.value = task
  logLines.value = []
  showLogs.value = true
  void loadLogs()
  logTimer = window.setInterval(loadLogs, 1500)
}

function stopLogPolling() {
  if (logTimer) {
    window.clearInterval(logTimer)
    logTimer = undefined
  }
  logTask.value = null
}

const dependencyNames = (task: Task): string[] =>
  task.dependsOn.map((id) => tasks.value.find((item) => item.id === id)?.name ?? `#${id}`)

const columns: DataTableColumns<Task> = [
  {
    title: '顺序',
    key: 'sortOrder',
    width: 120,
    render: (row) =>
      h('div', { style: 'display:flex;align-items:center;gap:2px' }, [
        h('span', { class: 'mono', style: 'width:26px' }, String(row.sortOrder)),
        h(
          NButton,
          { size: 'tiny', quaternary: true, disabled: tasks.value[0]?.id === row.id, onClick: () => move(row, -1) },
          { default: () => '↑' },
        ),
        h(
          NButton,
          {
            size: 'tiny',
            quaternary: true,
            disabled: tasks.value[tasks.value.length - 1]?.id === row.id,
            onClick: () => move(row, 1),
          },
          { default: () => '↓' },
        ),
      ]),
  },
  {
    title: '任务',
    key: 'name',
    minWidth: 160,
    render: (row) =>
      h('div', [
        h('div', { style: 'font-weight:600' }, row.name),
        h('div', { class: 'task-sub' }, `${row.autoStart ? '开机自启' : '手动启动'} · 延迟 ${row.delaySeconds}s`),
      ]),
  },
  {
    title: '启动入口',
    key: 'exe',
    minWidth: 280,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'mono' }, row.exe + (row.args.length ? ' ' + row.args.join(' ') : '')),
  },
  {
    title: '依赖',
    key: 'dependsOn',
    minWidth: 160,
    render: (row) => {
      const names = dependencyNames(row)
      if (!names.length) {
        return '—'
      }
      return h(
        'div',
        { style: 'display:flex;gap:4px;flex-wrap:wrap' },
        names.map((name) => h(NTag, { size: 'tiny', bordered: false }, { default: () => name })),
      )
    },
  },
  {
    title: '状态',
    key: 'status',
    width: 150,
    render: (row) => {
      const meta = statusMeta(row.runtime?.status)
      return h('div', { style: 'display:flex;flex-direction:column;gap:2px' }, [
        h(NTag, { size: 'small', type: meta.type, bordered: false }, { default: () => meta.text }),
        row.runtime?.pid ? h('span', { class: 'task-sub mono' }, `PID ${row.runtime.pid}`) : null,
      ])
    },
  },
  {
    title: '端口',
    key: 'healthPort',
    width: 90,
    render: (row) => (row.healthPort ? String(row.healthPort) : '—'),
  },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    render: (row) => {
      const busy = busyTaskIds.value.includes(row.id)
      const running = row.runtime?.status === 'running' || row.runtime?.status === 'starting'
      return h(
        NSpace,
        { size: 4 },
        {
          default: () => [
            running
              ? h(
                  NButton,
                  { size: 'tiny', type: 'warning', ghost: true, loading: busy, onClick: () => taskAction(row, 'stop') },
                  { default: () => '停止' },
                )
              : h(
                  NButton,
                  { size: 'tiny', type: 'primary', ghost: true, loading: busy, onClick: () => taskAction(row, 'start') },
                  { default: () => '启动' },
                ),
            h(
              NButton,
              { size: 'tiny', quaternary: true, disabled: busy, onClick: () => taskAction(row, 'restart') },
              { default: () => '重启' },
            ),
            h(NButton, { size: 'tiny', quaternary: true, onClick: () => openLogs(row) }, { default: () => '日志' }),
            h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
            h(
              NButton,
              { size: 'tiny', quaternary: true, type: 'error', onClick: () => confirmDelete(row) },
              { default: () => '删除' },
            ),
          ],
        },
      )
    },
  },
]

onMounted(async () => {
  await refresh()
  timer = window.setInterval(refresh, 5000)
})

onUnmounted(() => {
  if (timer) {
    window.clearInterval(timer)
  }
  stopLogPolling()
})
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h2 class="page-title">托管任务</h2>
        <p class="page-subtitle">配置启动顺序、延迟、参数与依赖关系，随程序开机自动编排</p>
      </div>
      <n-space>
        <n-button type="primary" @click="openCreate">新建任务</n-button>
        <n-button :loading="loading" @click="refresh">刷新</n-button>
        <n-button :disabled="sequence?.running" @click="runSequence('start-all')">按序启动</n-button>
        <n-button ghost type="error" :disabled="sequence?.running" @click="runSequence('stop-all')">按序停止</n-button>
      </n-space>
    </div>

    <n-alert v-if="lastError" type="error" :bordered="false" style="margin-bottom: 16px">{{ lastError }}</n-alert>
    <n-alert v-if="sequence?.running" type="info" :bordered="false" style="margin-bottom: 16px" :title="sequence.message">
      {{ sequence.current }} · 成功 {{ sequence.started }} · 失败/跳过 {{ sequence.failed }}
    </n-alert>

    <n-card :bordered="false" size="small">
      <n-data-table
        size="small"
        :columns="columns"
        :data="tasks"
        :row-key="(row: Task) => row.id"
        :bordered="false"
        :scroll-x="1500"
        :max-height="640"
        :loading="loading && !tasks.length"
        :locale="{ emptyText: '尚未配置托管任务，点击右上角「新建任务」开始' }"
      />
    </n-card>

    <n-modal v-model:show="showForm" preset="card" :title="form.id ? '编辑任务' : '新建任务'" style="width: 760px">
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
        <n-grid :cols="2" :x-gap="18">
          <n-gi>
            <n-form-item label="任务名称" path="name">
              <n-input v-model:value="form.name" placeholder="例如：后端 API 服务" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="可执行文件 / 脚本路径" path="exe">
              <n-input v-model:value="form.exe" placeholder="C:\app\start.bat 或 C:\app\server.exe" />
            </n-form-item>
          </n-gi>
          <n-gi :span="2">
            <n-form-item label="启动参数（每行一个参数）">
              <n-input v-model:value="form.argsText" type="textarea" :rows="3" placeholder="--port&#10;8080" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="工作目录">
              <n-input v-model:value="form.workDir" placeholder="留空则使用程序所在目录" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="依赖任务">
              <n-select
                v-model:value="form.dependsOn"
                multiple
                :options="dependencyOptions"
                placeholder="按依赖顺序启动"
              />
            </n-form-item>
          </n-gi>
          <n-gi :span="2">
            <n-form-item label="环境变量（每行 KEY=VALUE）">
              <n-input v-model:value="form.envText" type="textarea" :rows="3" placeholder="APP_ENV=production" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="启动延迟（秒）">
              <n-input-number v-model:value="form.delaySeconds" :min="0" :max="3600" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="排序权重（越小越先）">
              <n-input-number v-model:value="form.sortOrder" :min="0" :max="999" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="健康检查端口（可选）">
              <n-input-number v-model:value="form.healthPort" :min="0" :max="65535" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="停止超时（秒）">
              <n-input-number v-model:value="form.stopTimeoutSeconds" :min="1" :max="600" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="异常退出最大重启次数">
              <n-input-number v-model:value="form.maxRestarts" :min="0" :max="20" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="运行策略">
              <n-space vertical>
                <n-switch v-model:value="form.enabled">
                  <template #checked>已启用</template>
                  <template #unchecked>已禁用</template>
                </n-switch>
                <n-switch v-model:value="form.autoStart">
                  <template #checked>开机自启</template>
                  <template #unchecked>不随开机启动</template>
                </n-switch>
                <n-switch v-model:value="form.restartOnExit">
                  <template #checked>异常退出自动重启</template>
                  <template #unchecked>退出后不重启</template>
                </n-switch>
              </n-space>
            </n-form-item>
          </n-gi>
        </n-grid>
      </n-form>
      <template #footer>
        <div class="form-footer">
          <n-button @click="showForm = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">保存</n-button>
        </div>
      </template>
    </n-modal>

    <n-drawer v-model:show="showLogs" :width="620" placement="right" @update:show="(value: boolean) => !value && stopLogPolling()">
      <n-drawer-content :title="`日志 · ${logTask?.name ?? ''}`" closable>
        <div class="log-meta muted">
          {{ logTask?.runtime?.pid ? `PID ${logTask.runtime.pid}` : '当前未运行' }} · 最近 {{ logLines.length }} 行
        </div>
        <div ref="logBox" class="log-box">
          <div v-if="!logLines.length" class="muted">暂无输出</div>
          <div v-for="(line, index) in logLines" :key="index" :class="['log-line', line.stream]">
            <span class="log-time">{{ formatTime(line.time) }}</span>
            <span>{{ line.text }}</span>
          </div>
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.task-sub {
  font-size: 11px;
  color: var(--app-text-muted);
  margin-top: 2px;
}

.form-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.log-meta {
  font-size: 12px;
  margin-bottom: 10px;
}

.log-box {
  height: calc(100vh - 160px);
  overflow: auto;
  padding: 10px 12px;
  border-radius: 8px;
  background: #0c1119;
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.65;
}

.log-line {
  display: flex;
  gap: 10px;
  white-space: pre-wrap;
  word-break: break-all;
  color: #cfd8e6;
}

.log-line.stderr {
  color: #ff9c8f;
}

.log-time {
  color: #5c6a80;
  flex-shrink: 0;
}
</style>
