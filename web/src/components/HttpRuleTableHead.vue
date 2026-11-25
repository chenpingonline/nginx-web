<script lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";

type Boundary = "name" | "entrance";
const columns = [{ key: "name", label: "名称" }, { key: "entrance", label: "入口" }] as const;
const storageKey = "nginx-web.http-rule-column-boundaries";
const preferences = reactive<Partial<Record<Boundary, number>>>({});
try {
  const stored = JSON.parse(localStorage.getItem(storageKey) || "{}");
  for (const { key } of columns) {
    if (Number.isFinite(stored?.[key]) && stored[key] >= 0 && stored[key] <= 1) preferences[key] = stored[key];
  }
} catch { /* Browser storage may be unavailable. */ }

function savePreferences() {
  try { localStorage.setItem(storageKey, JSON.stringify(preferences)); } catch { /* Session values still apply. */ }
}

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));
</script>

<script setup lang="ts">
const row = ref<HTMLTableRowElement>();
const total = ref(0);
const available = ref(0);
let observer: ResizeObserver | undefined;
onMounted(() => {
  const header = row.value!;
  observer = new ResizeObserver(() => {
    const width = header.getBoundingClientRect().width;
    if (!width) return;
    total.value = width;
    available.value = width - header.cells[0].getBoundingClientRect().width - header.cells[4].getBoundingClientRect().width;
  });
  observer.observe(header);
});
onBeforeUnmount(() => observer?.disconnect());

const minName = 70;
const minEntrance = 130;
const minTarget = 100;
const end = computed(() => clamp(preferences.entrance == null ? 110 + total.value * 0.34 : preferences.entrance * available.value,
  minName + minEntrance, available.value - minTarget));
const name = computed(() => clamp(preferences.name == null ? 110 : preferences.name * available.value, minName, end.value - minEntrance));
const widths = computed(() => ({ name: name.value, entrance: end.value - name.value }));

function position(key: Boundary) {
  return key === "name" ? name.value : end.value;
}
function adjust(key: Boundary, position: number) {
  if (!available.value) return;
  const next = key === "name"
    ? clamp(position, minName, end.value - minEntrance)
    : clamp(position, name.value + minEntrance, available.value - minTarget);
  // Keep the other divider still even if the viewport previously constrained it.
  const other = key === "name" ? "entrance" : "name";
  if (preferences[other] != null) preferences[other] = (other === "name" ? name.value : end.value) / available.value;
  preferences[key] = next / available.value;
}

let drag: { id: number; key: Boundary; x: number; start: number } | undefined;
function startDrag(event: PointerEvent, key: Boundary) {
  if (!event.isPrimary || event.button !== 0 || drag) return;
  const handle = event.currentTarget as HTMLElement;
  handle.setPointerCapture(event.pointerId);
  handle.focus({ preventScroll: true });
  drag = { id: event.pointerId, key, x: event.clientX, start: position(key) };
}
function moveDrag(event: PointerEvent) {
  if (!drag || drag.id !== event.pointerId) return;
  adjust(drag.key, drag.start + event.clientX - drag.x);
}
function endDrag(event: PointerEvent) {
  if (drag?.id !== event.pointerId) return;
  drag = undefined;
  savePreferences();
}
function step(key: Boundary, delta: number) {
  adjust(key, position(key) + delta);
  savePreferences();
}
</script>

<template>
  <thead>
    <tr ref="row">
      <th scope="col">状态</th>
      <th v-for="column in columns" :key="column.key" scope="col" class="resizable-heading"
        :style="available ? { width: widths[column.key] + 'px' } : undefined">
        {{ column.label }}
        <span class="rule-column-divider" role="separator" tabindex="0" aria-orientation="vertical"
          :aria-label="'调整' + column.label + '列宽'" :title="'拖动调整' + column.label + '列宽'"
          :aria-valuemin="column.key === 'name' ? minName : minEntrance"
          :aria-valuemax="Math.round(column.key === 'name' ? end - minEntrance : available - name - minTarget)"
          :aria-valuenow="Math.round(widths[column.key])" :aria-valuetext="Math.round(widths[column.key]) + ' 像素'"
          @pointerdown.prevent="startDrag($event, column.key)" @pointermove="moveDrag" @lostpointercapture="endDrag"
          @keydown.left.prevent="step(column.key, -8)" @keydown.right.prevent="step(column.key, 8)" />
      </th>
      <th scope="col">转发服务</th>
      <th scope="col" aria-label="操作"></th>
    </tr>
  </thead>
</template>

<style scoped>
.resizable-heading { position: relative; }
.rule-column-divider {
  position: absolute;
  z-index: 1;
  inset: 0 -6px 0 auto;
  width: 12px;
  display: grid;
  place-items: center;
  cursor: col-resize;
  touch-action: none;
  user-select: none;
}
.rule-column-divider::after { content: ""; width: 2px; height: 48%; border-radius: 2px; background: var(--line); }
.rule-column-divider:hover::after, .rule-column-divider:focus-visible::after, .rule-column-divider:active::after { background: var(--accent); }
.rule-column-divider:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
</style>
