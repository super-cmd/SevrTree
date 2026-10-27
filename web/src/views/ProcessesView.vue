<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NInput,
  NSpace,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
  NTimeline,
  NTimelineItem,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { api, ApiError } from '@/api/client'
import type { ProcInfo, Snapshot } from '@/types'
import { formatBytes, formatTime } from '@/utils'
import ServiceTopology from '@/components/ServiceTopology.vue'

interface ProcRow extends ProcInfo {
  key: number
  children?: ProcRow[]
}

const message = useMessage()

const snapshot = ref<Snapshot | null>(null)
const loading = ref(true)
const lastError = ref('')
const keyword = ref('')
const managedOnly = ref(false)
const tab = ref<'tree' | 'topology'>('tree')
const expandedRowKeys = ref<number[]>([])
const selected = ref<ProcRow | null>(null)
const showDetail = ref(false)
const importingPid = ref<number | null>(null)

let timer: number | undefined
let inFlight = false

const filteredProcesses = computed<ProcInfo[]>(() => {
  let list = snapshot.value?.processes ?? []
  if (managedOnly.value) {
    list = list.filter((p) => p.managed)
  }
  const kw = keyword.value.trim().toLowerCase()
  if (kw) {
    list = list.filter((p) =>
      [p.name, p.cmdline, p.exe, p.launcher, p.user, p.service, ...p.dependencies]
        .join(' ')
        .toLowerCase()
        .includes(kw),
    )
  }
  return list
})

function buildTree(list: ProcInfo[]): ProcRow[] {
  const map = new Map<number, ProcRow>()
  for (const proc of list) {
    map.set(proc.pid, { ...proc, key: proc.pid })
  }
  const roots: ProcRow[] = []
  for (const row of map.values()) {
    const parent = map.get(row.ppid)
    if (parent && parent.key !== row.key) {
      parent.children = parent.children ?? []
      parent.children.push(row)
    } else {
      roots.push(row)
    }
  }
  const sortRows = (rows: ProcRow[]) => {
    rows.sort((a, b) => a.pid - b.pid)
    rows.forEach((row) => row.children && sortRows(row.children))
  }
  sortRows(roots)
  return roots
}

const tableData = computed<ProcRow[]>(() => {
  if (keyword.value.trim()) {
    return filteredProcesses.value.map((p) => ({ ...p, key: p.pid })).sort((a, b) => a.pid - b.pid)
  }
  return buildTree(filteredProcesses.value)
})

const managedCount = computed(() => (snapshot.value?.processes ?? []).filter((p) => p.managed).length)

function expandAll() {
  const keys: number[] = []
  const walk = (rows: ProcRow[]) => {
    rows.forEach((row) => {
      if (row.children?.length) {
        keys.push(row.key)
        walk(row.children)
      }
    })
  }
  walk(tableData.value)
  expandedRowKeys.value = keys
}

function collapseAll() {
  expandedRowKeys.value = []
}

async function load() {
  if (inFlight) {
    return
  }
  inFlight = true
  try {
    snapshot.value = await api.get<Snapshot>('/api/processes')
    lastError.value = ''
  } catch (err) {
    lastError.value = err instanceof ApiError ? err.message : '采集进程失败'
  } finally {
    loading.value = false
    inFlight = false
  }
}

function openDetail(row: ProcRow) {
  selected.value = row
  showDetail.value = true
}

async function importProcess(row: ProcRow) {
  importingPid.value = row.pid
  try {
    const res = await api.post<{ id: number; name: string; port: number }>(`/api/processes/${row.pid}/task`)
    const portTip = res.port ? `，健康检查端口 ${res.port}` : ''
    message.success(`已将「${res.name}」加入开机自启${portTip}；可在「托管任务」中调整顺序与延迟`)
  } catch (err) {
    message.error(err instanceof ApiError ? err.message : '加入自启动失败')
  } finally {
    importingPid.value = null
  }
}

const columns = computed<DataTableColumns<ProcRow>>(() => [
  {
    title: '进程',
    key: 'name',
    minWidth: 220,
    render: (row) =>
      h('div', { class: 'proc-name' }, [
        h('span', { style: 'font-weight:600' }, row.name || `PID ${row.pid}`),
        row.managed
          ? h(NTag, { size: 'tiny', type: 'success', bordered: false, style: 'margin-left:8px' }, { default: () => '托管' })
          : null,
      ]),
  },
  {
    title: 'PID / PPID',
    key: 'pid',
    width: 120,
    render: (row) => h('span', { class: 'mono' }, `${row.pid} / ${row.ppid}`),
  },
  {
    title: '启动者',
    key: 'launcher',
    minWidth: 200,
    ellipsis: { tooltip: true },
    render: (row) => row.launcher || '—',
  },
  {
    title: '命令行',
    key: 'cmdline',
    minWidth: 300,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'mono' }, row.cmdline || row.exe || '—'),
  },
  {
    title: '依赖服务',
    key: 'dependencies',
    minWidth: 170,
    render: (row) =>
      row.dependencies.length
        ? h(
            'div',
            { style: 'display:flex;gap:4px;flex-wrap:wrap' },
            row.dependencies.map((dep) =>
              h(NTag, { size: 'tiny', bordered: false, type: 'info' }, { default: () => dep }),
            ),
          )
        : '—',
  },
  {
    title: '监听端口',
    key: 'ports',
    width: 130,
    render: (row) => (row.ports.length ? row.ports.join(', ') : '—'),
  },
  {
    title: 'CPU',
    key: 'cpu',
    width: 80,
    render: (row) => `${row.cpu.toFixed(1)}%`,
  },
  {
    title: '内存',
    key: 'memRss',
    width: 100,
    render: (row) => formatBytes(row.memRss),
  },
  {
    title: '用户',
    key: 'user',
    width: 150,
    ellipsis: { tooltip: true },
    render: (row) => row.user || '—',
  },
  {
    title: '操作',
    key: 'actions',
    width: 190,
    render: (row) =>
      h(NSpace, { size: 4 }, {
        default: () => [
          h(
            NButton,
            { size: 'tiny', quaternary: true, type: 'primary', onClick: () => openDetail(row) },
            { default: () => '详情' },
          ),
          h(
            NButton,
            {
              size: 'tiny',
              type: 'success',
              ghost: true,
              disabled: row.managed,
              loading: importingPid.value === row.pid,
              onClick: () => importProcess(row),
            },
            { default: () => (row.managed ? '已托管' : '加入自启') },
          ),
        ],
      }),
  },
])

onMounted(async () => {
  await load()
  expandAll()
  timer = window.setInterval(load, 3000)
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
        <h2 class="page-title">进程与依赖</h2>
        <p class="page-subtitle">
          共 {{ snapshot?.total ?? 0 }} 个进程，托管 {{ managedCount }} 个；采集于 {{ formatTime(snapshot?.collectedAt) }}
        </p>
      </div>
      <n-space>
        <n-input
          v-model:value="keyword"
          placeholder="搜索进程名 / 命令行 / 启动者 / 依赖"
          clearable
          style="width: 300px"
        />
        <n-switch v-model:value="managedOnly">
          <template #checked>仅托管</template>
          <template #unchecked>全部</template>
        </n-switch>
        <n-button :loading="loading" @click="load">刷新</n-button>
      </n-space>
    </div>

    <n-tabs v-model:value="tab" type="line" animated>
      <n-tab-pane name="tree" tab="进程树">
        <n-card :bordered="false" size="small">
          <template #header-extra>
            <n-space>
              <n-button size="small" quaternary @click="expandAll">展开全部</n-button>
              <n-button size="small" quaternary @click="collapseAll">折叠全部</n-button>
            </n-space>
          </template>
          <n-data-table
            v-model:expanded-row-keys="expandedRowKeys"
            size="small"
            :columns="columns"
            :data="tableData"
            :row-key="(row: ProcRow) => row.key"
            :bordered="false"
            :scroll-x="1880"
            :max-height="640"
            :loading="loading && !snapshot"
            :locale="{ emptyText: '没有匹配的进程' }"
          />
        </n-card>
      </n-tab-pane>

      <n-tab-pane name="topology" tab="依赖拓扑">
        <n-card :bordered="false" size="small">
          <service-topology :snapshot="snapshot" />
        </n-card>
      </n-tab-pane>
    </n-tabs>

    <n-drawer v-model:show="showDetail" :width="560" placement="right">
      <n-drawer-content
        v-if="selected"
        :title="`${selected.name} · PID ${selected.pid}`"
        closable
      >
        <n-descriptions :column="2" size="small" bordered label-placement="left">
          <n-descriptions-item label="PID">{{ selected.pid }}</n-descriptions-item>
          <n-descriptions-item label="父进程 PID">{{ selected.ppid }}</n-descriptions-item>
          <n-descriptions-item label="运行用户">{{ selected.user || '—' }}</n-descriptions-item>
          <n-descriptions-item label="状态">{{ selected.status || '—' }}</n-descriptions-item>
          <n-descriptions-item label="启动时间">{{ formatTime(selected.createdAt ? new Date(selected.createdAt).toISOString() : undefined) }}</n-descriptions-item>
          <n-descriptions-item label="内存">{{ formatBytes(selected.memRss) }}</n-descriptions-item>
          <n-descriptions-item label="启动者" :span="2">{{ selected.launcher || '—' }}</n-descriptions-item>
          <n-descriptions-item label="可执行文件" :span="2">
            <span class="mono">{{ selected.exe || '—' }}</span>
          </n-descriptions-item>
          <n-descriptions-item label="监听端口" :span="2">
            {{ selected.ports.length ? selected.ports.join(', ') : '—' }}
          </n-descriptions-item>
          <n-descriptions-item label="依赖服务" :span="2">
            <n-space v-if="selected.dependencies.length" :size="4">
              <n-tag v-for="dep in selected.dependencies" :key="dep" size="small" type="info" :bordered="false">
                {{ dep }}
              </n-tag>
            </n-space>
            <span v-else>—</span>
          </n-descriptions-item>
        </n-descriptions>

        <div class="detail-block">
          <div class="detail-title">完整命令行</div>
          <pre class="cmd">{{ selected.cmdline || '（无命令行信息）' }}</pre>
        </div>

        <div class="detail-block">
          <div class="detail-title">启动链（由谁启动）</div>
          <n-timeline v-if="selected.chain.length">
            <n-timeline-item
              v-for="item in selected.chain"
              :key="item.pid"
              :title="`${item.name} (PID ${item.pid})`"
              :content="item.cmdline || '—'"
            />
          </n-timeline>
          <p v-else class="muted">无父进程或父进程已退出</p>
        </div>

        <div class="detail-actions">
          <n-button
            type="primary"
            :disabled="selected.managed"
            :loading="importingPid === selected.pid"
            @click="importProcess(selected)"
          >
            {{ selected.managed ? '已在托管任务中' : '加入开机自启' }}
          </n-button>
          <span class="muted detail-hint">
            将按当前命令行参数创建任务，默认开机自启；随后可在「托管任务」调整延迟、顺序与依赖。
          </span>
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.proc-name {
  display: flex;
  align-items: center;
}

.detail-block {
  margin-top: 22px;
}

.detail-title {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}

.cmd {
  margin: 0;
  padding: 12px;
  border-radius: 8px;
  background: rgba(127, 140, 160, 0.12);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.6;
}

.detail-actions {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid var(--app-border);
  display: flex;
  align-items: center;
  gap: 12px;
}

.detail-hint {
  font-size: 12px;
  line-height: 1.6;
}
</style>
