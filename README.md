# Best Post Manager

Best Post Manager 是一个单用户、本地运行的社交平台发帖工作台：在一个话题下维护基础稿、各平台版本和发布记录，再把内容交给合适的平台发布。它面向个人创作者，不是团队协作服务；项目不会规划多人协作、多用户账户、RBAC、云同步或服务端托管。

## 能做什么

- 用 WebUI 或 CLI 管理 Profile、平台账号、话题、基础稿、平台版本和发布记录。
- 在同一话题下关联不同账号和平台的版本，记录哪些目标已经发布。
- 发布图片，并按平台能力处理 Markdown、MFM、emoji 和预览。
- 通过官方 API/SDK 连接 Fediverse（Mastodon-compatible 与 Misskey）和 Bluesky。
- 通过本地浏览器桥接连接 X、微博、小红书和 QQ 空间；浏览器脚本在用户已登录且当前可见的页面中工作。
- 使用 Google Cloud Translation v3、DeepL v2、OpenAI Responses-compatible 或非官方 `google-gtx` provider 翻译内容；译文总是生成新的 detached Variant。`google-gtx` 是无凭据的非官方接口，单次最多 5000 Unicode 字符，可能随时失效。

编辑页的平台版本使用搜索切换器，账号很多时只渲染当前选中的版本。基础稿工具栏可针对所选账号直接生成快照并进入发布流程：API 平台直接发布，浏览器桥接平台会明确显示为“填入”，仍需在平台页确认。当前版本编辑器旁提供按实例加载的 emoji（搜索、分批和点击插入）及翻译工具；翻译完成后自动切换到新的 detached 版本。

## 站外帖子关联

URL 不是可靠的抓取入口，Best Post Manager 不会因用户粘贴 URL 而在后台请求或爬取站点。站外帖子有两种明确模式：

1. **仅标记 URL**：保存平台、账号、URL/远端 ID、时间和备注，用来表示“已经在这里发布”，不保存正文。
2. **浏览器辅助导入快照**：用户在已登录浏览器中打开自己有权查看的帖子，由用户脚本读取页面当前可见 DOM，并把正文、媒体信息和 URL 回传到本地程序。私密内容只依赖用户当前权限，不绕过登录、权限或反爬保护。

浏览器桥接默认只填充待发内容。目前只有纯文本 X Variant 提供一次性的“填入并自动发布”：确认后只点击已知、可见且启用的发布控件。微博、小红书和 QQ 空间的最终发布控件尚未完成真实页面验收，因此只填入；带媒体的 X 请求也因无法可靠判断上传完成而退回人工发布。

## 首版范围

首版优先完成本地数据存储、话题到发布记录的端到端闭环，以及 Mastodon-compatible 的发布。随后加入 Misskey/MFM、Bluesky，再加入统一浏览器桥接和 X/微博/小红书/QQ 空间适配器，最后补齐翻译、重试、幂等和备份可靠性。

首版明确不做：

- 关注时间线、全站内容或账号历史的后台抓取；
- 多人协作、组织、权限审批、云同步和远程服务；
- 统计面板、粉丝数和流量分析；
- 绕过登录、私密范围、验证码、反爬机制或平台限制；
- 未经当前请求明确确认就替用户点击浏览器平台的最终发布。

## 运行形态

Go 单体程序承载本地 API、应用层和嵌入的 Vue SPA；SQLite 保存结构化数据，本地目录保存图片。CLI 与 WebUI 复用同一应用层，避免两套业务规则。具体边界和阶段性验收见：

- [技术方案](docs/design.md)
- [开发 TODO](TODO.md)

项目仍在设计和快速实现阶段，平台能力以各适配器的实际支持范围为准。

## 本地运行

需要 Go 1.23+。默认数据目录位于操作系统用户配置目录下的 `best-post-manager`，也可以用 `BPM_DATA_DIR` 指定一个权限受限的本地目录：

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm serve
```

然后打开 `http://127.0.0.1:8787/`。CLI 与服务使用同一数据目录，例如：

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm profile add personal
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm topic add <profile-id> "一个话题"
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm history
```

### 构建单二进制

单二进制会嵌入已经构建的 Vue WebUI，并使用纯 Go SQLite 驱动，不需要随包复制 `dist/`、Node.js 或动态 SQLite 库。安装依赖并构建当前系统版本：

```sh
mkdir -p build
npm --prefix frontend ci
npm --prefix frontend run build
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o build/bpm ./cmd/bpm
```

如果系统提供 `make`，以上命令等价于 `make build`。产物为 `build/bpm`，可以单独复制到其他目录运行：

```sh
BPM_DATA_DIR="$PWD/.data" ./build/bpm serve
```

Go 支持的平台可以通过 `GOOS`、`GOARCH` 交叉编译；Windows 目标需使用 `.exe` 文件名，例如：

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o build/bpm.exe ./cmd/bpm
```

Mastodon-compatible Account 手动配置可以把 token 保存到数据目录下权限受限的 `credentials.json`，也可以只保存环境变量名而不保存 token；环境变量模式下，发布前设置该变量，例如 `export BPM_MASTODON_TOKEN=...`，再将 `BPM_MASTODON_TOKEN` 作为 Account 的 `token_env`。设置页也提供 Mastodon OAuth 和 Misskey MiAuth 自动连接：授权回调只在回环服务上接收，access token 同样写入 `credentials.json`，SQLite/API/备份只保存不透明引用。该文件不应加入版本库或手工备份。

OAuth 连接要求通过回环地址打开 WebUI。Mastodon 会先发现实例是否支持 `write:statuses`/`write:media`，旧实例回退到兼容的 `write` scope；Misskey 请求 `write:notes,write:drive`。OAuth state、MiAuth session 和 client secret 只保存在进程内短期状态，失败或过期不会创建半成账号。若不使用自动连接，可直接保存 token，也可按下文使用环境变量。

手动添加账号支持直接填写 access token/app password（推荐，保存在数据目录权限受限的 `credentials.json`）或只保存环境变量名；secret 不进入 SQLite、API 响应或日志。已有账号可以编辑名称、实例/PDS、远端身份和凭据，凭据留空表示保持当前值。Bluesky 的 `remote_id` 是 handle/identifier，PDS 为空时默认规范化为 `https://bsky.social`，也接受裸域名和尾斜杠。Bluesky 会话 JWT 仅在内存请求期间使用，不写入数据库。示例：

Bluesky 手动账号添加后会自动执行一次只读连接测试，也可以在设置中的账号卡片展开“Bluesky 连接诊断”并重新测试。测试只调用 `com.atproto.server.createSession`，不会发帖；界面会区分配置、网络、鉴权和协议阶段，并给出下一步建议。返回内容和 bpm 日志都不会包含 app password、JWT 或完整远端响应。

Mastodon-compatible 手动账号或 OAuth 账号也可以在账号列表中点击“测试连接”。诊断依次检查实例、token、`verify_credentials` 身份端点，并使用 `OPTIONS` 探测标准发帖/媒体端点；探测结果只是实例兼容性信号，不是对 OAuth scope 的绝对证明，也不会发送帖子或媒体。发布历史会把失败区分为本地校验、身份认证、实例连通、媒体上传和正文提交阶段。

```sh
export MISSKEY_TOKEN=...
export BSKY_APP_PASSWORD=...
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> misskey misskey-main https://misskey.example MISSKEY_TOKEN
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> bluesky bluesky-main https://bsky.social BSKY_APP_PASSWORD name.example
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm import <topic-id> <account-id> "https://bsky.app/profile/name.example/post/abc"
```

Misskey 保留 MFM 原文并使用官方 API；Bluesky 生成链接/提及 facets 并记录 URI/CID。导入只调用用户主动指定的单帖官方端点，不扫描时间线，也不会因仅标记 URL 而联网。

### 节点 3 浏览器桥接

用户脚本可在 Topic 编辑页的“浏览器填入”中安装/更新。每个平台首次生成并输入一次 code；本地服务随后签发 90 天 bearer。服务端只把 token 哈希和生命周期存入 SQLite，用户脚本按平台把明文 token 保存在油猴隔离存储，不使用站点 localStorage。页面刷新和程序重启后均可恢复连接，也可从 WebUI 撤销或在脚本中断开。

运行时默认关闭 HTTP 路由访问日志，避免本地开发时被大量轮询刷屏；启动日志和 Bluesky/Mastodon 诊断阶段日志仍保留。需要临时排查路由时使用 `BPM_HTTP_LOG=1 bpm serve` 或 `bpm serve --http-log`，日志只记录 method、path、status 和耗时，不记录 query、body 或凭据。

实际使用顺序：安装脚本，选择平台并生成 code（WebUI 会尝试自动复制），在已登录的平台页输入 code 并连接；以后直接在 Variant 选择“仅填入”，X 纯文本还可对当前一次请求选择“填入并自动发布”。重新安装脚本、清除油猴存储或主动重置配对时才需要新 code。页面 URL 由脚本回传，不需要手填；脚本的更新地址会自动跟随当前回环端口。

填入请求会先创建 `needs_confirmation` 发布记录。默认模式下请在平台页人工检查并发布；自动发布模式仍需核对结果。之后在脚本中填入帖子永久链接并回传；如果脚本窗口已经丢失，也可在“发布记录”中选择“确认已发布”或“确认未发布”来结束待确认状态。超时或脚本错误不会自动重试。捕获请求也必须由 WebUI 主动创建，脚本只回传当前页面可见正文；折叠或无法读取的内容会标记为不完整。当前 X 的纯文本填入/发布选择器和其他三个平台的填入选择器仍需用真实登录页面持续验收；页面结构变化时脚本会停止并要求人工处理。

为了保持发布记录可追溯，一个 Variant 只要有过发布尝试就会变为只读；要修改并再发时使用“复制为新版本”。已参与历史发布的图片不能删除，而发布后新添加的通用图片不会被旧记录锁定。

X、微博、小红书和 QQ 空间账号只用于关联 Profile/Variant，`base_url` 与 `token_env` 可留空；页面权限完全来自浏览器当前登录会话。

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> qzone qzone-main
```

### 节点 4：翻译、恢复与备份

翻译配置只保存 provider、地址、模型/项目，以及环境变量名或不透明本地凭据引用，不把 token、API key 或会话写入 SQLite。凭据示例：

```sh
export OPENAI_TRANSLATION_KEY=...
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm translation-config add openai OPENAI_TRANSLATION_KEY https://api.openai.com gpt-4o-mini
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm translate <source-variant> <target-account> zh <config-id> [source-language]
```

也支持 `google`（凭据环境变量名 + base URL + project id）和 `deepl`（key 环境变量名 + base URL）。`google-gtx` 可用 `bpm translation-config add google-gtx [base-url]` 创建，不需要 key，但属于非官方接口且限制 5000 Unicode 字符。CLI 与 WebUI 都会明确显示译文是新的 detached 版本；provider 鉴权、限流、网络和协议错误不会把原文改掉。凭据文件采用 0700 目录、0600 文件和原子替换；环境变量仍是可选 fallback。程序不会把环境变量值或凭据文件内容写入数据库、日志、API 响应或普通备份。

离线备份包含 SQLite 一致快照、本地图片和版本/校验清单，不包含环境变量值：

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm backup create "$PWD/bpm-backup.zip"
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm backup restore "$PWD/bpm-backup.zip" "$PWD/empty-restore-dir"
```

恢复目标必须不存在或为空，程序会先校验清单、SHA-256、SQLite schema 和路径安全，再落盘，拒绝覆盖已有数据。若程序在最终发布期间重启，`publishing` 会变为 `needs_confirmation`；请先去平台确认，不要盲目重试。只有明确标记为 `retryable` 的失败才开放 retry。
