# SevrTree

Windows 进程依赖分析与开机启动管理器：采集进程启动链、推导服务依赖，并将运行中的进程登记为按顺序编排的自启任务。

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3.5-42B883?logo=vue.js&logoColor=white)](https://vuejs.org/)
[![Vite](https://img.shields.io/badge/Vite-8-646CFF?logo=vite&logoColor=white)](https://vite.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 功能特性

- **进程采集**：WMI `Win32_Process` 与 gopsutil 双通道，读取全量命令行、父进程、运行用户、CPU/内存与监听端口。
- **启动链还原**：识别 explorer、services.exe 与脚本拉起，展示 `cmd.exe → xxx.bat → 目标进程` 的完整链路。
- **依赖推导**：基于 TCP LISTEN、本机 ESTABLISHED 连接与服务特征库（nginx、MySQL、Redis 等），聚合服务依赖拓扑。
- **一键加入自启**：将运行中的进程直接登记为任务，自动带出可执行文件、命令行参数、工作目录与健康检查端口。
- **启动编排**：依赖拓扑排序、排序权重、启动延迟与健康端口探测；启动有序，停止按逆序。
- **任务托管**：环境变量、停止超时、异常退出重启与次数上限，任务输出日志实时查看。
- **访问控制**：仅监听回环地址，口令登录、会话管理、失败锁定与操作审计。

## 基于 NSSM

开机自启由 [NSSM](https://nssm.cc/)（Non-Sucking Service Manager）提供。原版负责服务层工作：安装与移除服务、设置启动类型、启动/停止/重启、stdout 与 stderr 重定向及按大小轮转、进程退出后的重启策略。

SevrTree 在其上补充进程层能力：

- 进程树与启动链还原
- 服务依赖关系推导与拓扑视图
- 多任务依赖编排与顺序控制
- Web 管理界面与任务日志
- 口令认证与操作审计

## 技术栈

| 模块 | 选型 |
| --- | --- |
| 后端 | Go 1.26、`net/http`（方法路由）、`gopsutil/v4`、`wmi` |
| 存储 | SQLite（`modernc.org/sqlite`，纯 Go，WAL 模式） |
| 前端 | Vue 3.5、TypeScript 5.9、Vite 8、Naive UI 2.45、Pinia、Vue Router |
| 包管理 | pnpm 10 |
| 服务托管 | nssm |

## 前置要求

- Windows 10 / 11
- Go 1.26 及以上
- Node.js 22 及以上、pnpm 10 及以上
- [nssm.exe](https://nssm.cc/download)（仅开机自启功能需要）

## 构建

```powershell
# 前端
cd web
pnpm install --store-dir .pnpm-store --cache-dir .pnpm-cache
pnpm build
cd ..

# 后端（前端产物经 go:embed 打入二进制）
go vet ./...
go build -trimpath -ldflags "-s -w" -o dist/sevrtree.exe ./cmd/sevrtree

# 开机自启所需
copy nssm.exe dist\
```

推送 `v*` 标签即可触发 GitHub Actions 自动构建并创建 Release，产物含 `sevrtree.exe`、`nssm.exe`、`README.md`、`LICENSE` 与 `checksums.txt`：

```powershell
git tag v0.1.0
git push origin v0.1.0
```

## 运行

```powershell
dist\sevrtree.exe                 # 建议以管理员身份运行
dist\sevrtree.exe -addr 127.0.0.1:9000 -debug
```

访问 `http://127.0.0.1:8088`。首次启动生成的管理员口令会打印在控制台，并写入 `data/initial-admin-password.txt`（权限 `0600`），登录后请在「设置」中修改。

开发模式下前后端分离运行：

```powershell
go run ./cmd/sevrtree -debug      # 终端 1
cd web && pnpm dev                # 终端 2，/api 已代理至 8088
```

## 公网访问

默认仅监听 `127.0.0.1`。程序内置的是纯 HTTP 服务，不含 TLS、IP 白名单等公网防护能力，因此公网访问应通过反向代理或隧道终止 TLS，不建议直接暴露端口。

### 反向代理（推荐）

1. 让服务监听所有网卡，并限制只有代理服务器可访问该端口（Windows 防火墙入站规则仅放行代理地址）：

```powershell
setx SEVRTREE_ADDR 0.0.0.0:8088
# 或启动时指定
dist\sevrtree.exe -addr 0.0.0.0:8088
```

2. Caddy（自动签发与续期证书）：

```caddyfile
sevrtree.example.com {
    reverse_proxy 127.0.0.1:8088
}
```

3. Nginx：

```nginx
server {
    listen 443 ssl;
    server_name sevrtree.example.com;

    ssl_certificate     /etc/ssl/sevrtree.crt;
    ssl_certificate_key /etc/ssl/sevrtree.key;

    location / {
        proxy_pass         http://127.0.0.1:8088;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

4. 只对外开放 `443`；`80` 仅用于证书校验或 301 跳转。

### 隧道（不开放入站端口）

```powershell
cloudflared tunnel --url http://127.0.0.1:8088
```

也可用 frp 将本机 `127.0.0.1:8088` 映射到公网服务器。此时无需修改监听地址。

### 注意事项

- 必须使用强口令，首次生成的随机口令登录后立即修改。
- 应用需部署在域名根路径，不支持 `/sevrtree/` 之类的子路径前缀。
- 反向代理后，审计日志与登录限流记录的来源 IP 是代理地址（当前按 `RemoteAddr` 统计），来源限制建议在代理层用 allow/deny 完成。
- 直接监听 `0.0.0.0` 且不启用 TLS 时，口令与会话将明文传输，禁止在不可信网络使用。

## 配置

| 参数 / 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8088` | HTTP 监听地址，优先于环境变量 |
| `-debug` | `false` | 输出调试日志 |
| `-version` | `false` | 打印版本后退出 |
| `SEVRTREE_ADDR` | `127.0.0.1:8088` | HTTP 监听地址 |
| `SEVRTREE_DATA_DIR` | `<exe 目录>/data` | 数据目录（SQLite、日志、任务输出） |
| `SEVRTREE_LOG_DIR` | `<DATA_DIR>/logs` | 服务日志目录 |
| `SEVRTREE_SESSION_MINUTES` | `720` | 会话有效期（分钟） |
| `SEVRTREE_ADMIN_PASSWORD` | 空 | 仅首次初始化生效，未设置则生成 14 位随机口令 |
| `SEVRTREE_AUTOBOOT` | 非 `0` | 设为 `0` 时启动不执行任务编排 |

## 任务模型

| 字段 | 说明 |
| --- | --- |
| `exe` / `args` / `workDir` / `env` | 可执行文件、启动参数、工作目录、附加环境变量 |
| `delaySeconds` / `sortOrder` | 启动前等待秒数、同层级排序权重 |
| `dependsOn` | 依赖的任务 ID，参与拓扑排序，成环时报错 |
| `healthPort` | 健康检查端口，`0` 表示不检查（最长等待 30s） |
| `autoStart` / `enabled` | 随 SevrTree 启动、参与批量启停 |
| `restartOnExit` / `maxRestarts` | 异常退出重启及次数上限 |

脚本自动适配：`.bat` / `.cmd` 经 `cmd.exe /c`，`.ps1` 经 PowerShell，`.sh` 经 bash，其余直接执行。

## 项目结构

```text
cmd/sevrtree/        程序入口与依赖装配
internal/
  api/               HTTP 路由与 Handler
  auth/              口令、会话与失败锁定
  config/            配置加载与目录初始化
  logging/           slog 控制台 / 文件双通道
  manager/           任务生命周期、拓扑编排、日志缓冲
  platform/          Windows 提权、进程树终止、nssm 服务
  procmon/           进程采集、依赖推导、服务特征库、WMI
  store/             SQLite（tasks / settings / audit_logs）
web/                 Vite + Vue 3 前端工程，产物嵌入二进制
```

## 安全

- 默认仅监听 `127.0.0.1`，不对外暴露。
- 口令以 bcrypt（cost 12）散列存储；5 分钟内失败 5 次锁定 5 分钟。
- 安装自启服务、读取高权限进程命令行需管理员权限。

## 贡献

提交前请确保 `go vet ./...` 与 `pnpm build` 均通过；提交信息遵循 Conventional Commits（如 `feat: 支持进程一键加入自启`）。新增平台相关代码请放入 `_windows.go` / `_other.go` 并加构建标签。

## 许可证

[MIT](LICENSE)
