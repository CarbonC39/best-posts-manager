# 开发 TODO

按纵向节点快速交付。每个节点完成后都应有一个能在本地运行、能被用户实际使用的结果；节点内的条目可以并行，但不把工作拆成无法独立验证的碎片。复选框表示计划状态，完成实现后勾选并附对应提交或测试。

## 节点 1：本地核心 + Mastodon-compatible 闭环

依赖：无。目标是从“创建内容”走到“在一个 Mastodon-compatible 实例成功发布并看见记录”。

- [x] 建立 Go 单体入口、Vue 3 + TypeScript SPA、SQLite 迁移和本地媒体目录；默认只监听回环地址。
- [x] 实现 Profile、Account、Topic、BaseDraft、Variant、Media、Publication 的最小创建/读取和必要编辑；Account 必须且只能属于一个 Profile，不设计共享 Account。
- [x] 实现从 BaseDraft 生成 linked Variant（记录 source revision）；源稿变化只标记 `outdated`，用户确认应用更新/重新生成后才更新，首次直接编辑自动转 detached；从其他 Variant 复制默认 detached；提供预览和图片上传。
- [x] 实现 Mastodon-compatible adapter：实例 URL/令牌配置、文本与媒体校验（只报告平台限制）、媒体上传、可见性/内容警告、发布结果归一化；获取并按实例缓存 custom emoji，在预览渲染，发布发送原生短代码/规则；仅生成本地预览缩略图，不自动裁剪、压缩或转码。
- [x] 提供对应 CLI（serve、Profile/Account/Topic 创建与查看、Draft/Variant 编辑与应用更新、媒体、预览、发布、重试、历史、URL 标记），并与 WebUI 复用应用层。
- [x] 提供 ExternalPost 的“仅标记 URL”路径；该操作不产生后台网络请求。

验收标准：用户可在全新本地目录创建 Profile、账号和 Topic，编辑基础稿，生成 Variant，附图并从 WebUI 或 CLI 发布；成功的远端 ID/URL、时间和媒体关联能在重启后查看；失败显示可行动原因且不会丢稿；粘贴 URL 只产生本地关联记录。

必要测试策略：

- [x] Go 单元测试覆盖 Profile/Account 所属约束、linked/source revision/outdated 更新、首次编辑自动 detached、跨 Variant detached 复制、各实体状态和 needs_confirmation 语义、能力/字符/媒体校验、幂等键和 URL 标记不触网。
- [x] Go SQLite 集成测试覆盖迁移、重启后读取，以及媒体与发布历史的关联。
- [x] adapter contract 测试使用假的 Mastodon-compatible HTTP server，覆盖 custom emoji 获取/按实例缓存/预览数据、媒体先传后发、错误归类和规范化结果。
- [x] Vue 单测覆盖编辑/保存、Variant 状态显示、预览警告和 URL 标记表单；CLI 至少有一条应用层端到端测试。

## 节点 2：Misskey/MFM + Bluesky

依赖：节点 1 的领域模型、媒体存储、adapter 接口和 CLI/WebUI 发布流程。目标是在同一 Topic 中新增两种官方 API 平台，而不引入时间线抓取。

- [x] 实现 Misskey 官方 API adapter，支持 note 文本、媒体、可见性、CW、localOnly 和自定义 emoji；预览使用原生规则，发送保留原生短代码。
- [x] 通过真实 v2/v3 SQLite migration 扩展 Misskey/Bluesky 所需字段，旧数据库重启升级且保留数据。
- [x] 保存并预览原始 MFM；当前不做 Markdown/HTML 静默改写，不能表达的格式由发布校验报告。
- [x] 实现 Bluesky 官方 AT Protocol adapter：会话、文本 facet、图片 blob、记录 URI/CID 等结果。
- [x] 为两种 adapter 提供格式、媒体和可见性校验，并让表单按平台限制显示选项。
- [x] 支持用户主动指定的官方单帖导入（能可靠定位且有权限时）；失败不退化为网页抓取，不扫描时间线或最近帖子。

验收标准：用户可为同一 Topic 创建 Misskey MFM 和 Bluesky Variant，预览中看出格式能力，发布后记录远端标识；超出平台限制的版本在提交前阻止或给出明确修正提示；官方 API 单帖导入失败不会退化成后台爬取。

必要测试策略：

- [x] Go adapter contract 测试用 fixture HTTP/AT Protocol 服务覆盖成功、鉴权、限流、媒体限制和可重试性。
- [x] Service 按账号平台分发发布/官方单帖导入；Misskey/Bluesky 无远端幂等能力时最终请求未知进入待确认。
- [x] MFM/纯文本与 emoji 规范化测试覆盖 Misskey 自定义 emoji 的原生短代码保留，保留不会被误发送的原始文本。
- [x] Bluesky facet 字节偏移、图片 blob 和 URI/CID 解析使用固定 Unicode/媒体 fixture。
- [x] Vue 单测覆盖平台选项、MFM/纯文本提示和导入模式选择。

## 节点 3：浏览器桥接 + X/微博/小红书/QQ 空间

依赖：节点 1 的本地 API/媒体与 Publication，节点 2 的 capability/ExternalPost 约定。目标是让用户在自己已登录的浏览器页面完成填充发布和可见内容快照导入；平台页面最终发布始终由用户确认。

- [x] 定义版本化本地桥接协议：配对码/短期 token、请求 ID、`hello`/`fill`/`capture`/`result`/`error` 消息、大小/媒体限制和可撤销配对。
- [x] 实现本地回环桥接端点：校验来源和 token，限制请求体，不把 token 放入 URL/日志，并处理超时后的“待确认”状态。
- [x] 提供用户脚本的通用通信层和四个平台适配器：X、微博、小红书、QQ 空间；读取当前可见 DOM，填充文本/图片，回传用户确认后的 URL/远端 ID。
- [x] 支持从用户主动展开的当前页面导入 ExternalPost 内容快照；无法读取的正文/媒体标为不完整并提示补充。
- [x] 在发布页展示平台选项和人工确认提示；QQ 空间无法可靠自动设置的可见范围与图片顺序仍提示用户检查。
- [x] 文档化脚本安装、配对、撤销和最小权限；没有实现隐藏接口、全量抓取或无人值守点击发布。

验收标准：用户可从本地 Topic 将 Variant 填入四个平台页面，检查后手动发布，再把结果回传为 Publication；也可在已登录且有权查看的帖子页面点击导入，将当前可见文字/媒体快照关联到 Topic；程序不会因 URL、打开 Topic 或重试而后台访问这些平台；桥接超时不会错误地重复发帖。

必要测试策略：

- [x] Go 协议/桥接集成测试覆盖配对生命周期、来源/token 校验、重放、过期和结果幂等；请求大小由回环 handler 限制。
- [x] 用户脚本通信模块用静态 DOM fixture 做四平台填充和不完整内容提示测试；fixture 不包含真实账号数据。
- [x] 四个平台共享安全边界与能力提示；不伪造平台 API，图片顺序失败返回明确提示。
- [x] Vue 单测覆盖桥接平台账号选项、人工确认提示；桥接状态操作通过同一 API 路径。
- [ ] 手工验收记录四个平台的页面选择器/版本差异，但不把真实登录流程写进自动化测试。

## 节点 4：翻译 + 可靠性收尾

依赖：前三节点的 Variant、Media、Publication、凭据和 adapter 边界。目标是让日常使用可恢复、可备份，并能在用户选择后生成多语言版本。

- [x] 实现可替换的 Google Cloud Translation v3、DeepL v2、OpenAI Responses-compatible provider；翻译显式生成新的 detached Variant/Translation 记录，不覆盖原文。
- [x] 配置只保存凭据环境变量名或本地 credential 引用，API/CLI/日志/备份不输出 secret，并提示用户自行保护环境；操作系统 keychain 仍是后续可选增强。
- [x] 分类鉴权、能力校验、媒体上传、限流、网络超时和 provider 协议错误；只对明确安全的 `retryable` 失败开放重试。
- [x] 完成发布幂等、超时待确认、启动重启恢复、v5 事务迁移保护和本地备份/恢复（数据库、媒体、无凭据清单）。
- [x] 补齐 WebUI/CLI 翻译入口、恢复提示、备份/凭据提示和简约界面说明；README 与设计文档已按实际范围更新。

### 后续连接与凭据增强（当前实现）

节点 4 的可靠性边界已完成后，以下小节点补齐单用户本地连接体验；它们不改变前三节点的范围：

- [x] 本地权限受限 credential Store、v6 migration，以及 Account/TranslationConfig 的引用优先、环境变量 fallback。
- [x] 设置页动态平台/provider 说明；Mastodon OAuth granular-scope discovery/旧实例 fallback；Misskey MiAuth；过期、重放和失败清理测试。
- [x] 非官方 `google-gtx` provider（本地 5000 Unicode 字符校验、分段响应、fake-server 测试）；真实 provider/真实账号仍只做手工验收。

验收标准：翻译可追溯且原文不变；失去网络或程序重启后能恢复编辑和发布历史；超时不会盲目重复发布；备份可在另一空目录恢复正文、媒体和记录但不泄露 token；常见错误能告诉用户下一步操作。

必要测试策略：

- [x] Go provider contract 测试使用本地假的翻译服务，覆盖三个 provider、Unicode/标准响应和敏感错误脱敏；超时沿用网络错误分类。
- [x] Go 集成测试覆盖备份恢复、损坏/覆盖拒绝、v5 migration、重启恢复和幂等重试矩阵。
- [x] Vue 单测覆盖翻译生成新版本、错误恢复、备份提示和凭据不可见。
- [ ] 对真实平台只做小范围手工 smoke test；持续集成不放真实 token、账号或远程站点依赖。

## Backlog（不阻塞首版）

- [ ] 更多官方 API 平台或适配器，前提是其权限和服务条款允许。
- [ ] 更丰富的 Markdown/MFM/平台 emoji 编辑辅助和无损转换提示。
- [ ] 媒体裁剪、压缩、替代文本检查和可选本地全文搜索。
- [ ] 可靠的发布计划/定时提醒（仍由用户确认，不做无人值守浏览器发布）。
- [ ] 可选的导出格式、冲突诊断和更完整的诊断日志。

## 明确不做

以下是范围护栏，不是待办事项：

- 多用户、多人协作、组织、RBAC、审批流或共享 Profile。
- 云同步、托管后台、远程开放 API 或默认局域网服务。
- 关注时间线、全站内容、账号历史或后台轮询抓取。
- 统计面板、粉丝数、流量分析或以此为目标的数据采集。
- 绕过登录、私密权限、验证码、付费墙、反爬虫或平台限制。
- 仅凭 URL 在后台解析正文，或代替用户自动点击浏览器平台的最终发布按钮。
