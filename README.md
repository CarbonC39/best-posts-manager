# Best Post Manager

Best Post Manager 是一个单用户、本地运行的社交平台发帖工作台：在一个话题下维护基础稿、各平台版本和发布记录，再把内容交给合适的平台发布。它面向个人创作者，不是团队协作服务；项目不会规划多人协作、多用户账户、RBAC、云同步或服务端托管。

## 能做什么

- 用 WebUI 或 CLI 管理 Profile、平台账号、话题、基础稿、平台版本和发布记录。
- 在同一话题下关联不同账号和平台的版本，记录哪些目标已经发布。
- 发布图片，并按平台能力处理 Markdown、MFM、emoji 和预览。
- 通过官方 API/SDK 连接 Fediverse（Mastodon-compatible 与 Misskey）和 Bluesky。
- 通过本地浏览器桥接连接 X、微博、小红书和 QQ 空间；浏览器脚本在用户已登录且当前可见的页面中工作。
- 使用 Google Cloud Translation v3、DeepL v2 或 OpenAI Responses-compatible provider 翻译内容；译文总是生成新的 detached Variant。

## 站外帖子关联

URL 不是可靠的抓取入口，Best Post Manager 不会因用户粘贴 URL 而在后台请求或爬取站点。站外帖子有两种明确模式：

1. **仅标记 URL**：保存平台、账号、URL/远端 ID、时间和备注，用来表示“已经在这里发布”，不保存正文。
2. **浏览器辅助导入快照**：用户在已登录浏览器中打开自己有权查看的帖子，由用户脚本读取页面当前可见 DOM，并把正文、媒体信息和 URL 回传到本地程序。私密内容只依赖用户当前权限，不绕过登录、权限或反爬保护。

浏览器桥接默认只填充待发内容，最终发布由用户在平台页面人工确认。若平台页面无法提供图片或完整展开的正文，程序应提示用户补充，而不是偷偷重新抓取。

## 首版范围

首版优先完成本地数据存储、话题到发布记录的端到端闭环，以及 Mastodon-compatible 的发布。随后加入 Misskey/MFM、Bluesky，再加入统一浏览器桥接和 X/微博/小红书/QQ 空间适配器，最后补齐翻译、重试、幂等和备份可靠性。

首版明确不做：

- 关注时间线、全站内容或账号历史的后台抓取；
- 多人协作、组织、权限审批、云同步和远程服务；
- 统计面板、粉丝数和流量分析；
- 绕过登录、私密范围、验证码、反爬机制或平台限制；
- 自动替用户在浏览器桥接平台点击最终发布。

## 运行形态

Go 单体程序承载本地 API、应用层和嵌入的 Vue SPA；SQLite 保存结构化数据，本地目录保存图片。CLI 与 WebUI 复用同一应用层，避免两套业务规则。具体边界和阶段性验收见：

- [技术方案](docs/design.md)
- [开发 TODO](TODO.md)

项目仍在设计和快速实现阶段，平台能力以各适配器的实际支持范围为准。

## 本地运行（节点 1–4）

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

Mastodon-compatible Account 只保存 token 环境变量名，不把 token 写入 SQLite；发布前设置该变量，例如 `export BPM_MASTODON_TOKEN=...`，再将 `BPM_MASTODON_TOKEN` 作为 Account 的 `token_env`。节点 1 使用这一最小方案，后续再接操作系统 keychain。

节点 2 的账号仍只保存环境变量名：Misskey 的 `token_env` 指向 access token，`base_url` 是实例地址；Bluesky 的 `remote_id` 是 handle/identifier，`base_url` 是 PDS 地址，`token_env` 指向 app password。Bluesky 会话 JWT 仅在内存请求期间使用，不写入数据库。示例：

```sh
export MISSKEY_TOKEN=...
export BSKY_APP_PASSWORD=...
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> misskey misskey-main https://misskey.example MISSKEY_TOKEN
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> bluesky bluesky-main https://bsky.social BSKY_APP_PASSWORD name.example
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm import <topic-id> <account-id> "https://bsky.app/profile/name.example/post/abc"
```

Misskey 保留 MFM 原文并使用官方 API；Bluesky 生成链接/提及 facets 并记录 URI/CID。导入只调用用户主动指定的单帖官方端点，不扫描时间线，也不会因仅标记 URL 而联网。

### 节点 3 浏览器桥接

用户脚本位于 [`frontend/userscript/bpm-bridge.user.js`](frontend/userscript/bpm-bridge.user.js)，可在 Tampermonkey/Violentmonkey 中安装。启动本地服务后，在编辑器的“浏览器桥接 v1”生成对应平台配对，只把一次性 code 填入脚本；hello 成功后本地服务才签发八小时 bearer，code 立即失效，token 只以哈希保存在 Go 进程并只明文保留在当前平台页面的脚本内存中，可从 WebUI 撤销。脚本只匹配 X、微博、小红书和 QQ 空间的当前页面，读取可见 DOM、填充文本/图片并回传结果；它绝不会点击最终发布按钮。

填入请求会先创建 `needs_confirmation` 发布记录。请在平台页人工检查并发布，再点击脚本的“我已人工发布，回传 URL”；超时或脚本错误不会自动重试。捕获请求也必须由 WebUI 主动创建，脚本只回传当前页面可见正文；折叠或无法读取的内容会标记为不完整。当前四个平台的页面选择器和真实登录流程尚未验收，页面结构变化时请人工检查并补充。

X、微博、小红书和 QQ 空间账号只用于关联 Profile/Variant，`base_url` 与 `token_env` 可留空；页面权限完全来自浏览器当前登录会话。

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> qzone qzone-main
```

### 节点 4：翻译、恢复与备份

翻译配置只保存 provider、地址、模型/项目和环境变量名，不保存 token、API key 或会话。凭据示例：

```sh
export OPENAI_TRANSLATION_KEY=...
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm translation-config add openai OPENAI_TRANSLATION_KEY https://api.openai.com gpt-4o-mini
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm translate <source-variant> <target-account> zh <config-id> [source-language]
```

也支持 `google`（凭据环境变量名 + base URL + project id）和 `deepl`（key 环境变量名 + base URL）。CLI 与 WebUI 都会明确显示译文是新的 detached 版本；provider 鉴权、限流、网络和协议错误不会把原文改掉。当前凭据最小方案是环境变量引用，程序不会把环境变量值写入数据库、日志、API 响应或备份；操作系统 keychain 尚未作为默认依赖引入。

离线备份包含 SQLite 一致快照、本地图片和版本/校验清单，不包含环境变量值：

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm backup create "$PWD/bpm-backup.zip"
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm backup restore "$PWD/bpm-backup.zip" "$PWD/empty-restore-dir"
```

恢复目标必须不存在或为空，程序会先校验清单、SHA-256、SQLite schema 和路径安全，再落盘，拒绝覆盖已有数据。若程序在最终发布期间重启，`publishing` 会变为 `needs_confirmation`；请先去平台确认，不要盲目重试。只有明确标记为 `retryable` 的失败才开放 retry。
