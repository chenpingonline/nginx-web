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
const metric = ref<"hits" | "bytes">("hits");
const sankey = computed(() => layout.value === "sankey");
const listeners = computed(() => [
  ...new Set(frame.value?.sites.map((s) => s.listener)),
]);
const visitors = computed(() =>
  [...new Set(frame.value?.flows.map((f) => f.client).filter(Boolean))].sort(),
);
const icons = [PhGlobe, PhHardDrives, PhPlugsConnected, PhBrowser, PhDatabase];
const names = ["互联网", "Nginx", "监听", "站点 / 动作", "后端 / 去向"];
const xs = [55, 155, 325, 570, 855];
interface Node {
  id: string;
  label: string;
  col: number;
  x: number;
  y: number;
  parent?: Node;
  hits: number;
  bytes: number;
  clients: Set<string>;
  missingUpstream?: boolean;
}
const widths = [46, 46, 188, 238, 250];
const graph = computed(() => {
  const nodes: Node[] = [],
    byId = new Map<string, Node>();
  function put(id: string, label: string, col: number, parent?: Node) {
    let n = byId.get(id);
    if (!n) {
      n = {
        id,
        label,
        col,
        x: xs[col],
        y: 0,
        hits: 0,
        bytes: 0,
        clients: new Set(),
        parent,
      };
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
  // Keep each listener’s branches together so their cards cannot overlap.
  const sites = [...(frame.value?.sites ?? [])].sort((a, b) =>
    a.listener.localeCompare(b.listener),
  );
  for (const site of sites) {
    if (listenerFilter.value && site.listener !== listenerFilter.value)
      continue;
    const flows = flowsBySite.get(site.id) ?? [];
    const destinations = new Set(site.targets ?? []);
    const outcomes = flows.map((flow) => {
      // Nginx separates retries with commas and upstream groups with spaced colons.
      const attempts = flow.upstream
        .split(/,|\s+:\s+/)
        .map((value) => value.trim())
        .filter((value) => value && value !== "-");
      for (const target of attempts) destinations.add(target);
      return { flow, target: attempts.at(-1) };
    });
    const hasMissing = outcomes.some((outcome) => !outcome.target);
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
      dest.y = 72 + row++ * 64;
    }
    const missing = hasMissing
      ? put("missing-upstream|" + site.id, "未记录到上游", 4, host)
      : undefined;
    if (missing) {
      missing.missingUpstream = true;
      missing.y = 72 + row++ * 64;
    }
    if (!destinations.size && !missing) host.y = 72 + row++ * 64;
    for (const { flow: f, target } of outcomes) {
      for (const node of [host, listener, nginx, root]) {
        node.hits += f.requests;
        node.bytes += f.bytes;
        if (f.client) node.clients.add(f.client);
      }
      const dest = target ? byId.get(site.id + "|" + target) : missing;
      if (dest) {
        dest.hits += f.requests;
        dest.bytes += f.bytes;
        if (f.client) dest.clients.add(f.client);
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
  root.y = nginx.y = Math.min(nginx.y, 136);
  return { nodes, height: Math.max(210, row * 64 + 55) };
});
function edge(n: Node) {
  if (!n.parent) return "";
  const { parent: p } = n,
    start = p.x + widths[p.col] / 2,
    end = n.x - widths[n.col] / 2,
    mid = (start + end) / 2;
  return `M ${start} ${p.y} C ${mid} ${p.y}, ${mid} ${n.y}, ${end} ${n.y}`;
}
// Stack each child's share along the parent's outgoing edge on one common scale.
const bands = computed(() => {
  const field = metric.value;
  const peak = Math.max(
    1,
    ...graph.value.nodes.filter((n) => n.col === 2).map((n) => n[field]),
  );
  const used = new Map<Node, number>();
  const paths = [];
  for (const n of graph.value.nodes) {
    const parent = n.parent;
    if (!parent || n.col < 3 || !n[field]) continue;
    const width = (40 * n[field]) / peak;
    const offset = used.get(parent) ?? 0;
    const startY = parent.y - (20 * parent[field]) / peak + offset + width / 2;
    used.set(parent, offset + width);
    const start = parent.x + widths[parent.col] / 2;
    const end = n.x - widths[n.col] / 2;
    const mid = (start + end) / 2;
    const path = `M ${start} ${startY} C ${mid} ${startY}, ${mid} ${n.y}, ${end} ${n.y}`;
    paths.push({ node: n, width, path });
  }
  return paths;
});
const visibleNodes = computed(() =>
  sankey.value
    ? graph.value.nodes.filter((n) => n.col >= 2)
    : graph.value.nodes,
);
const expanded = ref(false);
function exitExpanded(event: KeyboardEvent) {
  if (event.key === "Escape") expanded.value = false;
}
const detail = computed(
  () =>
    graph.value.nodes.find((n) => n.id === selected.value) ??
    graph.value.nodes[1],
);
const related = computed(() =>
  graph.value.nodes.filter((n) => n.parent === detail.value),
);
function traffic(bytes: number) {
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + " MiB";
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + " KiB";
  return bytes + " B";
}
function label(n: Node) {
  return mask.value && n.col >= 3 && !n.missingUpstream
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
  <section aria-label="网络拓扑" class="card topology" :class="{ expanded }">
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
        ><option value="tree">链路树</option>
        <option value="sankey">流量桑基</option>
        <option value="list">列表</option></AppSelect
      >
      <div
        v-if="sankey"
        class="metric-switch"
        role="group"
        aria-label="流量带宽度"
      >
        <button
          class="button"
          :aria-pressed="metric === 'hits'"
          @click="metric = 'hits'"
        >
          请求数
        </button>
        <button
          class="button"
          :aria-pressed="metric === 'bytes'"
          @click="metric = 'bytes'"
        >
          响应流量
        </button>
      </div>
      <input
        class="input topology-search"
        v-model="search"
        aria-label="搜索拓扑"
        placeholder="搜索域名、监听或后端"
      />
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
    <div class="workspace">
      <div class="canvas-pane">
        <div
          v-if="layout !== 'list'"
          class="plot"
          tabindex="0"
          aria-label="流量拓扑画布"
        >
          <svg
            :viewBox="(sankey ? '210 0 790 ' : '0 0 1000 ') + graph.height"
            :style="{
              width: 75 * zoom + '%',
              minWidth: (sankey ? 510 : 637.5) * zoom + 'px',
            }"
          >
            <text
              v-for="(name, i) in names"
              v-show="!sankey || i >= 2"
              :key="name"
              :x="xs[i]"
              y="24"
              class="lane"
            >
              {{ name }}
            </text>
            <template v-if="sankey">
              <path
                v-for="band in bands"
                :key="band.node.id"
                :d="band.path"
                class="flow-band"
                :class="{ unattributed: band.node.missingUpstream }"
                :style="{ strokeWidth: band.width }"
              >
                <title>
                  {{ label(band.node) }} ·
                  {{
                    metric === "hits"
                      ? band.node.hits + " 次请求"
                      : traffic(band.node.bytes)
                  }}
                </title>
              </path>
            </template>
            <template v-for="n in sankey ? [] : graph.nodes" :key="n.id">
              <path v-if="n.parent" :class="{ hot: n.hits }" :d="edge(n)" />
            </template>
            <g
              v-for="n in visibleNodes"
              :key="n.id"
              :transform="'translate(' + n.x + ' ' + n.y + ')'"
              tabindex="0"
              role="button"
              :aria-label="label(n)"
              :aria-pressed="detail.id === n.id"
              :class="{ hot: n.hits, chosen: detail.id === n.id }"
              @click="selected = n.id"
              @keydown.enter="selected = n.id"
              @keydown.space.prevent="selected = n.id"
            >
              <title>{{ label(n) }}</title>
              <template v-if="n.col < 2">
                <circle class="node-focus" r="27" />
                <circle class="node-face" r="23" />
                <component :is="icons[n.col]" x="-11" y="-11" :size="22" />
                <text y="42">{{ label(n) }}</text>
              </template>
              <template v-else>
                <rect
                  class="node-focus"
                  :x="-widths[n.col] / 2 - 3"
                  y="-25"
                  :width="widths[n.col] + 6"
                  height="50"
                  rx="11"
                />
                <rect
                  class="node-face"
                  :x="-widths[n.col] / 2"
                  y="-22"
                  :width="widths[n.col]"
                  height="44"
                  rx="8"
                />
                <component
                  :is="icons[n.col]"
                  :x="-widths[n.col] / 2 + 11"
                  y="-9"
                  :size="18"
                />
                <foreignObject
                  :x="-widths[n.col] / 2 + 38"
                  y="-21"
                  :width="widths[n.col] - 48"
                  height="42"
                >
                  <div class="node-copy">
                    <strong>{{ label(n) }}</strong
                    ><small>{{ n.hits }} 次请求 · {{ traffic(n.bytes) }}</small>
                  </div>
                </foreignObject>
              </template>
            </g>
            <text v-if="graph.nodes.length === 2" x="565" y="101">
              {{
                focus || search || listenerFilter
                  ? "没有匹配的站点"
                  : "暂无已应用的站点监听"
              }}
            </text>
          </svg>
        </div>
        <div v-else class="node-list">
          <button
            v-for="n in graph.nodes"
            :key="n.id"
            class="list-row"
            :aria-pressed="detail.id === n.id"
            @click="selected = n.id"
          >
            <component :is="icons[n.col]" :size="17" /><strong>{{
              label(n)
            }}</strong
            ><span>{{ n.hits }} 次请求</span>
          </button>
        </div>
        <div v-if="layout !== 'list'" class="canvas-controls">
          <button
            class="button"
            aria-label="缩小"
            :disabled="zoom <= 0.25"
            @click="zoom -= 0.25"
          >
            −
          </button>
          <button class="button" title="重置缩放" @click="zoom = 1">
            {{ zoom * 100 }}%
          </button>
          <button
            class="button"
            aria-label="放大"
            :disabled="zoom >= 4"
            @click="zoom += 0.25"
          >
            ＋
          </button>
          <button class="button" @click="expanded = !expanded">
            {{ expanded ? "退出全屏" : "全屏" }}
          </button>
        </div>
      </div>
      <aside class="inspector" aria-label="节点详情">
        <div class="detail-heading">
          <component :is="icons[detail.col]" :size="23" />
          <div>
            <small>{{ names[detail.col] }} · 节点详情</small
            ><strong>{{ label(detail) }}</strong>
          </div>
        </div>
        <div class="detail-section">
          <h3>近 60 秒</h3>
          <dl>
            <div>
              <dt>完成请求</dt>
              <dd>{{ detail.hits }}</dd>
            </div>
            <div>
              <dt>响应流量</dt>
              <dd>{{ traffic(detail.bytes) }}</dd>
            </div>
            <div>
              <dt>平均请求/秒</dt>
              <dd>{{ (detail.hits / 60).toFixed(2) }}</dd>
            </div>
            <div>
              <dt>活动 IP</dt>
              <dd>{{ detail.clients.size }}</dd>
            </div>
          </dl>
        </div>
        <div v-if="detail.clients.size" class="detail-section">
          <h3>活动访客</h3>
          <button
            v-for="client in detail.clients"
            :key="client"
            class="link-row"
            :aria-pressed="ip === client"
            title="按此访客筛选流量"
            @click="ip = ip === client ? '' : client"
          >
            {{ mask ? "访客 " + (visitors.indexOf(client) + 1) : client
            }}<span>{{ ip === client ? "✓" : "›" }}</span>
          </button>
        </div>
        <div v-if="detail.parent || related.length" class="detail-section">
          <h3 v-if="detail.parent">上游节点</h3>
          <button
            v-if="detail.parent"
            class="link-row"
            @click="selected = detail.parent.id"
          >
            {{ label(detail.parent) }}<span>›</span>
          </button>
          <h3 v-if="related.length">
            下游节点 <small>{{ related.length }}</small>
          </h3>
          <button
            v-for="n in related"
            :key="n.id"
            class="link-row"
            @click="selected = n.id"
          >
            {{ label(n) }}<span>›</span>
          </button>
        </div>
      </aside>
    </div>
  </section>
</template>
<style scoped>
.tools .select {
  width: auto;
  max-width: 100%;
}

.topology {
  min-width: 0;
  padding: 10px 14px;
}
.tools,
.stats,
.tools label {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.topology-search {
  flex: 0 1 240px;
  margin-inline-start: auto;
  min-width: 0;
}
@media (max-width: 650px) {
  .topology-search {
    flex-basis: 100%;
  }
}
.tools + .tools,
.stats {
  margin-top: 6px;
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
  max-height: max(240px, calc(100dvh - 175px));
  margin-top: 6px;
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
.plot > svg > path.flow-band {
  stroke: var(--accent);
  opacity: 0.48;
}
.plot > svg > path.unattributed {
  stroke: var(--text-muted);
}
.metric-switch {
  display: inline-flex;
  gap: 2px;
}
.plot g {
  cursor: pointer;
  color: var(--text-muted);
}
.plot .node-face {
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
.plot g.chosen {
  color: var(--accent-dark);
}
.plot .chosen .node-face {
  fill: var(--accent-soft);
}
.plot .chosen > text {
  font-weight: 600;
}
.plot g[role="button"]:focus {
  outline: none;
}
.node-focus {
  fill: none;
  stroke: transparent;
  stroke-width: 1.5;
  pointer-events: none;
}
.plot g:focus-visible > .node-focus {
  stroke: var(--accent-dark);
}
@media (forced-colors: active) {
  .plot g:focus-visible > .node-focus,
  .plot .chosen .node-face {
    stroke: Highlight;
  }
}
.topology.expanded {
  position: fixed;
  inset: 8px;
  z-index: 100;
  overflow: auto;
  background: var(--surface-solid);
}
.topology.expanded .plot {
  max-height: calc(100dvh - 140px);
}
.stats span:not(:last-child),
.stats strong {
  background: var(--surface-soft);
  padding: 2px 8px;
  border-radius: 20px;
}

.workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 228px;
  gap: 10px;
  margin-top: 10px;
  align-items: start;
}
.canvas-pane {
  min-width: 0;
  position: relative;
  background: var(--surface-soft);
  border: 1px solid var(--line);
  border-radius: 10px;
  overflow: hidden;
}
.plot {
  margin: 0;
  border: 0;
  border-radius: 0;
  min-height: clamp(240px, 52vh, 640px);
  background-image: radial-gradient(
    var(--line-strong) 0.7px,
    transparent 0.9px
  );
}
.plot .lane {
  font-size: 12px;
  fill: var(--text-muted);
}
.plot .node-face {
  stroke: var(--line-strong);
  stroke-width: 1.2;
}
.plot .hot .node-face {
  stroke: var(--accent);
}
.plot .chosen .node-face {
  stroke: var(--accent-dark);
  stroke-width: 2;
}
.plot > svg > path {
  stroke-width: 1.5;
}
.node-copy {
  height: 100%;
  display: grid;
  align-content: center;
  color: var(--text);
  font-size: 13px;
}
.node-copy strong,
.list-row strong {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.node-copy small,
.inspector small,
.inspector dt,
.list-row span {
  color: var(--text-muted);
  font-size: 12px;
}
.canvas-controls {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 3px;
  padding: 4px 8px;
  background: var(--surface);
  border-top: 1px solid var(--line);
}
.inspector {
  min-width: 0;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--surface);
  overflow: hidden;
}
.detail-heading {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px;
  background: var(--accent-soft);
  color: var(--accent-dark);
}
.detail-heading svg {
  flex: none;
}
.detail-heading strong {
  display: block;
  overflow-wrap: anywhere;
  color: var(--text);
}
.detail-section {
  padding: 12px;
  border-top: 1px solid var(--line);
  max-height: 48vh;
  overflow: auto;
}
.detail-section h3 {
  font-size: 13px;
  margin: 0 0 7px;
}
.detail-section button + h3 {
  margin-top: 12px;
}
.inspector dl {
  margin: 0;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 5px;
}
.inspector dl div {
  padding: 8px;
  background: var(--surface-soft);
  border-radius: 6px;
}
.inspector dd {
  margin: 2px 0 0;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}
.link-row,
.list-row {
  width: 100%;
  border: 0;
  border-bottom: 1px solid var(--line);
  background: var(--surface);
  color: var(--text);
  padding: 7px;
  display: flex;
  align-items: center;
  gap: 8px;
  text-align: left;
  overflow-wrap: anywhere;
  cursor: pointer;
}
.link-row span,
.list-row span {
  margin-left: auto;
  flex: none;
}
.link-row:hover,
.list-row:hover,
.list-row[aria-pressed="true"] {
  background: var(--accent-soft);
  color: var(--accent-dark);
}
.node-list {
  max-height: 75vh;
  overflow: auto;
}
@media (max-width: 1150px) {
  .workspace {
    grid-template-columns: minmax(0, 1fr);
  }
  .inspector {
    display: flex;
    flex-wrap: wrap;
  }
  .detail-section,
  .detail-heading {
    flex: 1 1 200px;
  }
}
@media (max-width: 650px) {
  .topology {
    padding: 8px;
  }
  .plot {
    min-height: 220px;
    max-height: 50vh;
  }
  .inspector {
    display: block;
  }
}
</style>
