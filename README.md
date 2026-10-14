# Best Posts Manager

[English](README.md) | [简体中文](README.zh-CN.md)

Best Posts Manager is a local, single-user tool for managing social media publishing. It keeps a base draft, platform-specific versions, and publication records together under each topic, and publishes only after confirmation. It is designed for individual creators and does not include collaboration features.

## Features

- Manage profiles, platform accounts, topics, base drafts, platform versions, and publication records through the Web UI or CLI.
- Keep versions for different accounts and platforms under the same topic, and track which targets have been published.
- Publish images, with Markdown, MFM, emoji, and previews handled according to each platform's capabilities.
- Connect to the Fediverse (Mastodon-compatible services and Misskey) and Bluesky through official APIs and SDKs.
- Connect to X, Weibo, Xiaohongshu, and Qzone through a local browser bridge. The userscript works on pages where the user is signed in and that are currently visible.
- Translate content with Google Cloud Translation v3, DeepL v2, OpenAI Responses-compatible providers, or the unofficial `google-gtx` provider. Translations always create a new detached Variant. `google-gtx` is an unofficial, unauthenticated interface, supports up to 5,000 Unicode characters per request, and may stop working at any time.

The editor uses a searchable switcher for platform versions and renders only the selected version when there are many accounts. The base-draft toolbar can create a snapshot for a selected account and start the publishing flow: API platforms publish directly, while browser-bridge platforms are clearly marked as “Fill” and require confirmation on the platform page. The current version editor also provides instance-specific emoji search, pagination, and insertion, plus translation tools. After translation, the editor switches to the new detached version.

## Linking external posts

Best Posts Manager does not request or crawl a site just because you paste a URL. External posts can be linked in two ways:

1. **Record a URL only:** Save the platform, account, URL or remote ID, time, and notes to record that a post was published there. The post body is not saved.
2. **Import a snapshot through the browser:** Open a post you are allowed to view in a signed-in browser. The userscript reads the currently visible DOM and sends the text, media information, and URL back to the local app.

> The browser bridge fills in content by default. Plain-text X and Weibo Variants that have been tested support a one-time “Fill and publish” action. After confirmation, the script clicks only a unique, known, visible, enabled publish control. Xiaohongshu and Qzone remain fill-only. Bridge requests with media fall back to manual publishing because upload completion cannot be detected reliably.

## Architecture

A Go application provides the local API, application layer, and embedded Vue SPA. SQLite stores structured data, and images are stored in a local directory. The CLI and Web UI share the same application layer and business rules. See the [technical design](docs/design.md) and [development TODO](TODO.md) for system boundaries and milestone acceptance criteria.

The project is still in active design and development. Platform support depends on the capabilities implemented by each adapter.

## Run locally

Go 1.23 or later is required. By default, the data directory is `best-post-manager` under the operating system's user configuration directory. You can also set `BPM_DATA_DIR` to use a restricted local directory:

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm serve
```

Then open `http://127.0.0.1:8787/`. The CLI and server use the same data directory. For example:

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm profile add personal
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm topic add <profile-id> "A topic"
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm history
```

### Build a single binary

The binary embeds the built Vue Web UI and uses a pure-Go SQLite driver. You do not need to ship `dist/`, Node.js, or a dynamically linked SQLite library with it. Install dependencies and build for the current system:

```sh
mkdir -p build
npm --prefix frontend ci
npm --prefix frontend run build
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o build/bpm ./cmd/bpm
```

If `make` is available, these commands are equivalent to `make build`. The output is `build/bpm`, which can be copied and run from another directory:

```sh
BPM_DATA_DIR="$PWD/.data" ./build/bpm serve
```

You can cross-compile for Go-supported platforms with `GOOS` and `GOARCH`. Use an `.exe` filename for Windows, for example:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o build/bpm.exe ./cmd/bpm
```

For a manually configured Mastodon-compatible account, you can store the token in the permission-restricted `credentials.json` file under the data directory, or store only the environment variable name. In environment-variable mode, set the variable before publishing, for example with `export BPM_MASTODON_TOKEN=...`, and use `BPM_MASTODON_TOKEN` as the account's `token_env`. The Settings page also supports Mastodon OAuth and Misskey MiAuth. Authorization callbacks are accepted only by the loopback service, and access tokens are stored in `credentials.json`; SQLite, API responses, and backups store only opaque references. Do not add this file to version control or back it up manually.

OAuth connections require opening the Web UI through a loopback address. Mastodon first checks whether the instance supports `write:statuses` and `write:media` scopes, and falls back to the compatible `write` scope for older instances. Misskey requests `write:notes,write:drive`. OAuth state, MiAuth sessions, and client secrets are held only in short-lived process memory. Failed or expired flows do not create incomplete accounts. If you do not use automatic connection, you can enter a token directly or use an environment variable as described above.

When adding an account manually, you can enter an access token or app password (recommended; stored in the permission-restricted `credentials.json`) or save only the environment variable name. Secrets are not stored in SQLite or returned by the API or logs. Existing accounts can be edited to change their name, instance or PDS, remote identity, and credentials. Leaving the credential field blank keeps the existing credential. For Bluesky, `remote_id` is the handle or identifier. If the PDS is blank, it defaults to `https://bsky.social`; bare domain names and trailing slashes are also accepted. Bluesky session JWTs are used only in memory during a request and are not stored in the database.

After a manually added Bluesky account is saved, the app automatically runs a read-only connection test. You can also expand “Bluesky connection diagnostics” on the account card in Settings and run it again. The test calls only `com.atproto.server.createSession`. The UI identifies whether an issue occurred during configuration, networking, authentication, or protocol handling, and suggests next steps. Neither the response nor bpm logs include the app password, JWT, or full remote response.

Manually configured or OAuth-connected Mastodon-compatible accounts can also be tested from the account list. Diagnostics check the instance and token, call the `verify_credentials` identity endpoint, and probe standard posting and media endpoints with `OPTIONS`. Publication history classifies failures by stage: local validation, authentication, instance connectivity, media upload, or post submission.

```sh
export MISSKEY_TOKEN=...
export BSKY_APP_PASSWORD=...
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> misskey misskey-main https://misskey.example MISSKEY_TOKEN
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> bluesky bluesky-main https://bsky.social BSKY_APP_PASSWORD name.example
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm import <topic-id> <account-id> "https://bsky.app/profile/name.example/post/abc"
```

Misskey preserves the original MFM and uses the official API. Bluesky generates link and mention facets and records the URI and CID. Import calls only the official endpoint for the specific post requested by the user; it does not scan timelines, and recording a URL alone never makes a network request.

### Browser bridge

Install or update the userscript from “Browser fill” on the Topic editor page. Generate and enter a code once for each platform. The local service then issues a 90-day bearer token. The server stores only the token hash and lifecycle data in SQLite; the userscript stores the token itself in the userscript manager's isolated storage for that platform, never in the site's `localStorage`. The connection survives page refreshes and app restarts. You can revoke it from the Web UI or disconnect in the script.

HTTP route access logs are disabled by default to avoid noise from frequent polling during local development. Startup and Bluesky/Mastodon diagnostic stage logs remain enabled. To troubleshoot routes temporarily, set `BPM_HTTP_LOG=1` or pass `bpm serve --http-log`. Logs include only the method, path, status, and duration; they do not include query strings, request bodies, or credentials.

The Web UI's content, topic editor, history, and settings pages use `/topics`, `/topics/:id`, `/history`, and `/settings`. These URLs can be refreshed, copied, or opened in a new tab. Terminal publication records and platform versions can be removed locally without deleting remote posts. When deactivating an account, you can separately confirm permanent removal of its local versions, media, publication records, and associations.

Typical use: install the script, choose a platform, and generate a code (the Web UI will try to copy it automatically); then enter the code and connect on the platform page where you are signed in. Later, choose “Fill only” on a Variant. For plain-text X or Weibo, you can also choose “Fill and publish” for that one request. You need a new code only if you reinstall the script, clear its userscript storage, or reset pairing. The script returns the page URL, and its update URL follows the current loopback port automatically.

A fill request first creates a `needs_confirmation` publication record. In the default mode, review and publish the content on the platform page. Even in automatic publishing mode, verify the result. Then enter the post's permanent link in the script and send it back. If the script window is gone, you can resolve the pending state from “Publication history” by choosing “Confirm published” or “Confirm not published.” Timeouts and script errors are never retried automatically. Capture requests must also be started from the Web UI; the script returns only text visible on the current page. Collapsed or unreadable content is marked incomplete. The plain-text posting selectors for X and Weibo and the fill selectors for all four platforms still need ongoing checks on real signed-in pages. If a page changes, the script stops and asks for manual handling.

To preserve publication history, a Variant becomes read-only once it has been used in a publishing attempt. Use “Copy as new version” to edit and publish it again. Images used in publication history cannot be deleted, while general images added after publishing are not locked by older records.

X, Weibo, Xiaohongshu, and Qzone accounts are used only to associate a Profile with a Variant. Their `base_url` and `token_env` can be left blank; page access comes entirely from the browser's current signed-in session.

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm account add <profile-id> qzone qzone-main
```

### Translation, recovery, and backups

Translation configuration stores only the provider, endpoint, model or project, and an environment variable name or opaque local credential reference. It does not store tokens, API keys, or sessions in SQLite. For example:

```sh
export OPENAI_TRANSLATION_KEY=...
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm translation-config add openai OPENAI_TRANSLATION_KEY https://api.openai.com gpt-4o-mini
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm translate <source-variant> <target-account> zh <config-id> [source-language]
```

The app also supports `google` (credential environment variable name, base URL, and project ID) and `deepl` (key environment variable name and base URL). Create a `google-gtx` configuration with `bpm translation-config add google-gtx [base-url]`. It requires no key, but is unofficial and limited to 5,000 Unicode characters. The CLI and Web UI clearly show that a translation creates a new detached version. The credentials file uses a 0700 directory, a 0600 file, and atomic replacement. Environment variables remain available as a fallback.

Offline backups contain a consistent SQLite snapshot, local images, and a version and checksum manifest. They do not contain environment variable values:

```sh
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm backup create "$PWD/bpm-backup.zip"
BPM_DATA_DIR="$PWD/.data" go run ./cmd/bpm backup restore "$PWD/bpm-backup.zip" "$PWD/empty-restore-dir"
```

The restore target must not exist or must be empty. Before writing files, the app validates the manifest, SHA-256 checksums, SQLite schema, and path safety, and refuses to overwrite existing data. If the app restarts during final publication, `publishing` changes to `needs_confirmation`. Check the platform before retrying. Retry is available only for failures explicitly marked `retryable`.
