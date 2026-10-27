export interface ChainItem {
  pid: number
  name: string
  cmdline: string
}

export interface ProcInfo {
  pid: number
  ppid: number
  name: string
  exe: string
  cmdline: string
  user: string
  createdAt: number
  cpu: number
  memRss: number
  status: string
  ports: number[]
  service: string
  serviceKind: string
  dependencies: string[]
  chain: ChainItem[]
  launcher: string
  managed: boolean
  taskId?: number
}

export interface ProcEdge {
  from: number
  to: number
  kind: 'parent' | 'network'
}

export interface ServiceNode {
  key: string
  name: string
  kind: string
  pids: number[]
  ports: number[]
  clients: number
  managed: boolean
}

export interface Snapshot {
  collectedAt: string
  processes: ProcInfo[]
  edges: ProcEdge[]
  services: ServiceNode[]
  total: number
}

export type TaskStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'failed'

export interface RuntimeState {
  taskId: number
  name: string
  status: TaskStatus
  pid: number
  startedAt: string
  restarts: number
  exitCode?: number
  lastError?: string
  logPath?: string
}

export interface Task {
  id: number
  name: string
  exe: string
  args: string[]
  workDir: string
  env: Record<string, string>
  delaySeconds: number
  sortOrder: number
  enabled: boolean
  autoStart: boolean
  dependsOn: number[]
  healthPort: number
  stopTimeoutSeconds: number
  restartOnExit: boolean
  maxRestarts: number
  createdAt: string
  updatedAt: string
  runtime?: RuntimeState
}

export interface SequenceState {
  running: boolean
  mode: string
  current: string
  message: string
  startedAt: string
  started: number
  failed: number
}

export interface SystemInfo {
  hostname: string
  os: string
  platform: string
  platformVersion: string
  arch: string
  uptimeSeconds: number
  cpuCount: number
  memTotal: number
  memUsed: number
  memPercent: number
  elevated: boolean
}

export interface AppInfo {
  version: string
  addr: string
  dataDir: string
  logDir: string
  startedAt: string
  uptimeSeconds: number
}

export interface AutostartInfo {
  serviceName: string
  installed: boolean
  state: string
}

export interface SystemPayload {
  system: SystemInfo
  app: AppInfo
  autostart: AutostartInfo
  sequence: SequenceState
}

export interface LogLine {
  time: string
  stream: 'stdout' | 'stderr'
  text: string
}

export interface AuditLog {
  id: number
  createdAt: string
  username: string
  action: string
  target: string
  ip: string
  detail: string
}

export interface TaskListPayload {
  tasks: Task[]
  sequence: SequenceState
}
