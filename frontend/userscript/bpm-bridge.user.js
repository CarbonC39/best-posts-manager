// ==UserScript==
// @name         Best Post Manager local bridge
// @namespace    best-post-manager
// @version      1.0.0
// @description  Fill visible editors and capture visible posts; final publishing always stays manual.
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
// @connect      127.0.0.1
// @connect      localhost
// @noframes
// ==/UserScript==
(function() {
  "use strict";
  const adapters = {
    x: { selectors: ['[data-testid="tweetTextarea_0"]', 'textarea[aria-label*="Post"]', "textarea", '[contenteditable="true"]'], captureSelectors: ['article [data-testid="tweetText"]', "article"], mediaSelectors: ['input[type="file"]'] },
    weibo: { selectors: ['textarea[placeholder*="分享"]', "textarea", '[contenteditable="true"]'], captureSelectors: [".WB_text", '[node-type="feed_list_content"]'], mediaSelectors: ['input[type="file"]'] },
    xiaohongshu: { selectors: ["textarea", '[contenteditable="true"]'], captureSelectors: [".note-content", '[class*="note-content"]'], mediaSelectors: ['input[type="file"]'] },
    qzone: { selectors: ["textarea", '[contenteditable="true"]'], captureSelectors: [".feed-content", '[class*="content"]'], mediaSelectors: ['input[type="file"]'] }
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
  const saved = JSON.parse(localStorage.getItem("bpm-bridge-config-v1") || "{}");
  const config = { base: saved.base || "http://127.0.0.1:8787", token: "", pairing: "" };
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
  panel.innerHTML = '<b>Best Post Manager bridge v1</b><div data-bpm="platform"></div><div data-bpm="status">未连接</div><input data-bpm="base" placeholder="本地服务地址"><input data-bpm="code" placeholder="一次性 pairing code"><button data-bpm="connect">连接</button><div data-bpm="request"></div>';
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
    localStorage.setItem("bpm-bridge-config-v1", JSON.stringify({ base: config.base }));
    const hello = await call("/bridge/v1/hello", { method: "POST", body: JSON.stringify({ code, platform, page_url: location.href }) });
    config.token = hello.token;
    config.pairing = hello.id;
    element("code").value = "";
    status("已连接；token 仅保存在当前页面内存中");
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
    summary.textContent = `待填入：${(request.text || "").slice(0, 80)}；图片 ${((_a = request.media) == null ? void 0 : _a.length) || 0} 张`;
    requestBox.appendChild(summary);
    addButton("填入（绝不发布）", () => void (async () => {
      try {
        const failures = await fillVisibleWithMedia(platform, request, localMediaFetcher);
        status(failures.length ? `文字已填入；请人工处理：${failures.join("；")}` : "已填入，请人工检查可见范围和图片顺序后发布");
      } catch (error) {
        await call(`/bridge/v1/requests/${request.id}/error`, { method: "POST", body: JSON.stringify({ error: String(error) }) });
        status(`填入失败：${String(error)}`);
        current = null;
      }
    })());
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
    window.setTimeout(() => void poll(), 3e3);
  }
  element("connect").addEventListener("click", () => void connect().catch((error) => status(`连接失败：${error instanceof Error ? error.message : String(error)}`)));
})();
