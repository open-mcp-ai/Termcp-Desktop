# Core 接口覆盖

Termcp 通过受限 Wails API 桥访问当前 Core 的 `/api/` 路径。终端使用同一 Core 的 WebSocket；上传和下载由 Go 流式处理并调用系统文件选择器。

| 能力 | Core 接口 | GUI 入口 | 自动验证 |
| --- | --- | --- | --- |
| 连接列表 | `GET /api/connections` | 资源树 | 快照与集成测试 |
| 连接详情 | `GET /api/connections/{name}` | 编辑连接 | API 测试 |
| 保存/重命名连接 | `PUT /api/connections/{name}` | SSH 配置编辑器 | 集成测试 |
| 删除连接 | `DELETE /api/connections/{name}` | 连接详情 | 集成测试 |
| 测试连接 | `POST /api/connections/test` | 编辑器与连接详情 | 集成测试 |
| 会话列表/创建 | `GET/POST /api/sessions` | 资源树、新建会话 | 集成测试 |
| Core 版本 | `GET /api/version` | 设置页当前运行实例版本 | 集成测试 |
| 会话详情/重命名/删除 | `GET/PATCH/DELETE /api/sessions/{id}` | 会话详情 | 集成测试 |
| 结束并归档 | `POST /api/sessions/{id}/terminate` | 会话详情 | 集成测试 |
| Shell 列表/创建 | `GET/POST /api/sessions/{id}/shells` | 会话详情、终端窗格 | 集成测试 |
| 关闭 Shell | `DELETE /api/shells/{id}` | Shell 详情 | Core handler 测试与 UI 验证 |
| 输出分页 | `GET /api/shells/{id}/output-range` | 终端启动恢复 | 集成测试 |
| 终端流 | `GET /api/ui/ws` | xterm 工作区 | 浏览器协议适配与原生构建 |
| 输入活动 | WebSocket `shell_activity`（`shell_id`、`src`、`submit`） | Shell 标签中的人类／AI 输入提示 | Core WebSocket 集成测试、前端状态测试 |
| 已结束会话列表 | `GET /api/sessions`、`GET /api/sessions/{id}/shells` | 历史页 | 快照与集成测试 |
| 对话索引 | `GET /api/shells/{id}/marks`（与 MCP `message(action=list)` 同源） | 历史时间线 | 集成测试 |
| 历史输出 | `GET /api/shells/{id}/output-range` | 片段预览、终端回放 | 集成测试 |
| 历史改名/删除 | `PATCH/DELETE /api/sessions/{id}` | 历史详情 | 集成测试 |
| 资源标签 | 桌面端本地存储 | 连接、会话、Shell 详情与筛选 | 前端测试 |
| 全部/会话转发 | `GET /api/forwards`, `GET/POST /api/sessions/{id}/forwards` | 右侧转发面板 | 集成测试 |
| 关闭转发 | `DELETE /api/forwards/{id}` | 右侧转发面板 | 集成测试 |
| 通知规则 | `GET/DELETE /api/notifications/{id}` | 右侧通知面板 | 列表集成测试、Core handler 测试 |
| 审批默认值 | 连接 TOML 的 `default_approval`；`GET/PUT /api/connections/{name}` | 设置页全局默认值、连接详情与编辑器 | 前端策略测试、Core 集成测试 |
| 会话审批 | `PATCH /api/sessions/{id}/approval` | 会话详情 | Core 集成测试 |
| 待审批操作与决策 | `GET /api/approvals`；`POST /api/approvals/{id}/approve`、`reject`；WebSocket `approval` 事件 | 审批列表、导航角标、应用内提示及可选系统通知 | 前端事件测试、Core 集成测试 |
| 文件浏览 | `GET /api/sessions/{id}/files` | 右侧文件面板 | 集成测试 |
| 上传/下载 | `/files/upload`, `/files/download` | 系统文件对话框 | 集成测试 |
| 重命名/删除/目录 | `PUT/DELETE /files`, `POST /files/dir` | 右侧文件面板 | 集成测试 |

兼容路由仍由 Core 保留，GUI 使用 canonical 路由，避免把旧协议继续扩散到新客户端。

审批默认关闭。桌面端保存全局默认值和单个连接的继承、开启或关闭策略，将实际值同步到连接配置；Core 根据连接配置为新会话启用审批。审批作用于 MCP/AI 发起的 Shell 输入、文件变更和端口转发等受控操作。运行中会话需要在会话详情单独切换；人工 REST/GUI 操作不进入审批队列。

## 本机服务绑定

系统服务操作不经过 Core REST，由 Wails 直接绑定到平台服务管理器。

| Wails 绑定 | 功能 | 平台后端 |
| --- | --- | --- |
| `GetServiceStatus` | 注册、运行、自启、PID、定义与日志位置 | LaunchAgent / systemd / SCM |
| `InstallCoreService` | 写入服务定义并启动同一可执行文件的 `--core-service` 模式 | macOS / Linux / Windows |
| `UninstallCoreService` | 停止并移除服务，恢复应用内 Core | macOS / Linux / Windows |
| `StartLocalCore` / `StopLocalCore` / `RestartLocalCore` | 本机 Core 生命周期 | macOS / Linux / Windows |
| `SetCoreAutostart` | 用户登录或系统启动时自动运行 | macOS / Linux / Windows |
| `GetConversationIndex` | 调用 Core marks 接口读取归档 Shell 索引 | 受限 REST 桥 |
| `SaveConversationLog` | 导出 Core 数据目录内的原始终端日志 | 本机文件系统 |
| `RequestApprovalNotificationPermission` / `SendApprovalNotification` | 请求系统通知权限、发送待审批提醒 | Wails 原生通知接口 |
