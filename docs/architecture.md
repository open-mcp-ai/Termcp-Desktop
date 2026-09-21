# Termcp-Desktop 架构

## 分层

```mermaid
flowchart TB
    UI[资源管理器 UI\n连接 / 会话 / Shell / 历史] -->|Wails binding| APP[App 层\n窗口控制 / Core 生命周期 / REST 客户端]
    APP -->|未注册服务| CORE[进程内 termcp Core]
    APP -->|注册 / 启停 / 自启| OS[操作系统服务管理器]
    OS -->|--core-service| DAEMON[本机 termcp Core 服务]
    APP -->|HTTP 127.0.0.1:18765| API[REST / UI WebSocket]
    CORE --> API
    DAEMON --> API
    CORE --> SSH[SSH / PTY / SFTP / 转发]
    CORE --> DATA[~/.termcp\n配置 / 会话 / 历史]
    DAEMON --> SSH
    DAEMON --> DATA
    TRAY[系统托盘\nCore / 服务状态与快捷操作] --> APP
    APP --> LOG[~/.termcp/logs\n结构化日志 / 14 天保留]
    API --> LOG
```

界面不重新实现 SSH。它只编排 termcp 的连接配置和会话资源。这样 MCP、WebUI 和 GUI 看到同一批会话、Shell、历史与转发。

## Core 生命周期

应用只管理固定在 `127.0.0.1:18765` 的本机 Core：

1. 未注册系统服务时，在 Wails 进程内构造 termcp 的 SSH server、storage、message、history、session、sshconfig、forward、MCP 和 WebUI handler。
2. 注册服务时，先停止进程内 Core，再写入平台服务定义，以同一可执行文件的 `--core-service` 模式启动并等待本机 API 就绪。
3. 已注册服务时，GUI 只附着本机 API；关闭桌面窗口不停止服务进程。
4. 卸载服务时，先停止服务、移除定义，再立即恢复进程内 Core，保证桌面端继续可用。
5. Core 就绪后，资源管理器从 `/api/connections`、`/api/sessions`、`/api/history`、`/api/forwards` 生成快照。

所有 Wails 绑定、Core 生命周期、系统服务操作、GUI 到 Core 的出站请求，以及 Core 的入站 HTTP 接口共享结构化日志字段。日志不记录请求正文、凭据和查询参数值；单文件 10 MiB 或跨日轮转，保留 14 天。

## 工作台渲染生命周期

应用外壳可以按状态重新渲染，但 xterm 实例不随页面 DOM 销毁。渲染前终端 DOM 被移入内存片段，渲染后挂载到新的活动窗格；不活动会话继续保留滚动缓冲和 WebSocket watch。只有对应 Shell 从 Core 快照中消失时才释放实例。文件面板按会话、标签和路径缓存，并丢弃快速切换产生的过期异步响应。

平台服务后端分别为 macOS LaunchAgent、Linux systemd 用户服务和 Windows Service Control Manager。Windows 的服务入口使用 SCM 控制分派器处理 Stop 与 Shutdown，以专用的 `NT SERVICE\\termcp-desktop-core` 虚拟账户运行，只授予它访问当前 Core 数据目录的权限，并在需要修改 SCM 时请求 UAC；Linux/macOS 使用 SIGTERM 完成有序关闭。

## 桌面与托盘生命周期

Termcp 使用 Wails 的单实例锁。关闭主窗口只隐藏窗口，Core 与托盘继续运行；托盘“退出 Termcp”才结束桌面进程。再次启动程序时，已有实例恢复并聚焦主窗口。

macOS 托盘直接复用 Wails 已有的 AppKit 事件循环，通过 `NSStatusItem` 提供原生状态菜单。Windows 使用 `Shell_NotifyIconW`，Linux 使用 D-Bus StatusNotifierItem，因此 Linux 在 KDE、支持 AppIndicator 的 GNOME、XFCE 等桌面可用；没有托盘宿主的 Linux 环境仍可正常使用主窗口。

Termcp-Desktop 启动时把 Core 数据目录显式设置为当前用户 Home 下的 `~/.termcp`（Windows 为 `%USERPROFILE%\.termcp`）。应用内 Core 与操作系统服务使用同一路径，升级或切换运行模式不会迁移数据。前端语言和工作区布局保存在 WebView 本地存储，不混入 Core 数据目录。

## 已完成能力

- Core 进程内运行与系统服务运行之间切换。
- macOS、Linux、Windows 服务注册、卸载、启停、重启与自启设置。
- 连接配置读取、原始 TOML 编辑、测试、重命名和删除。
- 会话、Shell、输出分页、终端 WebSocket 输入输出与 PTY resize。
- 多工作区、四窗格、左右/上下/平铺、比例调整与布局持久化。
- 文件浏览、上传、下载、重命名、删除和创建目录。
- Local / Remote / Dynamic 转发与通知规则管理。
- 历史搜索、元数据编辑、正文和截图导出。
- Wails 原生窗口控制和单应用打包。
- 系统托盘状态、Core 快捷控制、主窗口隐藏/恢复与单实例唤醒。
- 浏览器脱敏预览及错误状态。

## 仍需平台级补充

- 系统监控没有对应的 termcp Core 接口，暂不显示模拟 CPU、内存或磁盘数据。
- 自动升级与三平台安装包仍属于桌面发布层。
- 终端仍需在目标平台持续压测中文输入法、宽字符、alternate screen 和大吞吐输出。

远程会话的公开 `ssh_endpoint` 当前只返回 `remote`，无法可靠映射回具体连接配置。资源树因此把运行中会话单列，避免在多个连接节点下重复或错误归属；Core API 增加非敏感的连接名称后再建立层级关联。
