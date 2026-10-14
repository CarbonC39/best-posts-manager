// ==UserScript==
// @name         Best Post Manager local bridge
// @namespace    best-post-manager
// @version      1.6.1
// @description  Fill visible editors, safely publish X or Weibo on explicit request, and capture visible posts.
// @match        https://x.com/*
// @match        https://*.x.com/*
// @match        https://twitter.com/*
// @match        https://*.twitter.com/*
// @match        https://weibo.com/*
// @match        https://*.weibo.com/*
// @match        https://xiaohongshu.com/*
// @match        https://*.xiaohongshu.com/*
// @match        https://rednote.com/*
// @match        https://*.rednote.com/*
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
  const platformRegistry = {
    mastodon: {
      label: "Mastodon",
      accountSetupMode: "oauth",
      setupDescription: "Mastodon：推荐 OAuth 自动连接；也可手动保存 access token，或使用环境变量。"
    },
    "mastodon-compatible": {
      label: "Mastodon",
      accountSetupMode: "credential",
      setupDescription: "Mastodon：推荐 OAuth 自动连接；也可手动保存 access token，或使用环境变量。"
    },
    misskey: {
      label: "Misskey",
      accountSetupMode: "miauth",
      setupDescription: "Misskey：推荐 MiAuth 自动连接；也可手动保存 access token，或使用环境变量。"
    },
    bluesky: {
      label: "Bluesky",
      accountSetupMode: "credential",
      setupDescription: "Bluesky：填写 handle/identifier、PDS（默认 https://bsky.social）与 app password。app password 可直接保存在本机凭据文件，也可使用环境变量。"
    },
    x: {
      label: "X",
      accountSetupMode: "bridge",
      setupDescription: "X：使用浏览器桥接；默认只填入，纯文本可对单次请求确认自动发布。",
      bridge: {
        hosts: ["x.com", "twitter.com"],
        matches: ["https://x.com/*", "https://*.x.com/*", "https://twitter.com/*", "https://*.twitter.com/*"],
        clientAutoPublish: "text-only"
      }
    },
    weibo: {
      label: "微博",
      accountSetupMode: "bridge",
      setupDescription: "微博：使用浏览器桥接填入；纯文本可在最终确认时选择自动发布，带图片仍需人工确认。",
      bridge: {
        hosts: ["weibo.com"],
        matches: ["https://weibo.com/*", "https://*.weibo.com/*"],
        clientAutoPublish: "text-only"
      }
    },
    xiaohongshu: {
      label: "小红书",
      accountSetupMode: "bridge",
      setupDescription: "小红书：使用浏览器桥接填入；发布按钮选择器尚未安全验收，因此最终发布保持人工操作。",
      bridge: {
        hosts: ["xiaohongshu.com", "rednote.com"],
        matches: ["https://xiaohongshu.com/*", "https://*.xiaohongshu.com/*", "https://rednote.com/*", "https://*.rednote.com/*"],
        clientAutoPublish: "none"
      }
    },
    qzone: {
      label: "QQ 空间",
      accountSetupMode: "bridge",
      setupDescription: "QQ空间：使用浏览器桥接填入；可见范围、发布和图片顺序仍需人工检查。",
      bridge: {
        hosts: ["qzone.qq.com"],
        matches: ["https://qzone.qq.com/*", "https://*.qzone.qq.com/*"],
        clientAutoPublish: "none"
      }
    }
  };
  const platformIDs = Object.keys(platformRegistry);
  const bridgePlatformIDs = platformIDs.filter((platform2) => "bridge" in platformRegistry[platform2]);
  bridgePlatformIDs.flatMap((platform2) => platformRegistry[platform2].bridge.matches);
  function platformForBridgeHost(hostname) {
    const host2 = hostname.toLowerCase();
    return bridgePlatformIDs.find((platform2) => platformRegistry[platform2].bridge.hosts.some((domain) => host2 === domain || host2.endsWith(`.${domain}`))) || null;
  }
  const adapters = {
    x: { selectors: ['[data-testid="tweetTextarea_0"]', 'textarea[aria-label*="Post"]'], captureSelectors: ['article [data-testid="tweetText"]'], mediaSelectors: ['input[type="file"]'], publishSelectors: ['[data-testid="tweetButtonInline"]', '[data-testid="tweetButton"]'] },
    weibo: { selectors: ['textarea[placeholder="有什么新鲜事想分享给大家？"]', 'textarea[placeholder*="分享"]', '[contenteditable="true"][node-type*="text"]'], captureSelectors: [".WB_text", '[node-type="feed_list_content"]'], mediaSelectors: ['.wbpro-form input[type="file"]', 'input[type="file"]'], publishSelectors: [".wbpro-form button", '.wbpro-form [role="button"]'], publishLabels: ["发送", "发布"] },
    xiaohongshu: { selectors: ['.ql-editor[contenteditable="true"]', '[contenteditable="true"][data-placeholder*="正文"]', 'textarea[placeholder*="正文"]', 'textarea[placeholder*="描述"]'], captureSelectors: [".note-content", '[class*="note-content"]'], mediaSelectors: ['.image-upload-buttons input[type="file"]', 'input[type="file"]'], publishSelectors: [] },
    qzone: { selectors: ['.qz-poster-editor-cont [x\\:id="content_content"]', '.qz-poster-editor-cont [contenteditable="true"][class~="textinput"]', 'textarea[placeholder*="说说"]'], captureSelectors: [".feed-content", '[class*="feed-content"]'], mediaSelectors: ['.qz-poster-editor-cont input[type="file"]', 'input[type="file"]'], publishSelectors: [] }
  };
  function adapter(platform2) {
    const value = adapters[platform2];
    if (!value) throw new Error(`unsupported bridge platform: ${platform2}`);
    return value;
  }
  function findComposer(platform2, root = document) {
    for (const selector of adapter(platform2).selectors) {
      const candidates = root.querySelectorAll(selector);
      for (const found of candidates) {
        if (!found.hidden && found.getAttribute("aria-hidden") !== "true" && found.style.display !== "none") return found;
      }
    }
    return null;
  }
  function composer(platform2, root = document) {
    if (platform2 === "qzone") {
      const container = root.querySelector(".qz-poster-editor-cont");
      const content = container == null ? void 0 : container.querySelector('[x\\:id="content_content"]');
      if (content) {
        const substitutor = container == null ? void 0 : container.querySelector('[x\\:id="substitutor_content"]');
        if (substitutor && !substitutor.hidden && substitutor.getAttribute("aria-hidden") !== "true" && substitutor.style.display !== "none") substitutor.click();
        content.focus();
        return content;
      }
    }
    const found = findComposer(platform2, root);
    if (found) return found;
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
  function setQzoneText(element2, text) {
    var _a;
    element2.focus();
    const selection = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(element2);
    selection == null ? void 0 : selection.removeAllRanges();
    selection == null ? void 0 : selection.addRange(range);
    element2.dispatchEvent(new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType: "insertText", data: text }));
    const editableDocument = document;
    let inserted = false;
    try {
      inserted = ((_a = editableDocument.execCommand) == null ? void 0 : _a.call(editableDocument, "insertText", false, text)) === true;
    } catch {
    }
    if (!inserted) {
      range.deleteContents();
      const node = document.createTextNode(text);
      range.insertNode(node);
      range.setStartAfter(node);
      range.collapse(true);
      selection == null ? void 0 : selection.removeAllRanges();
      selection == null ? void 0 : selection.addRange(range);
    }
    element2.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText", data: text }));
    element2.dispatchEvent(new Event("change", { bubbles: true }));
  }
  function fillVisible(platform2, text) {
    const element2 = composer(platform2);
    if (platform2 === "qzone" && !(element2 instanceof HTMLTextAreaElement)) setQzoneText(element2, text);
    else setText(element2, text);
    for (const selector of adapter(platform2).mediaSelectors) {
      const mediaInput = document.querySelector(selector);
      if (mediaInput) return { mediaInput };
    }
    return {};
  }
  function visibleElement(selectors) {
    for (const selector of selectors) {
      for (const element2 of document.querySelectorAll(selector)) {
        if (!element2.hidden && element2.getAttribute("aria-hidden") !== "true" && element2.style.display !== "none") return element2;
      }
    }
    return null;
  }
  const wait = (milliseconds) => new Promise((resolve) => window.setTimeout(resolve, milliseconds));
  async function waitForVisible(selectors, timeout = 4e3) {
    const deadline = Date.now() + timeout;
    let found = visibleElement(selectors);
    while (!found && Date.now() < deadline) {
      await wait(100);
      found = visibleElement(selectors);
    }
    return found;
  }
  async function prepareXiaohongshuComposer(hasMedia) {
    const creatorTab = visibleElement([".creator-tab"]);
    const modeSelectors = hasMedia ? [".image-upload-buttons .upload-button"] : [".image-upload-buttons .text2image-button"];
    const initialModeButton = visibleElement(modeSelectors);
    if (!creatorTab && !initialModeButton && findComposer("xiaohongshu")) return;
    if (creatorTab) {
      creatorTab.click();
    }
    const modeButton = creatorTab ? await waitForVisible(modeSelectors) : initialModeButton;
    if (modeButton) {
      modeButton.click();
      const deadline = Date.now() + 4e3;
      while (!findComposer("xiaohongshu") && Date.now() < deadline) await wait(100);
    }
  }
  function clickKnownPublish(platform2) {
    var _a;
    const viable = /* @__PURE__ */ new Set();
    const labels = adapter(platform2).publishLabels;
    for (const selector of adapter(platform2).publishSelectors) {
      const candidates = document.querySelectorAll(selector);
      for (const element2 of candidates) {
        if (element2.getAttribute("aria-disabled") === "true" || element2.disabled) continue;
        let current2 = element2;
        let hidden = false;
        while (current2) {
          const style = getComputedStyle(current2);
          if (current2.hidden || current2.getAttribute("aria-hidden") === "true" || style.display === "none" || style.visibility === "hidden" || style.opacity === "0") {
            hidden = true;
            break;
          }
          current2 = current2.parentElement;
        }
        if (hidden) continue;
        if (labels && !labels.includes((element2.innerText || element2.textContent || "").trim())) continue;
        viable.add(element2);
      }
    }
    if (viable.size === 0) return "当前页面未找到明确的可见发布按钮，未自动点击；请人工发布";
    if (viable.size > 1) return "当前页面同时出现多个发布按钮，无法安全确定目标，未自动点击";
    (_a = viable.values().next().value) == null ? void 0 : _a.click();
    return null;
  }
  async function fillVisibleWithMedia(platform2, request, fetcher = fetch) {
    var _a, _b;
    if (platform2 === "xiaohongshu") await prepareXiaohongshuComposer(Boolean((_a = request.media) == null ? void 0 : _a.length));
    const { mediaInput } = fillVisible(platform2, request.text || "");
    const failures = [];
    if (!((_b = request.media) == null ? void 0 : _b.length)) return failures;
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
  [...bridgePlatformIDs];
  const host = location.hostname.toLowerCase();
  const platform = platformForBridgeHost(host) || "qzone";
  const configKey = `bpm-bridge-config-v1-${platform}`;
  const saved = GM_getValue(configKey, {});
  const injectedOrigin = "__BPM_ORIGIN__";
  const defaultBase = injectedOrigin.startsWith("http://") || injectedOrigin.startsWith("https://") ? injectedOrigin : "http://127.0.0.1:8787";
  const config = { base: saved.base || defaultBase, token: "", pairing: "" };
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
  panel.style.cssText = 'position:fixed;z-index:2147483647;right:16px;bottom:16px;width:304px;color:#20241f;font:13px Inter,"Segoe UI",system-ui,sans-serif;line-height:1.45';
  panel.innerHTML = `<style>
  *{box-sizing:border-box}button,input{font:inherit}button{cursor:pointer}
  .bpm-shell{overflow:hidden;border:1px solid #d2d8d1;border-radius:14px;background:#fff;box-shadow:0 16px 44px rgb(24 38 29 / 18%)}
  .bpm-head{display:flex;align-items:center;gap:10px;min-height:48px;padding:8px 9px 8px 11px;background:#f7f8f6;cursor:grab;user-select:none;touch-action:none}
  .bpm-head:active{cursor:grabbing}.bpm-mark{display:grid;place-items:center;flex:0 0 30px;width:30px;height:30px;border-radius:9px;color:#fff;background:#264e36}.bpm-mark svg{width:17px;height:17px}
  .bpm-title{display:grid;min-width:0;line-height:1.15}.bpm-title strong{font-size:13px}.bpm-title small{margin-top:3px;color:#737c75;font-size:10px;text-transform:uppercase;letter-spacing:.08em}
  .bpm-icon{display:grid;place-items:center;width:30px;height:30px;margin-left:auto;padding:0;border:0;border-radius:7px;color:#607068;background:transparent;font-size:20px;line-height:1}.bpm-icon:hover{color:#173625;background:#e8f1eb}
  .bpm-content{display:grid;gap:11px;max-height:min(520px,calc(100vh - 92px));padding:12px;overflow:auto}
  .bpm-status{display:flex;align-items:flex-start;gap:8px;color:#496452;font-size:12px;overflow-wrap:anywhere}.bpm-dot{flex:0 0 7px;width:7px;height:7px;margin-top:5px;border-radius:50%;background:#c68a48}.bpm-shell[data-connected="true"] .bpm-dot{background:#5da777}
  label{display:grid;gap:5px;color:#3f4741;font-size:11px;font-weight:650}input{width:100%;height:36px;padding:0 9px;border:1px solid #c8d0c9;border-radius:7px;color:#20241f;background:#fff;font-weight:400;outline:0}input::placeholder{color:#707970}input:focus{border-color:#76a088;box-shadow:0 0 0 3px rgb(93 167 119 / 15%)}
  .bpm-actions{display:flex;align-items:center;gap:7px}.bpm-button{min-height:34px;padding:6px 10px;border:1px solid #c8d0c9;border-radius:7px;color:#405047;background:#fff}.bpm-button:hover{background:#f3f6f3}.bpm-button.primary{border-color:#264e36;color:#fff;background:#264e36}.bpm-button.primary:hover{background:#1e3f2c}
  [data-bpm=request]{display:grid;gap:8px;padding-top:10px;border-top:1px solid #e2e6e0;color:#3f4741;font-size:12px}[data-bpm=request]:empty{display:none}[data-bpm=request] button{justify-self:start;min-height:34px;padding:6px 10px;border:1px solid #c8d0c9;border-radius:7px;color:#315840;background:#fff}[data-bpm=request] input{margin-top:2px}
  .bpm-shell.minimized{width:52px;height:52px;border-radius:50%;border-color:#b9c9be;box-shadow:0 10px 26px rgb(24 38 29 / 22%)}.bpm-shell.minimized .bpm-head{width:52px;height:52px;min-height:52px;padding:0;justify-content:center;background:#264e36;cursor:grab}.bpm-shell.minimized .bpm-mark{width:52px;height:52px;border-radius:50%;background:transparent}.bpm-shell.minimized .bpm-mark svg{width:20px;height:20px}.bpm-shell.minimized .bpm-title,.bpm-shell.minimized .bpm-icon,.bpm-shell.minimized .bpm-content{display:none}
</style><section class="bpm-shell" data-bpm="shell"><header class="bpm-head" data-bpm="drag"><span class="bpm-mark" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="M4 16 16 4M7 4h9v9" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8"/></svg></span><span class="bpm-title"><strong>Best Post Manager</strong><small>Bridge · ${platform}</small></span><button type="button" class="bpm-icon" data-bpm="minimize" aria-label="最小化桥接面板" title="最小化">−</button></header><div class="bpm-content" data-bpm="content"><div class="bpm-status"><span class="bpm-dot" aria-hidden="true"></span><span data-bpm="status">未连接</span></div><label>本地服务<input data-bpm="base" placeholder="http://127.0.0.1:8787"></label><label>配对 code<input data-bpm="code" placeholder="一次性 code"></label><div class="bpm-actions"><button type="button" class="bpm-button primary" data-bpm="connect">连接</button><button type="button" class="bpm-button" data-bpm="disconnect">断开</button></div><div data-bpm="request"></div></div></section>`;
  const panelHost = document.createElement("div");
  panelHost.attachShadow({ mode: "closed" }).appendChild(panel);
  document.body.appendChild(panelHost);
  const element = (name) => panel.querySelector(`[data-bpm="${name}"]`);
  element("base").value = config.base;
  const shell = element("shell");
  const status = (text) => {
    element("status").textContent = text;
    shell.dataset.connected = String(Boolean(config.token && config.pairing));
  };
  const requestBox = element("request");
  let minimized = GM_getValue(`${configKey}-minimized`, false);
  const positionKey = `${configKey}-position`;
  const savedPosition = GM_getValue(positionKey, {});
  if (Number.isFinite(savedPosition.left) && Number.isFinite(savedPosition.top)) {
    panel.style.right = "auto";
    panel.style.bottom = "auto";
    panel.style.left = `${savedPosition.left}px`;
    panel.style.top = `${savedPosition.top}px`;
  }
  function clampPanel(left, top) {
    const rect = panel.getBoundingClientRect();
    return { left: Math.max(8, Math.min(left, window.innerWidth - rect.width - 8)), top: Math.max(8, Math.min(top, window.innerHeight - rect.height - 8)) };
  }
  function setPanelPosition(left, top, save = false) {
    const position = clampPanel(left, top);
    panel.style.right = "auto";
    panel.style.bottom = "auto";
    panel.style.left = `${position.left}px`;
    panel.style.top = `${position.top}px`;
    if (save) GM_setValue(positionKey, position);
  }
  function setMinimized(value) {
    const before = panel.getBoundingClientRect();
    minimized = value;
    GM_setValue(`${configKey}-minimized`, value);
    shell.classList.toggle("minimized", value);
    panel.style.width = value ? "52px" : "304px";
    const dragHandle = element("drag");
    if (value) {
      dragHandle.setAttribute("role", "button");
      dragHandle.setAttribute("tabindex", "0");
      dragHandle.setAttribute("aria-label", "展开桥接面板");
    } else {
      dragHandle.removeAttribute("role");
      dragHandle.removeAttribute("tabindex");
      dragHandle.removeAttribute("aria-label");
    }
    const after = panel.getBoundingClientRect();
    if (panel.style.left) setPanelPosition(before.right - after.width, before.bottom - after.height, true);
  }
  let dragged = false;
  element("drag").addEventListener("pointerdown", (event) => {
    if (event.target.closest('[data-bpm="minimize"]')) return;
    const handle = event.currentTarget;
    const start = panel.getBoundingClientRect();
    const originX = event.clientX;
    const originY = event.clientY;
    dragged = false;
    handle.setPointerCapture(event.pointerId);
    const move = (next) => {
      const dx = next.clientX - originX;
      const dy = next.clientY - originY;
      if (Math.abs(dx) + Math.abs(dy) > 3) dragged = true;
      setPanelPosition(start.left + dx, start.top + dy);
    };
    const up = (next) => {
      handle.removeEventListener("pointermove", move);
      handle.removeEventListener("pointerup", up);
      const rect = panel.getBoundingClientRect();
      if (dragged) {
        next.preventDefault();
        GM_setValue(positionKey, { left: rect.left, top: rect.top });
      }
    };
    handle.addEventListener("pointermove", move);
    handle.addEventListener("pointerup", up);
  });
  element("drag").addEventListener("click", (event) => {
    if (event.target.closest('[data-bpm="minimize"]')) return;
    if (minimized && !dragged) setMinimized(false);
    dragged = false;
  });
  element("drag").addEventListener("keydown", (event) => {
    if (minimized && (event.key === "Enter" || event.key === " ")) {
      event.preventDefault();
      setMinimized(false);
    }
  });
  window.addEventListener("resize", () => {
    if (panel.style.left) {
      const rect = panel.getBoundingClientRect();
      setPanelPosition(rect.left, rect.top, true);
    }
  });
  const addButton = (label, action) => {
    const button = document.createElement("button");
    button.textContent = label;
    button.addEventListener("click", action);
    requestBox.appendChild(button);
    return button;
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
    status("已连接");
    if (!polling) {
      polling = true;
      void poll();
    }
  }
  function showCapture(request) {
    const snapshot = captureVisible(platform, location.href);
    const summary = document.createElement("div");
    summary.textContent = `准备捕获：${snapshot.text.slice(0, 160) || "未读取到正文"}${snapshot.incomplete ? `（${snapshot.incomplete_reason}）` : ""}`;
    requestBox.appendChild(summary);
    addButton("确认捕获当前可见内容", () => void (async () => {
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
    const resultURL = document.createElement("input");
    resultURL.placeholder = "发布后的帖子永久链接（不是编辑页地址）";
    requestBox.appendChild(resultURL);
    const resultButton = addButton("确认已发布并回传链接", () => void (async () => {
      const url = resultURL.value.trim();
      if (!url) throw new Error("请先填写发布后的帖子永久链接");
      await call(`/bridge/v1/requests/${request.id}/result`, { method: "POST", body: JSON.stringify({ url }) });
      status("已回传人工确认的发布结果");
      current = null;
      requestBox.replaceChildren();
    })().catch((error) => status(`回传失败：${String(error)}`)));
    resultButton.disabled = true;
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
        resultButton.disabled = false;
        status(failures.length ? `文字已填入；请人工处理：${failures.join("；")}` : request.auto_publish ? "已填入并点击可见发布按钮；请确认平台结果" : "已填入，请人工检查可见范围和图片顺序后发布");
      } catch (error) {
        await call(`/bridge/v1/requests/${request.id}/error`, { method: "POST", body: JSON.stringify({ error: String(error) }) });
        status(`填入失败：${String(error)}`);
        current = null;
        requestBox.replaceChildren();
      }
    })();
  }
  async function poll() {
    if (!config.token || !config.pairing) {
      polling = false;
      return;
    }
    try {
      const requests = await call("/bridge/v1/requests");
      const request = requests[0];
      if (!current && request) {
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
    status("已断开");
  });
  element("minimize").addEventListener("click", () => setMinimized(!minimized));
  setMinimized(minimized);
  if (config.token && config.pairing) {
    status("已恢复连接，正在检查本地请求");
    polling = true;
    void poll();
  }
})();
