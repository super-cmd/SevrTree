import type { TaskStatus } from './types'

export function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(value >= 100 || index === 0 ? 0 : 1)} ${units[index]}`
}

export function formatDuration(totalSeconds: number): string {
  if (!totalSeconds || totalSeconds < 0) {
    return '—'
  }
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  if (days > 0) {
    return `${days} 天 ${hours} 小时`
  }
  if (hours > 0) {
    return `${hours} 小时 ${minutes} 分`
  }
  return `${minutes} 分钟`
}

export function formatTime(value?: string): string {
  if (!value) {
    return '—'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '—'
  }
  return date.toLocaleString('zh-CN', { hour12: false })
}

export function formatClock(value?: string): string {
  if (!value) {
    return '—'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '—'
  }
  return date.toLocaleTimeString('zh-CN', { hour12: false })
}

export function statusMeta(status?: TaskStatus): { text: string; type: 'default' | 'info' | 'success' | 'warning' | 'error' } {
  switch (status) {
    case 'running':
      return { text: '运行中', type: 'success' }
    case 'starting':
      return { text: '启动中', type: 'info' }
    case 'stopping':
      return { text: '停止中', type: 'warning' }
    case 'failed':
      return { text: '异常', type: 'error' }
    default:
      return { text: '未运行', type: 'default' }
  }
}

export function serviceKindType(kind: string): 'default' | 'info' | 'success' | 'warning' | 'error' {
  switch (kind) {
    case '数据库':
      return 'info'
    case '缓存':
      return 'warning'
    case 'Web 服务器':
    case '应用服务器':
      return 'success'
    case '消息队列':
      return 'error'
    default:
      return 'default'
  }
}
