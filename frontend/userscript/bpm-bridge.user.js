// ==UserScript==
// @name         Best Post Manager local bridge
// @namespace    best-post-manager
// @version      1.3.0
// @description  Fill visible editors, optionally publish on explicit request, and capture visible posts.
// @match        https://x.com/*
// @match        https://*.x.com/*
// @match        https://twitter.com/*
// @match        https://*.twitter.com/*
// @match        https://weibo.com/*
// @match        https://*.weibo.com/*
// @match        https://xiaohongshu.com/*
// @match        https://*.xiaohongshu.com/*
// @match        https://qzone.qq.com/*
// @match        https://*.qzone.qq.com/*
// @grant        GM_xmlhttpRequest
// @grant        GM_getValue
// @grant        GM_setValue
// @connect      127.0.0.1
// @connect      localhost
// @downloadURL  __BPM_ORIGIN__/userscript/bpm-bridge.user.js
// @updateURL    __BPM_ORIGIN__/userscript/bpm-bridge.user.js
// @noframes
// ==/UserScript==
(function() {
  "use strict";
  const adapters = {
    x: { selectors: ['[data-testid="tweetTextarea_0"]', 'textarea[aria-label*="Post"]', "textarea", '[contenteditable="true"]'], captureSelectors: ['article [data-testid="tweetText"]', "article"], mediaSelectors: ['input[type="file"]'], publishSelectors: ['[data-testid="tweetButtonInline"]', '[data-testid="tweetButton"]'] },
    weibo: { selectors: ['textarea[placeholder*="分享"]', "textarea", '[contenteditable="true"]'], captureSelectors: [".WB_text", '[node-type="feed_list_content"]'], mediaSelectors: ['input[type="file"]'], publishSelectors: ['button[node-type="submit"]', '[node-type="submit"]'] },
    xiaohongshu: { selectors: ["textarea", '[contenteditable="true"]'], captureSelectors: [".note-content", '[class*="note-content"]'], mediaSelectors: ['input[type="file"]'], publishSelectors: ['button[class*="publish"]', '[role="button"][class*="publish"]'] },
    qzone: { selectors: ["textarea", '[contenteditable="true"]'], captureSelectors: [".feed-content", '[class*="content"]'], mediaSelectors: ['input[type="file"]'], publishSelectors: ['button[node-type="submit"]', '[role="button"][aria-label*="发表"]'] }
  };
  function adapter(platform2) {
    const value = adapters[platform2];
    if (!value) throw new Error(`unsupported bridge platform: ${platform2}`);
    return value;
  }
  function composer(platform2, root = document) {
    for (const selector of adapter(platform2).selectors) {
      const found = root.querySelector(selector);
      if (found && !found.hidden && found.getAttribute("aria-hidden") !== "true" && found.style.display !== "none") return found;
    }
    throw new Error("当前页面未找到可见编辑框，请展开编辑器后重试");
  }
  function setText(element2, text) {
    var _a;
    if (element2 instanceof HTMLTextAreaElement || element2 instanceof HTMLInputElement) {
      const prototype = element2 instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
      const setter = (_a = Object.getOwnPropertyDescriptor(prototype, "value")) == null ? void 0 : _a.set;
      setter == null ? void 0 : setter.call(element2, text);
    } else {
      element2.textContent = text;
    }
    element2.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText", data: text }));
    element2.dispatchEvent(new Event("change", { bubbles: true }));
  }
  function fillVisible(platform2, text) {
    const element2 = composer(platform2);
    setText(element2, text);
    const mediaInput = document.querySelector(adapter(platform2).mediaSelectors[0]);
    return mediaInput ? { mediaInput } : {};
  }
  function clickKnownPublish(platform2) {
    for (const selector of adapter(platform2).publishSelectors) {
      const candidates = document.querySelectorAll(selector);
      for (const element2 of candidates) {
        if (element2.hidden || element2.getAttribute("aria-hidden") === "true" || element2.getAttribute("aria-disabled") === "true" || element2.disabled) continue;
        const style = getComputedStyle(element2);
        if (style.display === "none" || style.visibility === "hidden" || style.opacity === "0") continue;
        element2.click();
        return null;
      }
    }
    return "当前页面未找到明确的可见发布按钮，未自动点击；请人工发布";
  }
  async function fillVisibleWithMedia(platform2, request, fetcher = fetch) {
    var _a;
    const { mediaInput } = fillVisible(platform2, request.text || "");
    const failures = [];
    if (!((_a = request.media) == null ? void 0 : _a.length)) return failures;
    if (!mediaInput) return request.media.map((media) => `${media.filename}: 页面没有图片输入框`);
    const files = [];
    for (const media of request.media) {
      if (!media.url) {
        failures.push(`${media.filename}: 本地媒体地址缺失`);
        continue;
      }
      try {
        const response = await fetcher(media.url);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const blob = await response.blob();
        files.push(new File([blob], media.filename, { type: media.mime || blob.type }));
      } catch (error) {
        failures.push(`${media.filename}: ${error instanceof Error ? error.message : String(error)}`);
      }
    }
    if (files.length) {
      try {
        const transfer = new DataTransfer();
        files.forEach((file) => transfer.items.add(file));
        mediaInput.files = transfer.files;
        mediaInput.dispatchEvent(new Event("input", { bubbles: true }));
        mediaInput.dispatchEvent(new Event("change", { bubbles: true }));
      } catch (error) {
        failures.push(`图片无法填入当前页面：${error instanceof Error ? error.message : String(error)}`);
      }
    }
    return failures;
  }
  function captureVisible(platform2, pageURL = location.href, root = document) {
    let element2 = null;
    for (const selector of adapter(platform2).captureSelectors) {
      element2 = root.querySelector(selector);
      if (element2) break;
    }
    if (!element2) {
      return { text: "", url: pageURL, media: [], incomplete: true, incomplete_reason: "当前页面没有可见正文" };
    }
    const text = (element2 instanceof HTMLTextAreaElement ? element2.value : element2.textContent || "").trim();
    const container = element2.closest("article,.WB_feed_detail,.note-container,.feed-item") || element2.parentElement || element2;
    const media = Array.from(container.querySelectorAll("img[src]")).slice(0, 16).map((image, index) => {
      const raw = image.currentSrc || image.src;
      let filename = `image-${index + 1}`;
      try {
        filename = new URL(raw, pageURL).pathname.split("/").filter(Boolean).pop() || filename;
      } catch {
      }
      return { filename, mime: "", url: raw, alt_text: image.alt || "" };
    });
    const incomplete = !text;
    return { text, url: pageURL, media, incomplete, incomplete_reason: incomplete ? "当前页面没有可见正文" : void 0 };
  }
  const host = location.hostname.toLowerCase();
  const platform = host === "x.com" || host.endsWith(".x.com") || host === "twitter.com" || host.endsWith(".twitter.com") ? "x" : host === "weibo.com" || host.endsWith(".weibo.com") ? "weibo" : host === "xiaohongshu.com" || host.endsWith(".xiaohongshu.com") ? "xiaohongshu" : "qzone";
  const configKey = `bpm-bridge-config-v1-${platform}`;
  const saved = GM_getValue(configKey, {});
  const config = { base: saved.base || "http://127.0.0.1:8787", token: "", pairing: "" };
  if (saved.token && saved.pairing) {
    config.token = saved.token;
    config.pairing = saved.pairing;
  }
  let current = null;
  let polling = false;
  function gmRequest(options) {
    return new Promise((resolve, reject) => GM_xmlhttpRequest({ ...options, onload: resolve, onerror: reject, ontimeout: reject }));
  }
  function headers() {
    return { Authorization: `Bearer ${config.token}`, "X-BPM-Page-URL": location.href, "X-BPM-Pairing-ID": config.pairing, "X-BPM-Platform": platform, "Content-Type": "application/json" };
  }
  async function call(path, init = {}) {
    const response = await gmRequest({ method: init.method || "GET", url: config.base.replace(/\/$/, "") + path, headers: headers(), data: init.body, timeout: 1e4 });
    let body;
    try {
      body = JSON.parse(response.responseText);
    } catch {
      throw new Error(`本地桥接返回了无效响应（HTTP ${response.status}）`);
    }
    if (response.status < 200 || response.status >= 300) throw new Error(body.error || `HTTP ${response.status}`);
    return body;
  }
  async function localMediaFetcher(path) {
    const response = await gmRequest({ method: "GET", url: config.base.replace(/\/$/, "") + path, responseType: "blob", timeout: 3e4 });
    return { ok: response.status >= 200 && response.status < 300, status: response.status, blob: async () => response.response };
  }
  const panel = document.createElement("div");
  panel.style.cssText = "position:fixed;z-index:2147483647;right:12px;bottom:12px;background:#fff;color:#111;border:1px solid #888;padding:10px;font:13px sans-serif;width:280px;box-shadow:0 2px 8px #0003";
  panel.innerHTML = '<style>div{line-height:1.45}input{box-sizing:border-box;width:100%;margin:5px 0;padding:6px;border:1px solid #ccc;border-radius:4px}button{margin:5px 5px 5px 0;padding:5px 8px;border:1px solid #ccc;border-radius:4px;background:#f7f7f5;cursor:pointer}[data-bpm=status]{margin:6px 0;color:#4b6f55;overflow-wrap:anywhere}[data-bpm=request]{margin-top:6px;padding-top:6px;border-top:1px solid #eee}</style><b>Best Post Manager bridge v1</b><button data-bpm="minimize">缩小</button><div data-bpm="platform"></div><div data-bpm="status">未连接</div><input data-bpm="base" placeholder="本地服务地址"><input data-bpm="code" placeholder="一次性 pairing code"><button data-bpm="connect">连接</button><button data-bpm="disconnect">断开并清除</button><div data-bpm="request"></div>';
  const panelHost = document.createElement("div");
  panelHost.attachShadow({ mode: "closed" }).appendChild(panel);
  document.body.appendChild(panelHost);
  const element = (name) => panel.querySelector(`[data-bpm="${name}"]`);
  element("platform").textContent = `当前平台：${platform}`;
  element("base").value = config.base;
  const status = (text) => {
    element("status").textContent = text;
  };
  const requestBox = element("request");
  let minimized = GM_getValue(`${configKey}-minimized`, false);
  function setMinimized(value) {
    minimized = value;
    GM_setValue(`${configKey}-minimized`, value);
    panel.style.width = value ? "46px" : "280px";
    panel.style.height = value ? "46px" : "auto";
    panel.style.padding = value ? "0" : "10px";
    for (const name of ["platform", "status", "base", "code", "connect", "disconnect", "request"]) element(name).style.display = value ? "none" : "";
    const button = element("minimize");
    button.textContent = value ? "BPM" : "最小化";
    button.title = value ? "展开 Best Post Manager bridge" : "最小化 bridge";
    button.style.margin = value ? "0" : "5px";
    button.style.width = value ? "46px" : "auto";
    button.style.height = value ? "46px" : "auto";
  }
  const addButton = (label, action) => {
    const button = document.createElement("button");
    button.textContent = label;
    button.addEventListener("click", action);
    requestBox.appendChild(button);
  };
  async function connect() {
    config.base = element("base").value.trim().replace(/\/$/, "");
    const code = element("code").value.trim();
    if (!/^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(config.base)) throw new Error("本地服务地址必须是 127.0.0.1 或 localhost");
    if (!code) throw new Error("请输入 WebUI 生成的一次性 pairing code");
    GM_setValue(configKey, { base: config.base });
    const hello = await call("/bridge/v1/hello", { method: "POST", body: JSON.stringify({ code, platform, page_url: location.href }) });
    config.token = hello.token;
    config.pairing = hello.id;
    GM_setValue(configKey, { base: config.base, token: config.token, pairing: config.pairing });
    element("code").value = "";
    status("已连接；token 仅保存在油猴隔离存储中（服务端 TTL 90 天）");
    if (!polling) {
      polling = true;
      void poll();
    }
  }
  function showCapture(request) {
    const summary = document.createElement("div");
    summary.textContent = "本地请求：捕获当前可见帖子";
    requestBox.appendChild(summary);
    addButton("确认捕获当前可见内容", () => void (async () => {
      const snapshot = captureVisible(platform, location.href);
      await call(`/bridge/v1/requests/${request.id}/capture`, { method: "POST", body: JSON.stringify(snapshot) });
      status(snapshot.incomplete ? `已保存不完整快照：${snapshot.incomplete_reason}` : "已保存当前可见快照");
      current = null;
      requestBox.replaceChildren();
    })().catch((error) => status(`捕获失败：${String(error)}`)));
  }
  function showFill(request) {
    var _a;
    const summary = document.createElement("div");
    summary.textContent = `请求 ${request.id.slice(0, 8)}：${(request.text || "").slice(0, 80)}；图片 ${((_a = request.media) == null ? void 0 : _a.length) || 0} 张`;
    requestBox.appendChild(summary);
    status(request.auto_publish ? "已收到本次自动发布请求，正在填入并检查发布条件" : "已收到填入请求，正在写入当前可见编辑器（不会发布）");
    void (async () => {
      var _a2;
      try {
        const failures = await fillVisibleWithMedia(platform, request, localMediaFetcher);
        if (request.auto_publish && ((_a2 = request.media) == null ? void 0 : _a2.length)) {
          failures.push("页面媒体上传完成状态无法可靠确认，已取消自动点击；请检查图片后人工发布");
        } else if (request.auto_publish && failures.length === 0) {
          const publishWarning = clickKnownPublish(platform);
          if (publishWarning) failures.push(publishWarning);
        }
        await call(`/bridge/v1/requests/${request.id}/filled`, { method: "POST", body: JSON.stringify({ warning: failures.join("; ") }) });
        status(failures.length ? `文字已填入；请人工处理：${failures.join("；")}` : request.auto_publish ? "已填入并点击可见发布按钮；请确认平台结果" : "已填入，请人工检查可见范围和图片顺序后发布");
      } catch (error) {
        await call(`/bridge/v1/requests/${request.id}/error`, { method: "POST", body: JSON.stringify({ error: String(error) }) });
        status(`填入失败：${String(error)}`);
        current = null;
        requestBox.replaceChildren();
      }
    })();
    const resultURL = document.createElement("input");
    resultURL.placeholder = "发布后的帖子永久链接";
    resultURL.value = location.href;
    requestBox.appendChild(resultURL);
    addButton("我已人工发布，回传链接", () => void (async () => {
      await call(`/bridge/v1/requests/${request.id}/result`, { method: "POST", body: JSON.stringify({ url: resultURL.value.trim() }) });
      status("已回传人工确认的发布结果");
      current = null;
      requestBox.replaceChildren();
    })().catch((error) => status(`回传失败：${String(error)}`)));
  }
  async function poll() {
    if (!config.token || !config.pairing) {
      polling = false;
      return;
    }
    try {
      const requests = await call("/bridge/v1/requests");
      const request = requests[0];
      if (request && request.id !== (current == null ? void 0 : current.id)) {
        current = request;
        requestBox.replaceChildren();
        if (request.kind === "capture") showCapture(request);
        else showFill(request);
      }
    } catch (error) {
      status(`连接不可用：${error instanceof Error ? error.message : String(error)}`);
    }
    if (config.token && config.pairing) window.setTimeout(() => void poll(), 3e3);
    else polling = false;
  }
  element("connect").addEventListener("click", () => void connect().catch((error) => status(`连接失败：${error instanceof Error ? error.message : String(error)}`)));
  element("disconnect").addEventListener("click", () => {
    config.token = "";
    config.pairing = "";
    current = null;
    requestBox.replaceChildren();
    GM_setValue(configKey, { base: config.base });
    status("已断开；脚本存储中的 token 已清除");
  });
  element("minimize").addEventListener("click", () => setMinimized(!minimized));
  setMinimized(minimized);
  if (config.token && config.pairing) {
    status("已恢复连接，正在检查本地请求");
    polling = true;
    void poll();
  }
})();
