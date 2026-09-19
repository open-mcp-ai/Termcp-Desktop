# 桌面技术选型

## 结论

当前实现采用 **Go + Wails v2 + 系统 WebView**。这是 Clash Verge Rev 分层方式在 termcp 上的对应实现：桌面层负责窗口、生命周期和系统集成，Core 层负责 SSH、PTY、历史、转发与 MCP，界面通过 API 使用 Core 能力。

Wails 不打包 Electron 或 Chromium。它把前端资源和 Go 代码编译进原生程序，并提供 Go 与前端之间的绑定。选择 v2 是因为它是稳定版本；v3 仍处于 beta，不作为当前交付基线。

## 为什么选择 Wails

| 目标 | 设计决定 |
| --- | --- |
| 与 termcp 集成 | GUI 模块位于 `github.com/open-mcp-ai/termcp/gui`，可以直接复用 termcp 的 Go 内部组件 |
| 单一程序 | Core 在 Wails 进程内启动，前端资源随应用打包 |
| 本机独立 Core | 注册系统服务后附着 `127.0.0.1:18765`，GUI 退出不影响服务生命周期 |
| 替换 WebUI | 保留 REST / WebSocket 协议，桌面 UI 可以按纵向功能逐步迁移 |
| 避开 Electron | 使用 macOS WebKit、Windows WebView2 和 Linux WebKitGTK 等系统渲染引擎 |

## 与纯原生 UI 的取舍

Rust + GPUI、Go + Fyne 都适合自绘终端，但会增加现有管理界面的迁移成本。Wails 能直接延续已经验证的资源树、面板和分屏交互，同时让 Go 负责可信的 Core 生命周期与系统能力。终端性能必须通过真实 PTY 场景验证；若系统 WebView 的字符网格无法满足持续大输出和多窗格刷新，再把终端画布单独替换为原生组件，而不是重写整个管理端。

## 一手资料

- [Wails 官方介绍](https://wails.io/docs/introduction/)
- [Wails v2.15.0 发布页](https://github.com/wailsapp/wails/releases/tag/v2.15.0)
- [Clash Verge Rev 文档](https://clash-vergeapp.com/en/docs/)
- [Clash Verge Rev 源码](https://github.com/clash-verge-rev/clash-verge-rev)
