/* Generic opt-in gateway subpath compatibility. Upstream-specific code is not injected. */
(() => {
  "use strict";
  const prefix = __GATEWAY_PREFIX__;
  const namespace = __GATEWAY_COOKIE_PREFIX__;
  const target = new URL(__GATEWAY_TARGET__);
  const scopedURL = value => {
    if (typeof value !== "string" && !(value instanceof URL)) return value;
    if (/^(#|\?|data:|blob:|mailto:|tel:|javascript:)/i.test(String(value))) return value;
    try {
      const url = new URL(String(value), document.baseURI);
      if (!["http:", "https:", "ws:", "wss:"].includes(url.protocol)) return value;
      const isUpstream = url.host === target.host;
      if (url.host !== location.host && !isUpstream) return value;
      if (isUpstream) {
        const publicURL = new URL(document.baseURI);
        url.hostname = publicURL.hostname;
        url.port = publicURL.port;
        url.protocol = ["ws:", "wss:"].includes(url.protocol) ? (location.protocol === "https:" ? "wss:" : "ws:") : location.protocol;
      }
      if (url.pathname !== prefix && !url.pathname.startsWith(prefix + "/")) {
        const base = target.pathname.replace(/\/$/, "");
        if (base && (url.pathname === base || url.pathname.startsWith(base + "/"))) url.pathname = url.pathname.slice(base.length) || "/";
        url.pathname = prefix + url.pathname;
      }
      return url.href;
    } catch { return value; }
  };
  const fetchOriginal = window.fetch;
  window.fetch = function(input, init) {
    if (input instanceof Request) return fetchOriginal.call(this, new Request(scopedURL(input.url), input), init);
    return fetchOriginal.call(this, scopedURL(input), init);
  };
  const open = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url, ...args) { return open.call(this, method, scopedURL(url), ...args); };
  for (const method of ["pushState", "replaceState"]) {
    const original = history[method];
    history[method] = function(state, title, url) { return original.call(this, state, title, url == null ? url : scopedURL(url)); };
  }
  if (window.WebSocket) {
    const Original = window.WebSocket;
    window.WebSocket = class extends Original { constructor(url, protocols) { super(scopedURL(url), protocols); } };
  }
  if (window.EventSource) {
    const Original = window.EventSource;
    window.EventSource = class extends Original { constructor(url, options) { super(scopedURL(url), options); } };
  }
  const windowOpen = window.open;
  window.open = function(url, ...args) { return windowOpen.call(this, scopedURL(url), ...args); };
  // Dynamic resources and form submissions stay inside this service.

  for (const [prototype, property] of [[HTMLLinkElement.prototype, "href"], [HTMLScriptElement.prototype, "src"], [HTMLImageElement.prototype, "src"], [HTMLFormElement.prototype, "action"], [HTMLAnchorElement.prototype, "href"], [HTMLIFrameElement.prototype, "src"], [HTMLSourceElement.prototype, "src"], [HTMLVideoElement.prototype, "poster"]]) {
    const descriptor = Object.getOwnPropertyDescriptor(prototype, property);
    if (!descriptor?.set) continue;
    Object.defineProperty(prototype, property, { ...descriptor, set(value) { descriptor.set.call(this, scopedURL(value)); } });
  }
  const setAttribute = Element.prototype.setAttribute;
  Element.prototype.setAttribute = function(name, value) {
    return setAttribute.call(this, name, ["src", "href", "action", "poster"].includes(String(name).toLowerCase()) ? scopedURL(value) : value);
  };
  // Covers links/forms created via innerHTML without touching Hash navigation.
  document.addEventListener("click", event => {
    const anchor = event.target?.closest?.("a[href]");
    if (anchor) {
      const href = anchor.getAttribute("href");
      if (href && !href.startsWith("#")) anchor.setAttribute("href", scopedURL(href));
    }
  }, true);
  document.addEventListener("submit", event => {
    const form = event.target;
    if (form instanceof HTMLFormElement) {
      const action = form.getAttribute("action");
      if (action) form.setAttribute("action", scopedURL(action));
    }
  }, true);
  // Expose only this service's cookie namespace to upstream scripts.
  const cookie = Object.getOwnPropertyDescriptor(Document.prototype, "cookie");
  if (cookie?.get && cookie?.set) Object.defineProperty(document, "cookie", {
    configurable: true,
    get() { return cookie.get.call(document).split(/;\s*/).filter(item => item.startsWith(namespace)).map(item => item.slice(namespace.length)).join("; "); },
    set(value) {
      const parts = String(value).split(";");
      const first = parts.shift();
      if (!first || first.indexOf("=") <= 0) return;
      const attributes = parts.filter(item => !/^\s*(path|domain)\s*=/i.test(item));
      cookie.set.call(document, namespace + first.trim() + "; Path=" + prefix + "/;" + attributes.join(";"));
    }
  });
  // Namespace persisted UI/session state so unrelated devices do not collide.
  for (const property of ["localStorage", "sessionStorage"]) {
    let storage;
    try { storage = window[property]; } catch { continue; }
    const keys = () => Array.from({length: storage.length}, (_, i) => storage.key(i)).filter(key => key?.startsWith(namespace));
    const api = {
      getItem: key => storage.getItem(namespace + key),
      setItem: (key, value) => storage.setItem(namespace + key, value),
      removeItem: key => storage.removeItem(namespace + key),
      clear: () => keys().forEach(key => storage.removeItem(key)),
      key: index => keys()[index]?.slice(namespace.length) ?? null,
      get length() { return keys().length; }
    };
    Object.defineProperty(window, property, { configurable: true, value: new Proxy(api, {
      get(target, key) { return key in target ? Reflect.get(target, key) : storage.getItem(namespace + String(key)); },
      set(target, key, value) { storage.setItem(namespace + String(key), value); return true; },
      deleteProperty(target, key) { storage.removeItem(namespace + String(key)); return true; },
      ownKeys() { return keys().map(key => key.slice(namespace.length)); },
      getOwnPropertyDescriptor(target, key) {
        const value = storage.getItem(namespace + String(key));
        return value === null ? undefined : { configurable: true, enumerable: true, writable: true, value };
      }
    }) });
  }
})();
