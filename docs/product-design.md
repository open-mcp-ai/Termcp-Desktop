# termcp-gui 产品原型设计

## 产品定位

termcp-gui 是 termcp core 的独立图形客户端与本地服务管理端。人通过 GUI、Agent 通过 MCP、程序通过 REST 操作同一个 core 的持久会话。

产品同时承担 Core 管理端与 SSH 管理工具的角色。默认体验：打开 SSH 工作台 → 连接收藏主机或进入已有会话 → 用标签、分屏和平铺组织 Shell → 管理焦点窗格所属会话的文件与转发。Core 管理始终可从侧栏进入。

GUI 应提供两种接入模式：

- GUI 托管：由桌面进程管理层启动本地 core，持有其进程句柄，管理退出、日志与启动参数。
- 外部接入：连接用户已有的本机或远程 core，仅通过服务接口管理资源；其进程启停、升级由宿主管理。

核心能力和数据仍由 core 持有，GUI 不应创建另一套会话后端。关闭 GUI 与停止 core 应是两个独立动作；正式产品需定义托管进程在关闭窗口、退出应用与异常退出时的行为。

## 方案 01：控制中心

页面：Core 管理首页 + 功能侧栏 + 专用终端工作区。

首页突出实例来源、运行状态、地址、数据目录、日志及服务入口。待处理提醒直接定位会话。会话、配置、历史、通知与 MCP 有稳定入口。

优点：定位最符合“管理 core 并替代 WebUI”；生命周期和工作功能容易发现。代价：从首页进入终端多一次操作，可用恢复上次工作页减轻。

## 方案 02：SSH 工作台

页面：分组主机导航 + 工作区标签 + 多终端窗格 + 会话工具 + 主机监控侧栏。

默认以开发与预发布两个主机的并行终端展示布局。工作区可包含不同会话的 Shell，也可在同一 SSH 连接上新建多个 Shell。文件、转发、通知和右侧详情跟随焦点窗格。Core 状态保持在工作区顶部，管理入口位于侧栏。

优点：日常工作连续，终端占主要空间。代价：core 管理不再是第一视觉重点；多窗格需清楚标识当前输入目标。

## 方案 03：资源管理器

页面：资源树 + 面包屑 + 选中资源的工作区。

视图中将会话按连接配置分组。这是导航分组，不意味着连接配置自己持有 SSH Client；实际资源关系仍是每个会话持有一条连接，Shell 和转发属于会话。所有 Shell 平等，不设置“主 Shell / 子 Shell”产品层级。

优点：适合多个实例与环境，资源所属关系清晰。代价：资源数量增加时需要折叠、搜索、收藏及空状态；原型只演示本地实例和外部未连接状态，未实现多个在线实例。

## 推荐组合

采用方案 01 的导航与 Core 管理，方案 02 的终端操作区。方案 03 的资源树作为导航视图切换，在接入多个 core 后引入。

MVP 同时提供单本地 core 管理、SSH 主机入口和可分屏的终端工作台。多 core、监控采集和桌面系统集成分阶段接入。

## WebUI 能力迁移矩阵

依据当前本地 core 的 README、docs/api.md、internal/webui/handler.go、ws.go 与前端资源梳理。下列路径用于正式接入；原型没有请求这些接口。

| 现有能力 | GUI 入口 | Core 接口 / 实现方式 | 当前原型 |
| --- | --- | --- | --- |
| 会话列表、新建、改名 | 会话 | GET/POST /api/sessions；PATCH /api/sessions/{id} | 列表、新建、查找；运行会话改名待接入 |
| 结束会话并保留历史 | 会话 / 工作台 | POST /api/sessions/{id}/terminate | 模拟结束与归档 |
| 删除会话并清理历史 | 会话管理 | DELETE /api/sessions/{id}，与 terminate 语义分开 | 归档删除模拟；运行会话永久清理待接入 |
| 多 Shell 创建、关闭 | 工作台 | GET/POST /api/sessions/{id}/shells；DELETE /api/shells/{id} | 模拟创建、关闭、退出状态 |
| PTY 输入、输出、尺寸变化 | 工作台 | WebSocket /api/ui/ws；使用真实 Shell ID，协议见 ws.go | 文本模拟；xterm、PTY resize、真实流待接入 |
| 会话变更、人工通知推送 | 全局 / 工作台 | 同一 WebSocket 的 sessions / notify_user 消息 | 演示提醒；真实推送待接入 |
| 多终端分屏 / 平铺 | 工作台布局 | GUI 嵌套布局树 + 多 Shell 订阅 + 各窗格 resize | 嵌套左右/上下分屏、平铺、调整比例、最大化/收起；真实订阅与 resize 待接入 |
| 连接模板、SSH 配置 CRUD / 测试 | 连接配置 | /api/connection-templates；/api/connections；POST /api/connections/test | 新建、编辑、详情、删除、模拟测试、分组收藏和跳板引用；配置复制待接入 |
| 本机连接与能力门控 | 连接配置 / 设置 | internal 配置；--no-internal | 演示配置与开关 |
| 浏览文件、上传下载、重命名、删除、建目录 | 会话文件页 | /api/sessions/{id}/files 及其 download/upload/dir 路由 | 预览、建目录、添加演示文件；完整操作待接入 |
| Local / Remote / Dynamic 转发 | 会话转发页 | GET/POST /api/sessions/{id}/forwards；DELETE /api/forwards/{id} | 模拟创建和关闭 |
| 通知规则查看与移除 | 通知规则 | GET /api/notifications；DELETE /api/notifications/{id} | 模拟列表与移除；GUI 不创建规则 |
| 历史、正文、搜索、改名、标签、备注 | 历史记录 | /api/history；/api/history/search；PATCH /api/history/{id}；transcript | 元数据过滤、改名/标签、模拟正文；正文搜索/备注/分页待接入 |
| 历史 PNG 截图 | 历史输出 | GET /api/history/{id}/screenshot | 未实现 |
| MCP / REST 配置速查 | API / MCP | HTTP /stream；SSE /sse；REST /api/* | 可复制示例 |
| 资源 URL 复制 | 工作台 / 配置 | termcp://配置名；termcp://#会话ID；termcp://#会话ID:N | 演示会话与 Shell URL；N 按 core 返回的 Shell 列表索引生成 |
| 静态 Token 鉴权 | Core 接入 / 设置 | Bearer、Basic 或 cookie，参照 internal/auth | 输入与校验模拟；凭据不保存 |

当前 core 的 REST/WS 接口位于 internal/webui。独立 GUI 替换网页静态资源时，不能一并移除该包注册的 API 与 WebSocket 服务。

## Core 管理新增能力

当前 API 未提供进程生命周期、版本协商、安装/升级或进程日志端点。不能假设存在 /api/core/start 或 /api/health。

本地桌面管理层需要补充：

1. 定位/导入 core 二进制，校验平台与可执行性，读取可用版本信息。
2. 创建托管进程，捕获 stdout/stderr，检测端口冲突、退出码与启动失败。
3. 在启动后利用已有 API 探测可用性，区分未运行、连接失败、认证失败和版本不兼容。
4. 持有托管进程身份，仅对该进程实施启停；发现已有服务时转为外部接入。
5. 保存实例元数据、启动参数与最近访问状态；正式版认证信息进入系统凭据库。
6. 评估重启影响后停止进程，应用参数，再启动；旧 SSH 连接不自动恢复。
7. 后续版本支持安装/升级、数据备份、迁移与回滚，当前原型不模拟成功升级。

建议将 GUI 分为视图层、core API 适配层、桌面进程管理层。当前原型可独立审阅；原生实现主推 Rust + GPUI 与 Go core，另评估 Go + Fyne。原型 HTML 不作为原生技术选型。

## 状态与交互边界

- “进入输入”只是 GUI 本地切换，不能阻断其他 MCP 客户端的输入。“接管”若表示独占权限，需要 core 明确实现控制权协议。
- Ctrl+C 中断当前前台程序，不禁止 Agent 再次输入。暂停 Agent 需要客户端协调或新的 core 能力。
- 关闭 Shell 和结束会话分开；结束会话级联关闭 Shell 与转发。
- PTY Shell 自然退出保留只读输出，会话仍可创建 Shell。手动关闭直接移除 Shell。
- SSH 断线应展示 exited/只读状态、保留输出，并允许新建同配置会话；不能把它画成自动恢复的会话。
- 文件和转发仅对活跃连接可操作；历史视图只读，不提供活动文件/转发工具。
- 外部 core 未连接时不展示其会话数据。实际多 core 数据需要独立缓存和实例标识，不能混用 Session ID。
- 凭据不可读回；原型字段内容不存入 localStorage、文件或日志。
- core 停止期间 GUI 不假设能读取其远程历史；重新连接后从持久化接口加载。

## 原型验证

执行 npm run check 校验脚本语法；用浏览器检查三种方案、Shell 创建与输入、SOCKS 转发、资源树切换、Core 重启归档、新建会话、设置校验与窄窗口导航。详细验证记录见 docs/verification.md。

## 主机详情与紧凑操作

右侧参考 [XTerminal 官方界面导览](https://docs.xterminal.cn/getting-started/interface-overview/)的系统、CPU、内存、网络与磁盘监控分区，以及 [HexHub 官方 SSH 工作区截图](https://www.hexhub.cn/imgs/ssh1.png)的紧凑状态工具条。

默认显示深色监控面板。termcp 标签集中展示连接参数、Shell 列表与资源 URL，管理和复制使用图标按钮。收起后显示 CPU、内存、磁盘摘要和操作按钮。详情始终跟随焦点窗格，不按整个工作区混合汇总。

监控是静态演示快照，当前 core 没有 telemetry API。新添加主机显示“尚未采集”，不套用示例数据。正式版需增加独立采集通道、采样时间与失败状态，不能把监控命令注入用户正在操作的 Shell。

窗格的收起、移除、最大化与工作区关闭只改变 GUI 视图。关闭 Shell、结束会话、停止 Core 保留各自明确的资源语义。完整工作区关系见 [SSH 工作台设计](ssh-workspace-design.md)。Rust / Go 原生桌面端建议见 [技术选型](native-stack.md)。
