# termcp-gui

termcp-gui 是 termcp 的 Go 桌面管理端与 SSH 工作台。程序以资源管理器为主界面，管理 Core、SSH 连接、会话、Shell、历史记录与端口转发，并逐步接替 termcp WebUI。

桌面壳使用 [Wails v2](https://wails.io/docs/introduction/)，前端由系统 WebView 渲染，不打包 Electron 或 Chromium。termcp Core 通过 Go 模块直接集成到同一个应用；GUI 与 Core 之间继续使用稳定的 REST / WebSocket 边界。

## 当前可用

- 启动时探测 `127.0.0.1:18765`。已有 Core 正在运行时安全接入，否则在应用进程内启动 Core。
- 关闭应用时只停止由 GUI 启动的 Core，不影响外部实例。
- 从真实 Core 读取连接配置、活跃会话、Shell、历史记录和端口转发。
- 创建 SSH / 本机会话、在现有会话中新建 Shell、结束并归档会话。
- 资源管理器式导航、搜索、Core 状态页、连接详情、会话详情和应用设置。
- 无 Wails bridge 的浏览器预览使用脱敏演示数据，便于单独审阅界面。

终端页面目前展示真实 Shell 元数据，字节流输入输出还没有接入。下一步将通过 Core 的 UI WebSocket 完成终端渲染、窗口 resize、分屏和平铺。

## 开发

需要 Go 1.25.9 或更高版本、Node.js 和 npm。当前项目通过本地 `replace` 使用相邻的 `../termcp` 源码。

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

macOS 构建结果位于 `build/bin/termcp-gui.app`。

## 结构

```text
frontend/          资源管理器界面与浏览器预览
internal/core/     termcp Core 的进程内生命周期封装
app.go             Wails 绑定与 Core REST 客户端
main.go            原生窗口和资源打包入口
prototype/         前期多方案交互原型归档
docs/              产品、SSH 工作区与架构说明
```

整体架构和边界见 [docs/architecture.md](docs/architecture.md)，此前的多方案原型仍保存在 [prototype](prototype)。

