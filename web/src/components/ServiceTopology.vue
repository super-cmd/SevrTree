<script setup lang="ts">
import { computed } from 'vue'
import type { ProcInfo, ServiceNode, Snapshot } from '@/types'

const props = defineProps<{ snapshot: Snapshot | null }>()

interface Box {
  key: string
  label: string
  sub: string
  x: number
  y: number
  w: number
  h: number
  kind: string
}

const SERVICE_X = 30
const SERVICE_W = 200
const SERVICE_H = 58
const SERVICE_GAP = 16
const CLIENT_X = 640
const CLIENT_W = 300
const CLIENT_H = 44
const CLIENT_GAP = 10
const VIEW_W = 970

const serviceNodes = computed(() => props.snapshot?.services ?? [])
const processMap = computed(() => {
  const map = new Map<number, ProcInfo>()
  for (const proc of props.snapshot?.processes ?? []) {
    map.set(proc.pid, proc)
  }
  return map
})

const visibleServices = computed(() => {
  const list = serviceNodes.value.filter((s) => s.clients > 0 || s.managed)
  if (list.length > 0) {
    return list.slice(0, 12)
  }
  return serviceNodes.value.slice(0, 10)
})

const servicePidSet = computed(() => {
  const set = new Set<number>()
  for (const svc of visibleServices.value) {
    for (const pid of svc.pids) {
      set.add(pid)
    }
  }
  return set
})

const clients = computed(() => {
  const ids = new Set<number>()
  for (const edge of props.snapshot?.edges ?? []) {
    if (edge.kind === 'network' && servicePidSet.value.has(edge.to)) {
      ids.add(edge.from)
    }
  }
  return [...ids]
    .map((pid) => processMap.value.get(pid))
    .filter((p): p is ProcInfo => Boolean(p))
    .sort((a, b) => a.name.localeCompare(b.name))
    .slice(0, 14)
})

const serviceBoxes = computed<Box[]>(() =>
  visibleServices.value.map((svc, index) => ({
    key: `svc-${svc.key}`,
    label: `${svc.name}`,
    sub: `${svc.kind}${svc.ports.length ? ' · ' + svc.ports.join('/') : ''}`,
    x: SERVICE_X,
    y: 24 + index * (SERVICE_H + SERVICE_GAP),
    w: SERVICE_W,
    h: SERVICE_H,
    kind: svc.kind,
  })),
)

const clientBoxes = computed<Box[]>(() =>
  clients.value.map((proc, index) => ({
    key: `cli-${proc.pid}`,
    label: proc.name,
    sub: `PID ${proc.pid}${proc.dependencies.length ? ' · 依赖 ' + proc.dependencies.join('、') : ''}`,
    x: CLIENT_X,
    y: 24 + index * (CLIENT_H + CLIENT_GAP),
    w: CLIENT_W,
    h: CLIENT_H,
    kind: '',
  })),
)

const height = computed(() => {
  const servicesHeight = serviceBoxes.value.length
    ? serviceBoxes.value[serviceBoxes.value.length - 1].y + SERVICE_H + 24
    : 200
  const clientsHeight = clientBoxes.value.length
    ? clientBoxes.value[clientBoxes.value.length - 1].y + CLIENT_H + 24
    : 200
  return Math.max(servicesHeight, clientsHeight, 220)
})

interface Link {
  key: string
  path: string
  color: string
}

const links = computed<Link[]>(() => {
  const serviceByPid = new Map<number, ServiceNode & Box>()
  visibleServices.value.forEach((svc) => {
    const box = serviceBoxes.value.find((b) => b.key === `svc-${svc.key}`)
    if (!box) {
      return
    }
    for (const pid of svc.pids) {
      serviceByPid.set(pid, { ...svc, ...box })
    }
  })

  const clientByPid = new Map<number, Box>()
  clients.value.forEach((proc) => {
    const box = clientBoxes.value.find((b) => b.key === `cli-${proc.pid}`)
    if (box) {
      clientByPid.set(proc.pid, box)
    }
  })

  const result: Link[] = []
  const seen = new Set<string>()
  for (const edge of props.snapshot?.edges ?? []) {
    if (edge.kind !== 'network') {
      continue
    }
    const target = serviceByPid.get(edge.to)
    const source = clientByPid.get(edge.from)
    if (!target || !source) {
      continue
    }
    const key = `${edge.from}-${edge.to}`
    if (seen.has(key)) {
      continue
    }
    seen.add(key)

    const x1 = source.x
    const y1 = source.y + source.h / 2
    const x2 = target.x + target.w
    const y2 = target.y + target.h / 2
    const mid = (x1 + x2) / 2
    result.push({
      key,
      path: `M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`,
      color: kindColor(target.kind),
    })
  }
  return result
})

function kindColor(kind: string): string {
  switch (kind) {
    case '数据库':
      return '#4f9cf9'
    case '缓存':
      return '#e6a23c'
    case 'Web 服务器':
    case '应用服务器':
      return '#2fbf8f'
    case '消息队列':
      return '#e0654f'
    case '监控':
      return '#9a7cf7'
    default:
      return '#7a8799'
  }
}

function clip(text: string, limit: number): string {
  return text.length > limit ? text.slice(0, limit - 1) + '…' : text
}
</script>

<template>
  <div class="topology">
    <div v-if="!snapshot" class="empty">正在采集进程依赖…</div>
    <div v-else-if="serviceBoxes.length === 0" class="empty">
      暂未识别到本机服务依赖。当进程连接本机 nginx / MySQL / Redis 等监听端口时会自动出现在这里。
    </div>
    <svg v-else :viewBox="`0 0 ${VIEW_W} ${height}`" :style="{ height: height + 'px' }" role="img">
      <text :x="SERVICE_X" y="12" class="col-title">被依赖的服务</text>
      <text :x="CLIENT_X" y="12" class="col-title">发起依赖的进程</text>

      <g v-for="link in links" :key="link.key">
        <path :d="link.path" :stroke="link.color" fill="none" stroke-width="1.6" opacity="0.55" />
      </g>

      <g v-for="box in serviceBoxes" :key="box.key">
        <rect
          :x="box.x"
          :y="box.y"
          :width="box.w"
          :height="box.h"
          rx="9"
          :fill="kindColor(box.kind)"
          fill-opacity="0.13"
          :stroke="kindColor(box.kind)"
          stroke-opacity="0.5"
        />
        <circle :cx="box.x + 16" :cy="box.y + box.h / 2" r="4" :fill="kindColor(box.kind)" />
        <text :x="box.x + 28" :y="box.y + 24" class="node-title">{{ clip(box.label, 16) }}</text>
        <text :x="box.x + 28" :y="box.y + 42" class="node-sub">{{ clip(box.sub, 22) }}</text>
      </g>

      <g v-for="box in clientBoxes" :key="box.key">
        <rect
          :x="box.x"
          :y="box.y"
          :width="box.w"
          :height="box.h"
          rx="8"
          fill="#7a8799"
          fill-opacity="0.1"
          stroke="#7a8799"
          stroke-opacity="0.35"
        />
        <text :x="box.x + 14" :y="box.y + 20" class="node-title">{{ clip(box.label, 18) }}</text>
        <text :x="box.x + 14" :y="box.y + 35" class="node-sub">{{ clip(box.sub, 36) }}</text>
      </g>
    </svg>

    <div class="legend">
      <span v-for="item in ['数据库', '缓存', 'Web 服务器', '消息队列', '其他服务']" :key="item">
        <i :style="{ background: kindColor(item) }" />{{ item }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.topology {
  width: 100%;
  overflow: auto;
}

svg {
  width: 100%;
  min-width: 780px;
  display: block;
}

.col-title {
  font-size: 11px;
  fill: var(--app-text-muted);
  letter-spacing: 0.06em;
}

.node-title {
  font-size: 13px;
  font-weight: 600;
  fill: #dbe6f7;
}

html[data-theme='light'] .node-title {
  fill: #1f2937;
}

.node-sub {
  font-size: 11px;
  fill: #8b93a3;
}

.empty {
  padding: 48px 16px;
  text-align: center;
  color: var(--app-text-muted);
  font-size: 13px;
  line-height: 1.8;
}

.legend {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  padding: 8px 4px 2px;
  font-size: 12px;
  color: var(--app-text-muted);
}

.legend i {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
}
</style>
