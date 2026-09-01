# Best Post Manager

Best Post Manager 是一个单用户、本地运行的社交平台发帖工作台：在一个话题下维护基础稿、各平台版本和发布记录，再把内容交给合适的平台发布。它面向个人创作者，不是团队协作服务；项目不会规划多人协作、多用户账户、RBAC、云同步或服务端托管。

## 能做什么

- 用 WebUI 或 CLI 管理 Profile、平台账号、话题、基础稿、平台版本和发布记录。
- 在同一话题下关联不同账号和平台的版本，记录哪些目标已经发布。
- 发布图片，并按平台能力处理 Markdown、MFM、emoji 和预览。
- 通过官方 API/SDK 连接 Fediverse（Mastodon-compatible 与 Misskey）和 Bluesky。
- 通过本地浏览器桥接连接 X、微博、小红书和 QQ 空间；浏览器脚本在用户已登录且当前可见的页面中工作。
- 使用 Google Translate、DeepL 或 OpenAI-compatible provider 翻译内容（属于后续节点）。

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

## 本地运行（节点 1）

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

Mastodon-compatible Account 只保存 token 环境变量名，不把 token 写入 SQLite；发布前设置该变量，例如 `export BPM_MASTODON_TOKEN=...`，再将 `BPM_MASTODON_TOKEN` 作为 Account 的 `token_env`。节点 1 使用这一最小方案，后续再接操作系统 keychain。
