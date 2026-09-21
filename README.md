# Termcp-Desktop

**Termcp** 是 termcp 的 Go 桌面管理端与 SSH 工作台，项目与仓库名称为 **Termcp-Desktop**。程序以资源管理器为主界面，管理 Core、SSH 连接、会话、Shell、历史记录与端口转发，并逐步接替 termcp WebUI。

桌面壳使用 [Wails v2](https://wails.io/docs/introduction/)，前端由系统 WebView 渲染，不打包 Electron 或 Chromium。termcp Core 通过 Go 模块直接集成到同一个应用；GUI 与 Core 之间继续使用稳定的 REST / WebSocket 边界。

## 当前可用

- Core 只监听本机 `127.0.0.1:18765`。未注册系统服务时在应用进程内运行；注册后由操作系统独立托管。
- Core 的持久化目录固定为当前用户 Home 下的 `~/.termcp`（Windows 为 `%USERPROFILE%\.termcp`）；应用内 Core 与系统服务共用这一个目录。
- 独立的“系统服务”页面提供注册、卸载、启动、停止、重启和开机自启控制。关闭 GUI 不会停止已注册的 Core。
- 同一可执行文件通过 `--core-service` 进入无窗口服务模式，macOS、Linux 和 Windows 不需要另一套 Core 程序。
- 从真实 Core 读取连接配置、活跃会话、Shell、历史记录和端口转发。
- 创建、编辑、测试和删除 SSH 配置；创建会话与 Shell，重命名、结束、归档或永久删除。
- xterm 终端通过 `/api/ui/ws` 收发真实 PTY 字节流，同步 resize，并从 output-range 恢复滚动历史。
- 多工作区标签、最多四窗格、左右/上下/平铺、比例调整、窗格最大化和布局持久化。
- 右侧会话工具完整接入本地/SFTP 文件、上传下载、重命名、目录、Local/Remote/Dynamic 转发和通知规则。
- 历史搜索、标签与备注编辑、正文查看、Markdown 导出、PNG 截图和删除。
- 资源管理器式导航、搜索、Core 状态、MCP 配置和应用设置。
- 界面和托盘支持简体中文与英文，并可在设置中即时切换；语言偏好保存在 WebView 本地存储，不写入 Core 数据目录。
- macOS、Windows 系统托盘和 Linux StatusNotifierItem 托盘；菜单显示 Core、服务和自启状态，并提供工作台、服务管理、Core 控制、关于与退出入口。
- 关闭主窗口会隐藏到托盘；再次启动会唤醒已有实例，避免重复启动 Core。
- 无 Wails bridge 的浏览器预览使用脱敏演示数据，便于单独审阅界面。

Core WebUI 的规范接口均已接入，覆盖表见 [docs/api-coverage.md](docs/api-coverage.md)。

## 开发

需要 Go 1.25.9 或更高版本、Node.js 和 npm。`go.mod` 固定到 termcp `dev` 分支的公开提交，不依赖本机相邻目录。

```sh
# Wails 原生开发窗口
npm run dev:wails

# 仅预览前端，地址为 http://127.0.0.1:4173
npm run dev

# 静态检查、前端生产构建和 Go 测试
npm run check

# 生成原生应用
npm run build
```

macOS 构建结果位于 `build/bin/Termcp-Desktop.app`，应用显示名和主可执行文件名均为 `Termcp`。

`npm run build` 在当前操作系统生成原生包：macOS 为 `.app`，Windows 为 `.exe`，Linux 为原生可执行文件。发布构建应分别在三个目标系统上执行，以使用各自的 WebView 和打包工具链。

## 系统服务

| 平台 | 管理方式 | 服务定义 | 自启范围 |
| --- | --- | --- | --- |
| macOS | LaunchAgent | `~/Library/LaunchAgents/ai.openmcp.termcp.desktop.core.plist` | 当前用户登录 |
| Linux | systemd 用户服务 | `~/.config/systemd/user/ai.openmcp.termcp.desktop.core.service` | 当前用户登录；启用 linger 时可在未登录时运行 |
| Windows | Service Control Manager | `termcp-desktop-core` | 系统启动；需要变更服务时自动请求 UAC 授权 |

服务始终启动本机 Core。SSH 目标主机作为连接资源统一由这个 Core 管理。

## 结构

```text
frontend/          资源管理器界面与浏览器预览
internal/config/   产品信息、数据目录与系统服务注册（macOS / Linux / Windows）
internal/core/     进程内 Core 生命周期与服务运行入口
internal/bridge/   受限 API 桥与原生上传/下载
internal/fonts/    系统字体发现
internal/tray/     系统托盘与状态菜单（macOS / Windows / Linux）
internal/model/    前后端共享的数据结构
app.go             Wails 绑定与 Core 生命周期编排
main.go            原生窗口、单实例、服务模式和资源打包入口
prototype/         前期多方案交互原型归档
docs/              产品、SSH 工作区与架构说明
```

整体架构和边界见 [docs/architecture.md](docs/architecture.md)，此前的多方案原型仍保存在 [prototype](prototype)。

## 许可证

Termcp-Desktop 使用 [Apache License 2.0](LICENSE)。第三方组件保留各自的许可证与版权声明，见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
