<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, shallowRef } from "vue";
import {
  PhGlobe,
  PhHardDrives,
  PhPlugsConnected,
  PhBrowser,
  PhDatabase,
} from "@phosphor-icons/vue";
import AppSelect from "./AppSelect.vue";
import { request, errorMessage } from "../api";
interface Site {
  id: string;
  name: string;
  listener: string;
  targets: string[] | null;
}
interface Flow {
  rule: string;
  client: string;
  upstream: string;
  requests: number;
  bytes: number;
}
interface Frame {
  connections: number | null;
  sites: Site[];
  flows: Flow[];
  limited: boolean;
  logging: boolean;
  time: number;
}
const frames = shallowRef<Frame[]>([]),
  replay = ref(false),
  index = ref(0);
const frame = computed(
  () => frames.value[replay.value ? index.value : frames.value.length - 1],
);
const search = ref(""),
  ip = ref(""),
  focus = ref(false),
  mask = ref(false),
  selected = ref("nginx"),
  error = ref("");
const zoom = ref(1),
  layout = ref("tree"),
  listenerFilter = ref(""),
  showFilter = ref(false);
const listeners = computed(() => [
  ...new Set(frame.value?.sites.map((s) => s.listener)),
]);
const visitors = computed(() =>
  [...new Set(frame.value?.flows.map((f) => f.client).filter(Boolean))].sort(),
);
const icons = [PhGlobe, PhHardDrives, PhPlugsConnected, PhBrowser, PhDatabase];
const names = ["互联网", "Nginx", "监听", "站点 / 动作", "后端 / 去向"];
const xs = [80, 220, 385, 600, 850];
interface Node {
  id: string;
  label: string;
  col: number;
  x: number;
  y: number;
  parent?: Node;
  hits: number;
  bytes: number;
}
const graph = computed(() => {
  const nodes: Node[] = [],
    byId = new Map<string, Node>();
  function put(id: string, label: string, col: number, parent?: Node) {
    let n = byId.get(id);
    if (!n) {
      n = { id, label, col, x: xs[col], y: 0, hits: 0, bytes: 0, parent };
      byId.set(id, n);
      nodes.push(n);
    }
    return n;
  }
  const root = put("internet", "互联网", 0),
    nginx = put("nginx", "Nginx", 1, root);
  const flowsBySite = new Map<string, Flow[]>();
  for (const f of frame.value?.flows ?? []) {
    if (ip.value && ip.value !== f.client) continue;
    const group = flowsBySite.get(f.rule) ?? [];
    group.push(f);
    flowsBySite.set(f.rule, group);
  }
  let row = 0;
  for (const site of frame.value?.sites ?? []) {
    if (listenerFilter.value && site.listener !== listenerFilter.value)
      continue;
    const flows = flowsBySite.get(site.id) ?? [];
    const destinations = new Set(site.targets ?? []);
    for (const f of flows)
      for (const t of f.upstream.split(", "))
        if (t && t !== "-") destinations.add(t);
    if (focus.value && !flows.length) continue;
    if (
      ![site.name, site.listener, ...destinations]
        .join(" ")
        .toLowerCase()
        .includes(search.value.trim().toLowerCase())
    )
      continue;
    const listener = put(site.listener, site.listener, 2, nginx);
    const host = put(site.id, site.name, 3, listener);
    for (const target of destinations) {
      const dest = put(site.id + "|" + target, target, 4, host);
      dest.y = 80 + row++ * 80;
    }
    if (!destinations.size) host.y = 80 + row++ * 80;
    for (const f of flows) {
      for (const node of [host, listener, nginx, root]) {
        node.hits += f.requests;
        node.bytes += f.bytes;
      }
      const dest = byId.get(site.id + "|" + f.upstream.split(", ").at(-1));
      if (dest) {
        dest.hits += f.requests;
        dest.bytes += f.bytes;
      }
    }
  }
  const children = new Map<Node, Node[]>();
  for (const n of nodes)
    if (n.parent) {
      const list = children.get(n.parent) ?? [];
      list.push(n);
      children.set(n.parent, list);
    }
  for (let i = nodes.length - 1; i >= 0; i--) {
    const n = nodes[i],
      list = children.get(n);
    if (list) n.y = list.reduce((s, c) => s + c.y, 0) / list.length;
    else if (!n.y) n.y = 100;
  }
  return { nodes, height: Math.max(240, row * 80 + 65) };
});
function edge(n: Node) {
  if (!n.parent) return "";
  const { parent: p } = n,
    mid = (n.x + p.x) / 2;
  return `M ${p.x} ${p.y} C ${mid} ${p.y}, ${mid} ${n.y}, ${n.x} ${n.y}`;
}
const expanded = ref(false);
function exitExpanded(event: KeyboardEvent) {
  if (event.key === "Escape") expanded.value = false;
}
const detail = computed(
  () =>
    graph.value.nodes.find((n) => n.id === selected.value) ??
    graph.value.nodes[1],
);
function label(n: Node) {
  return mask.value && n.col >= 3
    ? names[n.col].split(" / ")[0] +
        " " +
        (graph.value.nodes.filter((other) => other.col === n.col).indexOf(n) +
          1)
    : n.label;
}
let timer: ReturnType<typeof setTimeout>,
  stopped = false;
async function poll() {
  try {
    const next = await request<Frame>("/topology");
    if (!stopped && !replay.value)
      frames.value = [...frames.value.slice(-29), next];
    error.value = "";
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    if (!stopped) timer = setTimeout(poll, 2000);
  }
}
function toggleReplay() {
  index.value = Math.max(0, frames.value.length - 1);
  replay.value = !replay.value;
}
onMounted(() => {
  window.addEventListener("keydown", exitExpanded);
  poll();
});
onBeforeUnmount(() => {
  window.removeEventListener("keydown", exitExpanded);
  stopped = true;
  clearTimeout(timer);
});
</script>
<template>
  <section class="card topology" :class="{ expanded }">
    <header>
      <h2>网络拓扑</h2>
      <input
        class="input"
        v-model="search"
        aria-label="搜索拓扑"
        placeholder="搜索域名、监听或后端"
      />
    </header>
    <div class="tools">
      <label
        >活动访客
        <AppSelect class="select" v-model="ip" searchable
          ><option value="">全部 IP</option>
          <option v-for="(v, i) in visitors" :key="v" :value="v">
            {{ mask ? "访客 " + (i + 1) : v }}
          </option></AppSelect
        ></label
      >
      <button class="button" :aria-pressed="focus" @click="focus = !focus">
        聚焦活动</button
      ><button class="button" :aria-pressed="mask" @click="mask = !mask">
        打码
      </button>
      <button
        class="button"
        :disabled="!frames.length"
        :aria-pressed="replay"
        @click="toggleReplay"
      >
        {{ replay ? "返回实时" : "回放" }}
      </button>
      <button
        class="button"
        :aria-expanded="showFilter"
        @click="showFilter = !showFilter"
      >
        筛选</button
      ><AppSelect
        class="select"
        v-if="showFilter"
        aria-label="筛选监听"
        v-model="listenerFilter"
        ><option value="">全部监听</option>
        <option v-for="l in listeners" :key="l" :value="l">
          {{ l }}
        </option></AppSelect
      >
      <AppSelect class="select" aria-label="拓扑布局" v-model="layout"
        ><option value="tree">树形</option>
        <option value="list">列表</option></AppSelect
      >
    </div>
    <div v-if="replay" class="tools">
      <label
        >本页回放
        <input
          type="range"
          v-model.number="index"
          min="0"
          :max="frames.length - 1" /></label
      >{{ new Date((frame?.time ?? 0) * 1000).toLocaleTimeString() }}
    </div>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-else-if="frame && !frame.logging">
      访问日志已关闭，仅显示已应用的代理配置。
    </p>
    <p v-if="frame?.limited">当前流量超过采样容量，显示最近 8192 条请求。</p>
    <div class="stats">
      <strong>{{ (graph.nodes[0].hits / 60).toFixed(2) }} 次/秒</strong
      ><span>{{ (graph.nodes[0].bytes / 61440).toFixed(2) }} KiB/s</span
      ><span v-if="frame?.connections != null"
        >{{ frame.connections }} 个活动连接</span
      ><span>{{ visitors.length }} 个活动 IP</span><span>近 60 秒</span>
    </div>
    <div
      v-if="layout === 'tree'"
      class="plot"
      tabindex="0"
      aria-label="流量拓扑画布"
    >
      <svg
        :viewBox="'0 0 1000 ' + graph.height"
        :style="{ width: 100 * zoom + '%', minWidth: 760 * zoom + 'px' }"
      >
        <text v-for="(name, i) in names" :key="name" :x="xs[i]" y="25">
          {{ name }}
        </text>
        <template v-for="n in graph.nodes" :key="n.id">
          <path v-if="n.parent" :class="{ hot: n.hits }" :d="edge(n)" />
        </template>
        <g
          v-for="n in graph.nodes"
          :key="n.id"
          :transform="'translate(' + n.x + ' ' + n.y + ')'"
          tabindex="0"
          role="button"
          :aria-label="label(n)"
          :class="{ hot: n.hits, chosen: detail?.id === n.id }"
          @click="selected = n.id"
          @keydown.enter="selected = n.id"
          @keydown.space.prevent="selected = n.id"
        >
          <title>{{ label(n) }}</title>
          <circle r="20" />
          <component :is="icons[n.col]" x="-11" y="-11" :size="22" />
          <text y="38">
            {{ label(n).length > 31 ? label(n).slice(0, 30) + "…" : label(n) }}
          </text>
        </g>
        <text v-if="graph.nodes.length === 2" x="600" y="110">
          {{ focus || search ? "没有匹配的站点" : "暂无已应用的站点监听" }}
        </text>
      </svg>
    </div>
    <div v-else class="tools">
      <button
        v-for="n in graph.nodes"
        :key="n.id"
        class="button"
        @click="selected = n.id"
      >
        {{ label(n) }}
      </button>
    </div>
    <footer v-if="detail">
      <div>
        <strong>{{ label(detail) }}</strong> ·
        {{ (detail.hits / 60).toFixed(2) }} 次/秒 ·
        {{ detail.hits }} 个已完成请求 ·
        {{ (detail.bytes / 1024).toFixed(1) }} KiB
      </div>
      <div v-if="layout === 'tree'">
        <button
          class="button"
          aria-label="缩小"
          @click="zoom = Math.max(0.75, zoom - 0.25)"
        >
          −</button
        ><button class="button" @click="zoom = 1">{{ zoom * 100 }}%</button
        ><button
          class="button"
          aria-label="放大"
          @click="zoom = Math.min(3, zoom + 0.25)"
        >
          ＋
        </button>
        <button class="button" @click="expanded = !expanded">
          {{ expanded ? "退出全屏" : "全屏" }}
        </button>
      </div>
    </footer>
  </section>
</template>
<style scoped>
.tools .select {
  width: auto;
  max-width: 100%;
}

.topology {
  min-width: 0;
  padding: clamp(12px, 2vw, 24px);
}
.topology header,
.topology footer,
.tools,
.stats,
.tools label {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.topology header,
.topology footer {
  justify-content: space-between;
}
.topology h2 {
  margin: 0;
}
.topology header input {
  width: min(100%, 270px);
}
.tools,
.stats,
footer {
  margin-top: 16px;
}
.tools label {
  font-weight: 600;
}
.tools [aria-pressed="true"] {
  color: var(--accent-dark);
  background: var(--accent-soft);
}
.stats span,
.stats strong {
  font-size: 14px;
}
.plot {
  overflow: auto;
  max-height: 62vh;
  margin-top: 14px;
  background-color: var(--surface-soft);
  background-image: radial-gradient(var(--line-strong) 1px, transparent 1px);
  background-size: 16px 16px;
  border: 1px solid var(--line);
  border-radius: 10px;
}
.plot > svg {
  display: block;
}
.plot text {
  text-anchor: middle;
  fill: var(--text);
  font: 14px system-ui;
}
.plot > svg > path {
  fill: none;
  stroke: var(--line-strong);
  stroke-width: 2;
}
.plot g {
  cursor: pointer;
  color: var(--text-muted);
}
.plot circle {
  fill: var(--surface);
  stroke: currentColor;
  stroke-width: 2;
}
.plot path.hot {
  stroke: var(--accent);
}
.plot g.hot {
  color: var(--accent-dark);
}
.plot .chosen circle {
  stroke-width: 4;
}
.plot g:focus-visible circle {
  stroke: var(--blue);
}
.topology.expanded {
  position: fixed;
  inset: 8px;
  z-index: 100;
  overflow: auto;
  background: var(--surface-solid);
}
.topology.expanded .plot {
  max-height: calc(100dvh - 220px);
}
.stats span:not(:last-child),
.stats strong {
  background: var(--surface-soft);
  padding: 4px 10px;
  border-radius: 20px;
}
.topology footer {
  border-top: 1px solid var(--line);
  padding-top: 12px;
}
.topology footer > div:last-child {
  display: flex;
  gap: 4px;
}

.topology footer {
  font-size: 14px;
}
</style>
