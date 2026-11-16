# Termcp Core v0.2.9 升级研究记录

核查日期：2026-11-13。目标版本限定为正式发布的 `v0.2.9`，包含 `v0.2.9-rc.1`、`v0.2.9-rc.2` 累积变化；上游固定提交为 `714f8d491d0913fd7012afadc7f22f062e213839`。正式发布于 2026-11-08，官方 compare 报告 146 个提交。本文依据该提交的官方源码归档，不以其他开发分支的代码推断正式版功能。[Release][release]、[版本比较][compare]、[固定提交][commit]

## 1. 接入结论与阻塞

`v0.2.9` 新增公开 `termcp/core`、`termcp/config`、`termcp/term`、`termcp/term/server` 和 `termcp/mcp` 包。Desktop 可以用公开 Core 装配整个运行时，避免继续逐项构造 session、storage、MCP、SSH、扩展和审批依赖。但正式版的 `core.wireUI()` 存在一个与 Desktop 需求冲突的装配边界：**只去掉上游浏览器页面时，会同时跳过 Desktop 依赖的 REST、WebSocket 与部分运行时接线。** 因此，直接改模块版本、加 `no_webui` 标签不满足“所有功能兼容”。[公开 Core 装配与分支][core-wiring]

具体路径如下：

| 目标形态 | 正式 v0.2.9 的源码行为 | 对 Desktop 的影响 |
| --- | --- | --- |
| 普通编译，`Headless=false` | `core.wireUI()` 构造 `webui.Handler` 并调用 `Register`；同时嵌入上游全部页面资产 | API 齐全，但不满足上游 WebUI 不进入产物的要求 |
| `-tags no_webui`，`Headless=false` | `Embedded()` 为 false，`wireUI()` 在构造 Handler 前直接返回 | `/api/sessions`、`/api/version`、`/api/ui/ws`、审批、文件、配置等未注册；注释声称保留的 `/api.md` 也未在该分支注册 |
| `Headless=true` | 只单独挂载 `/api.md` 后返回 | 仍无上述 Desktop REST 与 WebSocket；普通编译时页面字节仍被 embed，运行时开关不能代替编译标签 |
| `no_webui` 下手工构造并注册现有 `webui.Handler` | `Handler.Register()` 始终挂 API，末尾按 `uiEmbedded` 只保留 `/api.md`，无页面静态入口 | 说明纯 API 能力已存在于内核后端，但 public Core 的装配分支没有利用它 |

`no_webui` 选择的 `embed_no_webui.go` 只 embed `assets/api.md`，不会嵌入上游 HTML、JS、CSS、主题与 xterm 资产。后台 REST/WS handler 仍属于 Core 功能，不能因为 Go 包名中有 `webui` 就一并删除。[编译标签与唯一保留文档][embed-api]、[正常资产构建][embed-full]、[Handler 的 API/静态路由注册][routes]

另一个必须一起修复的装配点是 `SetExtensionReconciler()`：正式版把它放在 `wireUI()` 的正常页面分支中；`no_webui`/headless 早退时，MCP Server 没有收到已构造的扩展 reconciler。因此即使只使用 MCP，扩展命令和 `/ext/{name}/stream` 也不能仅凭路径已注册就宣称可用。[扩展与事件接线][core-wiring]、[扩展 MCP handler 对依赖的检查][ext-http]

推荐先修上游 Core 的装配边界：把 REST/WS、审批执行回调、扩展 reconciler、客户端变化、会话/输入/审批通知作为无页面 API 模式的必需接线；静态页面注册和资源嵌入单独处理。需要有 `no_webui` 下的功能测试，证明 API/WS/扩展完整并且 `/`、静态资源不可用。`Headless` 的既有语义及对应文档应明确，不能把“页面关闭”暗中变成“管理 API 关闭”。这是待开发内容，**不是正式 v0.2.9 已有保证**。[现有 Core 分支][core-wiring]、[现有 Handler 注册能力][routes]

如果必须先验证严格的 `v0.2.9` 依赖，可临时适配旧的手工装配并选用 `no_webui`，复用内核 Handler；它需要跟进新 MCP/配置等接口，且继续依赖 `internal/*`，应标为过渡方案。不能通过复制 REST handler、复制 SSH/SFTP/审批实现或重新编译 WebUI 来绕开上述阻塞。长期方案以公开 Core 装配为准；修复后的依赖必须锁定可复现的发布版本或固定提交，不能把本地补丁称为原版 `v0.2.9`。[Core 提取的接口边界][core-api]

## 2. 公开 Core API 与 Desktop 生命周期

| API | 真实契约 | 接入要求 |
| --- | --- | --- |
| `core.New(core.Options{Config, Version})` | 构造运行时与路由，不绑定端口 | 明确设置原有数据目录、loopback 地址和版本；构造后始终安排失败清理 |
| `Start()` | 同步启动内部 SSH、MCP 文档提示和扩展 reconciler，不监听端口 | 返回成功后可直接使用；不再把旧的 MCP `Start(addr)` 当 HTTP listener |
| `Listen()` | 要求先 Start；验证监听配置；阻塞到关停 | 使用它时由 Desktop 包装 goroutine/结果 channel，等待停止完成后才重启或切换服务 |
| `Handler()` | 返回同一 HTTP handler，包含 Core 配置的 auth middleware | 自管 listener 时使用 `Start()+自己的http.Server`，不再调用 Core.Listen；外层服务器由 Desktop 负责关闭 |
| `Close(reason)` | 幂等请求停止；有 Listen 时资源释放由 Listen 完成；无 Listen 时自身完成资源释放 | 不能把 Close 返回当作后台 Listen 已退出；必须有有界关停及端口释放验证 |
| `WaitReady(ctx)`、`Addr()` | 可等待就绪；Addr 是配置地址，不等于已监听的证明 | 健康检查需核对真实响应版本与实例，不能只判断端口有响应 |
| `MCP()`、`Term(ctx,cmd,args)` | 暴露公开 MCP Server，以及同一 TERM/1 执行入口 | 程序调用可复用，Desktop 的已有文件/终端/历史界面仍应使用 Core 原有管理 API |

上述公开入口及实现都在一个文件中。`Options` 仅有 `Config`、`Version`、`DaemonChild`、`IdleTimeout`、`IdleTimeoutSet`；没有可注入的 mux、HTTPServer、SessionManager、storage、加密服务或自定义 route hook。`config.Default()` 的 `DataDir` 为空，而 `ValidateInstance()` 拒绝空目录，嵌入方必须自行解析并设置，不应照搬 Core 文件头的旧注释。[公开 API][core-api]、[配置默认值与验证][config]

Desktop 如需保留统一日志中间件，可用自管 loopback HTTP server 包裹 `srv.Handler()`。此时需要显式调用监听配置校验，保留 `127.0.0.1:18765` 约束；Core 自己的 `Validate()` 原本在 `Listen()` 才执行。先停止外层 HTTP/WS 接入，再关闭 Core 资源，并等待已跟踪的 goroutine 退出，才能开始下一次启动。三平台系统服务与进程内模式应调用同一装配函数，避免两套能力发生差异。[Handler/Listen/Close 源码][core-api]、[认证与监听验证][config]

## 3. v0.2.8 → v0.2.9 的运行时改动

下表按 Desktop 的接入影响归纳。RC 章节也属于正式版升级范围；部分 changelog 同时重述 v0.2.8 已有的按需启动等行为，不能把重复描述当作全新功能。[正式版与两个 RC 的变更记录][changelog]

| 范围 | 目标版本的功能或行为 | Desktop 兼容重点 |
| --- | --- | --- |
| 公开库与配置 | TERM/1 解析/信封/排序与 HTTP 协议层公开；Core 可嵌入；flag/env 从 Config 标签统一生成，显式 flag 优先 | 迁移内部包引用；桌面设置与服务数据目录显式传递，禁止由环境变量意外覆盖桌面固定监听设置 |
| MCP 实现 | 上游改用自己的公开 `termcp/mcp`，移除 mcp-go 依赖；SSE、streamable HTTP 仍是内核服务面 | 使用真实 MCP 客户端验证 initialize、tools/list、tools/call、prompts、SSE/streamable、并发与清理，不以编译通过替代协议兼容 |
| TERM/1 回复与调用 | `content:false`、`structured_content`、`dry_run`；生成式回复统一遵守 `bytes_limit`；删除无人消费的 `on_time_overflow`；syntax Error-Code 统一为 `invalid_argument`，补 502/504 reason phrase | 保留现有 REST/MCP 入口；测试参数与结构化 envelope；未知/删除参数不得静默表现为成功 |
| 会话与通道 | 可用名称寻址并拒绝歧义；暴露检测到的 shell 家族；借用 `use:<host>` 会话可观察；`--later` 延后关闭 | 接收新增 `use_host`、`detected_shell` 等字段；旧会话/通道 ID 与稳定 index 继续工作，名称歧义应正确显示失败 |
| SSH、PTY、pipe、关闭 | 传输池/请求租约、会话生命周期隔离；本地 exec 经过 login shell；PTY quiet threshold 自适应；输入命令文本和 Enter 串行；阻塞 stdin、桥接、关闭、转发等等待有界 | 本地与远端、PTY 与 pipe 均回归；关闭一个资源不影响其他会话；受阻链路不拖死全局操作；退出后仍可读归档 |
| Windows | 本地 pipe 子进程使用 `CREATE_NO_WINDOW`，不抑制 GUI 程序窗口；删除归档时对短暂文件占用有界重试 | 原生 Windows 服务与窗口测试，不只交叉编译 |
| 文件与 SFTP | 每会话复用 SFTP 引擎，REST 文件面板与 MCP 共享；internal/use-host 跨节点复制不再 panic/被默认 bytes_limit 截断；UTF-16 BOM 文本识别；CRLF 行号修复 | 大文件逐字节一致；internal/remote/use-host 路径均测试；上传下载不走文本桥；保留文件能力/路径白名单 |
| 安全边界 | 删除绕过 `--no-internal` 的 `write --local-path`；变量只能填值，不能成为语法；文件管道完整写后原子替换 | 不为兼容恢复已移除的越界行为；同一安全拒绝必须在 Desktop、REST、MCP 保持一致 |
| 审批与事件 | 到期由时钟确定；关闭后仍可读取已取消队列；记录执行状态；新增 `GET /api/approvals/{id}`；WS watch cursor 原子性修复 | Pending/approved/rejected/expired/cancelled 与执行结果均可区分；批准不等于执行成功；人工输入与 MCP 受控操作边界保持内核语义 |
| 归档与元数据 | 持久化顺序和竞争修复；归档按结束时间排序；关闭/归档保留记录与结束原因 | 旧 manifest/log/marks 可读；结束时间/原因正确；单 Shell 关闭与永久删除继续分开 |
| 扩展 | 多主机、模板/本地值分离、typed holes、预览、resident、idle_timeout=0、URL native MCP peer、独立 MCP endpoint、TERM/1 下游声明 | 见下一节；仅沿用 v0.2.8 单目标卡片和字段序列化会丢失能力 |

以上行为可在 [changelog][changelog]、[Core][core-api]、[公开数据类型][api-types]、[TERM 参数表][term-args]、[审批实现][approval]、[SFTP 与文件后端][file-api] 直接核对。这里是实现前研究结论；平台实际运行结果应另列验证记录。

上游浏览器页面还包含 xterm.js 6、分屏/合并窗口、移动端布局、标签顺序/闪烁、语言切换和静态 ETag 等改动。它们不是 Desktop 内核依赖的编译输入。Desktop 应保留自己的终端、布局、国际化与资产；是否独立升级自己 vendored 的 xterm 应由兼容回归决定，不能复制上游页面来获得“功能兼容”。[上游浏览器改动记录][changelog]、[xterm 上游说明][xterm]

## 4. 扩展升级的字段与数据契约

- **不再按 `kind` 分类**：正式版 `Extension` 与列表/详情摘要没有旧的扩展 `kind` 字段，默认能力和提示按 `command`、`url`、`paths` 等实际声明推导。Desktop 现有表单仍提交 `kind`，卡片还用缺少 `kind`/`target` 判断模板无效；多主机扩展的顶层 `target` 可以为空，因此必须改掉这一判断。旧的 MCP/skill/prompt/custom 入口如保留，只能作为填写真实字段的创建预设，不能让标签决定能力或安全限制。旧模板升级应核对实际 command/allow/paths/notes 行为，不能仅以 TOML 可解析作为兼容证明。[配置形态][extension-config]、[默认能力推导][extension-preset]、[列表摘要][extension-api]
- **数据形态**：支持旧 `target = "profile"` 与顶层 `[[forward]]`，也支持 `[[targets]]` 多主机及各主机的转发。目标数量、转发数量、状态、失败、下次重试需要按主机显示，不能只取首项。`resident=true` 表示始终运行，`idle_timeout="0"` 表示禁用空闲回收；声明转发的扩展也具有常驻理由。[配置定义与兼容归一][extension-config]
- **模板和安装值**：`config.toml` 是可升级模板，`config.local.toml` 是本次安装的值。`"{{name}}"` 必须占据完整字符串值，`[params.name]` 声明类型和说明；类型支持 string/integer/number/boolean/array/table，缺值是 installed-and-waiting 状态。保存模板不能把渲染后的值反写回模板；替换模板不能删除本地值。[holes 与类型规则][holes]、[保存模板与 values 语义][extension-store]
- **REST 形态**：列表/WS 行新增 `targets`、`host_count`、`forward_count`、`holes`、`needs`；`GET /api/extensions/{name}?format=toml` 返回模板原文；详情展示安装值与 holes；`POST /api/extensions/{name}/preview` 用已存本地值渲染并验证提交的模板；编辑支持 `{template,values}`。空值、类型错误、已保存但尚未生效的 values 应沿用内核结果，而不是报成扩展不存在。[列表、详情、预览与编辑源码][extension-api]
- **导入导出**：批量导出读取模板原文，不带 `config.local.toml` 的安装值；导入撞名使用可用后缀，不覆盖现有模板。单模板更新保留本地值；Desktop 必须测试未知键/holes、命名冲突、未填值模板和模板更新后的值保留，不能把 JSON 表单缺少的字段默默清空。[批量导入导出][extension-batch]、[values 保存][extension-store]
- **URL native MCP peer**：URL 扩展挂载为可直接 `get <peer> tools`、`call <peer> <tool>` 的 MCP 对端；`open ext` 默认返回摘要，`--all` 才给全文。编辑 URL/headers 导致模板指纹变化时，应替换旧 mount；关闭或删除后旧 peer 地址失效。[URL peer 生命周期][ext-peer]、[指纹字段][extension-config]
- **原生 MCP endpoint**：`/ext/{name}/stream` 提供扩展自身的 MCP 接口，沿用扩展生命周期与能力/审批策略；它的 GET/DELETE 语义与主 `/stream` 不同，不应由 Desktop 自己猜测或转写。[扩展 HTTP 协议实现][ext-http]
- **TERM/1 下游**：URL 模板可声明 `termcp_tool`；内核验证工具存在及输入 schema 后才允许 `run` 路由，不合格时保留普通 peer 能力并报告原因。[下游声明与验证][ext-term]

## 5. 公开接口仍缺少的 Desktop 能力

公开包只有 Core 装配、Config、TERM/MCP 协议与 `pkg/api` DTO；没有 public storage/configcrypt 包，也没有公开 SessionManager/SFTP accessor。`internal/configcrypt` 仍提供导入导出信封加密，但不属于公开库契约。已经存在的管理 REST 应直接复用；不能为了迁移公开 Core 而复制加密或 manifest 逻辑。[Core 的私有依赖][core-api]、[公开 DTO][api-types]、[导入导出加密实现][configcrypt]

**本次升级可采用的兼容边界**：运行时装配和生命周期使用公开 `core/config`；现有历史读取确需的 `internal/storage` 只读调用，以及已有 `internal/configcrypt` 导入导出处理，在一层薄 Core 适配器中继续直接调用内核函数。它们不构造第二套运行时、不自行实现存储或密码算法、不改变记录语义，符合“仅调用内核、不编译上游 WebUI 前端”。应固定模块版本、集中内部引用并覆盖旧归档含子 Shell 的回归，未来公开窄接口成熟后再迁移；“所有内部包引用清零”不是用户这次要求，也不应因此丢失功能。后端 API handler 的 Go 代码继续由 Core 编译，前端页面资产由 `no_webui` 排除，二者需要在构建验收中区分。[Core 依赖与封装][core-api]、[无前端资产构建][embed-api]

固定版本没有 Desktop 自定义的 `files/default-directory` canonical REST 路由；`GET files?path=.` 只给文件名/目录子项等信息，不返回绝对工作目录。不过已有公开 TERM 操作 `get <session-id> file . --op getwd`，经内核能力检查及共享 SFTP 池执行 `Getwd()`，返回 `directory`；对远端等价于现有 Desktop 的 SFTP Getwd，可通过 `srv.Term()` 或 `/api/term` 的结构化结果复用，因此无需为远端默认目录增加上游硬阻塞。对 internal，这一操作返回本地 SFTP 服务的工作目录，未保证等于原 Desktop 的用户 Home；应保留桌面原生 `os.UserHomeDir()` 这一小适配，或明确扩展 Core 的 home 契约，并验证三平台路径，不能把 `.` 当作 Home。[全部注册路由][routes]、[文件路径与结果字段][file-api]、[TERM getwd handler][file-getwd]、[getwd 的官方测试语法][file-op-test]、[内部 SFTP 构造][internal-sftp]

历史导出与归档完成确认优先从 Core 输出/marks/session/shell API 取得。旧历史读取中现有 Core storage 函数提供而管理 API 尚未等价公开的能力，可以保留在上述适配器中；不得由 Desktop 自行解码磁盘 manifest 成为第二套存储实现。默认目录的远端 TERM 与本地 Home 适配则应通过 managed/attached 共用的 HTTP 桥，避免依赖私有 SessionManager。[Shell 与归档路由][routes]、[数据类型][api-types]

## 6. Integration 文档与工具链、版本范围

Desktop 当前 Integration 页生成 `/skills.md` 及下载为 `SKILL.md` 的 curl 安装片段，但 v0.2.9 固定源码全量文件清单中没有 skill 文档，`no_webui` 只保留 `api.md`，也没有分发 skill 的 CLI 入口。`Makefile` 中仍声称 API 构建保留 `/SKILL.md`，属于与源码不一致的旧注释。上游实际给出的纯 HTTP 使用方式是读 `/api.md`、调用 `/api/term`，通过空调用与 `help` 获取指令自述。[Desktop Integration 页][desktop-integrations]、[API 编译资产][embed-api]、[过时构建注释][makefile]、[上游纯 curl 文档][clients-doc]

完整兼容方案应同时修复 Integration 页：API/MCP 地址继续指向当前内核实例，不得展示无效的上游 skill URL。如果保留现有“安装 HTTP skill”能力，可由 Desktop 维护一份薄 `SKILL.md`，仅指引访问 Core `/api.md`、`/api/term` 与 `help`，通过文档导出或宿主文档路由分发；它不包含 WebUI 资产或第二套业务实现，但应明确是 Desktop 的文档交付物，不能称为 v0.2.9 自带文件。仅隐藏旧入口能修复死链接，不能单独视为完整保留该能力。

`v0.2.8` 与 `v0.2.9` 的 Go 版本均为 `1.27.3`。v0.2.9 从模块中移除了 `mark3labs/mcp-go`、`google/uuid` 及部分传递依赖，使用自己的公开 MCP 与 ID 实现。Desktop 的 mcp-go 使用位于 `core_e2e_test.go`，可以保留它作为外部协议兼容客户端，不应仅因内核不再使用就机械删除。[v0.2.8 go.mod][gomod-old]、[v0.2.9 go.mod][gomod]、[Desktop 外部 MCP 客户端测试][desktop-mcp-test]

正式版认证是 token/hash/DisableAuth，支持 Bearer、Basic 及 `termcp_token` cookie；不是后续开发中的 URL-only OAuth、OIDC 用户归属与 Agent 权限恢复。固定目标源码的 Config、Core 与 auth 均无这套实现。配置批量文件的密码保护仅用于导入导出，API 文档明确说明它不是配置静态加密。升级 issue 应把这些后续需求作为另一项版本升级处理。[目标 Config][config]、[目标认证][auth]、[目标导入导出文档][batch-doc]

## 7. 证据与验证边界

本记录未修改业务代码或依赖，也未执行三平台回归。父任务在临时目录对固定版本运行了公开 `core.Start()+Handler()` 探针：普通编译时 `GET /api/version`、`sessions`、`extensions`、`approvals`、`clients` 均为 200，`/api/ui/ws` 的非 Upgrade 请求为 400；`no_webui+Headless=false` 时上述路径与 `/api.md` 均为 404，`POST /api/term` 执行 `ls ext` 为 422 / `not_configured`；`no_webui+Headless=true` 仅把 `/api.md` 恢复为 200。`go list -tags no_webui` 的 `EmbedFiles` 为 `[assets/api.md]`。这些结果与上述源码分支一致。

另在临时副本中，仅升 Desktop 的 Core 版本会编译失败，首个错误是 `internal/config` 已不存在；临时迁移到公开 `config` 后，还会遇到 `RegisterBuiltin` 缺少 ViaDialer 参数，以及旧 `WithHTTPServer` 已不存在。因此升级必须替换装配契约，不能承诺仅更新 go.mod 即完成。

父任务还在临时 Core 快照验证最小修复方向：移除 `no_webui` 早退并使 `Embedded()` 正确报告页面是否嵌入后，`Headless=false` 的 `/` 为 404、`/api.md` 与管理 API 为 200、WS 非 Upgrade 请求为 400、`ls ext` 为 200。这只是局部可行性实验，不代表上游已合并修复，也不代表完整扩展、审批或三平台功能已通过回归。

结构发现先使用已安装的 codebase-memory 技能。现有 Core 图索引对应另一个开发 checkout，generation 为 `2026-11-13T02:23:44Z`；覆盖查询发现公开 `core/`、`config/` 等目标文件不在该图中，且即使其他文件记录 metadata_match，也不能证明它们等于发布 tag。故本文对目标版本的结论全部直接核对官方固定提交源码归档；未把当前图的新增认证/配置等功能当作 v0.2.9 的证据。

[release]: https://github.com/open-mcp-ai/Termcp/releases/tag/v0.2.9
[compare]: https://github.com/open-mcp-ai/Termcp/compare/v0.2.8...v0.2.9
[commit]: https://github.com/open-mcp-ai/Termcp/commit/714f8d491d0913fd7012afadc7f22f062e213839
[changelog]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/CHANGELOG.md#L5-L118
[core-wiring]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/core/core.go#L250-L310
[core-api]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/core/core.go
[embed-api]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/webui/embed_no_webui.go
[embed-full]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/webui/embed_full.go
[routes]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/webui/handler.go#L135-L298
[config]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/config/config.go
[gomod]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/go.mod
[gomod-old]: https://github.com/open-mcp-ai/Termcp/blob/v0.2.8/go.mod
[api-types]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/pkg/api/types.go
[term-args]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/mcp/term_args.go
[approval]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/webui/approval.go
[file-api]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/webui/handler_file.go
[xterm]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/docs/xterm.md
[extension-config]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/extensions/config.go
[extension-preset]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/extensions/preset.go#L110-L151
[holes]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/extensions/holes.go
[extension-store]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/extensions/store.go
[extension-api]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/webui/handler_extension.go
[extension-batch]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/extensions/batch.go
[ext-peer]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/mcp/ext_peer.go
[ext-http]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/mcp/ext_mcp.go
[ext-term]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/mcp/ext_termcp_tool.go
[configcrypt]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/configcrypt/configcrypt.go
[auth]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/auth/auth.go
[batch-doc]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/webui/assets/api.md#L211-L243
[file-getwd]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/mcp/handlers_file.go#L594-L607
[file-op-test]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/mcp/file_ops_surface_test.go#L46-L50
[internal-sftp]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/internal/sshserver/server.go#L163-L175
[desktop-mcp-test]: https://github.com/open-mcp-ai/Termcp-Desktop/blob/da11d899b10853f405eed3316ac8adf48f73e250/core_e2e_test.go
[desktop-integrations]: https://github.com/open-mcp-ai/Termcp-Desktop/blob/da11d899b10853f405eed3316ac8adf48f73e250/frontend/src/features/integrations/view.js#L24-L52

## 2026-11-16 当前状态复核与实施记录

本节是在开始实现前对 2026-11-13 历史研究的重新验证，不覆盖上面的历史证据。

- 上游 `v0.2.9`、`release/v0.2.9`、`dev` 与 `v0.2.10-rc.1` 均仍保留 `wireUI()` 的 `!Embedded()` 早退；没有可直接锁定的已发布 API-only 修复。不能改用包含后续 OAuth/Agent/静态加密功能的开发版本冒充 v0.2.9。
- 用固定提交重新运行 `no_webui` 探针，结果与历史记录一致：管理 API、`/api.md`、WS 为 404，`ls ext` 为 422 `not_configured`。
- 最小 Core 修补后，`no_webui + Headless=false` 保留 REST/WS、审批执行回调、扩展 reconciler、客户端/UINotify 事件和 `/api.md`；页面与 static/theme/xterm 路径仍为 404。`Embedded()` 按实际页面资产返回 false。
- URL 模板还存在同任务相关缺口：relay 必须有 pipe channel，否则 `/ext/{name}/stream` 返回 409。修补后 URL 模板自动获得 pipe relay，且始终不是 PTY。
- URL 模板更新后，直接调用旧 `mcp:<n>` handle 原本仍可用到下一次 `open ext`。修补后按模板 fingerprint 拒绝旧 handle，要求重新打开扩展。
- 当前交付采用相对路径 patch module：`require termcp v0.2.9` + `replace => ./third_party/termcp-core`，实际版本显示 `0.2.9+desktop.1`。上游发布等价修复后必须改成不可变发布版本并移除 replacement。
- 本轮验证详情见 `docs/verification.md`；Windows/Linux 原生运行和三平台系统服务生命周期仍是发布前未验证项。
[makefile]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/Makefile#L36-L41
[clients-doc]: https://github.com/open-mcp-ai/Termcp/blob/714f8d491d0913fd7012afadc7f22f062e213839/docs/clients.md#L218-L237
