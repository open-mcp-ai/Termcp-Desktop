# Core 接口覆盖

Termcp 通过受限 Wails API 桥访问当前 Core 的 `/api/` 路径。终端使用同一 Core 的 WebSocket；上传和下载由 Go 流式处理并调用系统文件选择器。

| 能力 | Core 接口 | GUI 入口 | 自动验证 |
| --- | --- | --- | --- |
| 连接模板 | `GET /api/connection-templates` | 新建连接 | 集成测试 |
| 连接列表 | `GET /api/connections` | 资源树 | 快照与集成测试 |
| 连接详情 | `GET /api/connections/{name}` | 编辑连接 | API 测试 |
| 保存/重命名连接 | `PUT /api/connections/{name}` | SSH 配置编辑器 | 集成测试 |
| 删除连接 | `DELETE /api/connections/{name}` | 连接详情 | 集成测试 |
| 测试连接 | `POST /api/connections/test` | 编辑器与连接详情 | 集成测试 |
| 会话列表/创建 | `GET/POST /api/sessions` | 资源树、新建会话 | 集成测试 |
| 会话详情/重命名/删除 | `GET/PATCH/DELETE /api/sessions/{id}` | 会话详情 | 集成测试 |
| 结束并归档 | `POST /api/sessions/{id}/terminate` | 会话详情 | 集成测试 |
| Shell 列表/创建 | `GET/POST /api/sessions/{id}/shells` | 会话详情、终端窗格 | 集成测试 |
| 关闭 Shell | `DELETE /api/shells/{id}` | Shell 详情 | Core handler 测试与 UI 验证 |
| 输出分页 | `GET /api/shells/{id}/output-range` | 终端启动恢复 | 集成测试 |
| 终端流 | `GET /api/ui/ws` | xterm 工作区 | 浏览器协议适配与原生构建 |
| 历史列表/搜索 | `GET /api/history`, `/search` | 历史页 | 集成测试 |
| 历史详情/编辑/删除 | `GET/PATCH/DELETE /api/history/{id}` | 历史详情 | 集成测试 |
| 正文与截图 | `/transcript`, `/screenshot` | 查看与导出 | 集成测试 |
| 全部/会话转发 | `GET /api/forwards`, `GET/POST /api/sessions/{id}/forwards` | 右侧转发面板 | 集成测试 |
| 关闭转发 | `DELETE /api/forwards/{id}` | 右侧转发面板 | 集成测试 |
| 通知规则 | `GET/DELETE /api/notifications/{id}` | 右侧通知面板 | 列表集成测试、Core handler 测试 |
| 文件浏览 | `GET /api/sessions/{id}/files` | 右侧文件面板 | 集成测试 |
| 上传/下载 | `/files/upload`, `/files/download` | 系统文件对话框 | 集成测试 |
| 重命名/删除/目录 | `PUT/DELETE /files`, `POST /files/dir` | 右侧文件面板 | 集成测试 |

兼容路由仍由 Core 保留，GUI 使用 canonical 路由，避免把旧协议继续扩散到新客户端。

## 本机服务绑定

系统服务操作不经过 Core REST，由 Wails 直接绑定到平台服务管理器。

| Wails 绑定 | 功能 | 平台后端 |
| --- | --- | --- |
| `GetServiceStatus` | 注册、运行、自启、PID、定义与日志位置 | LaunchAgent / systemd / SCM |
| `InstallCoreService` | 写入服务定义并启动同一可执行文件的 `--core-service` 模式 | macOS / Linux / Windows |
| `UninstallCoreService` | 停止并移除服务，恢复应用内 Core | macOS / Linux / Windows |
| `StartLocalCore` / `StopLocalCore` / `RestartLocalCore` | 本机 Core 生命周期 | macOS / Linux / Windows |
| `SetCoreAutostart` | 用户登录或系统启动时自动运行 | macOS / Linux / Windows |
