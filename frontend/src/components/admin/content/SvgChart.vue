<template>
  <div ref="wrapEl" class="svg-chart" :style="{ height: height + 'px' }">
    <!-- 折线 / 柱状 -->
    <svg v-if="kind === 'line' || kind === 'bar' || kind === 'hbar'" :width="W" :height="height" class="svg-chart__svg" role="img" :aria-label="ariaLabel">
      <defs>
        <linearGradient v-for="(g, gi) in gradients" :key="gi" :id="g.id" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" :stop-color="g.color" stop-opacity="0.30" />
          <stop offset="100%" :stop-color="g.color" stop-opacity="0.02" />
        </linearGradient>
      </defs>

      <!-- 网格线 -->
      <g class="svg-chart__grid">
        <line v-for="t in yTicks" :key="'g' + t.v" :x1="pad.l" :x2="W - pad.r" :y1="t.y" :y2="t.y" />
      </g>
      <!-- Y 轴刻度文字 -->
      <g class="svg-chart__ytick">
        <text v-for="t in yTicks" :key="'y' + t.v" :x="pad.l - 10" :y="t.y + 4" text-anchor="end">{{ t.label }}</text>
      </g>

      <!-- 折线 -->
      <template v-if="kind === 'line'">
        <path v-for="(s, si) in series" :key="'a' + si" :d="s.areaPath" :fill="'url(#' + s.gradId + ')'" />
        <path v-for="(s, si) in series" :key="'l' + si" :d="s.linePath" fill="none" :stroke="s.color" stroke-width="2.2" stroke-linejoin="round" stroke-linecap="round" />
        <g v-if="activeIdx >= 0">
          <line class="svg-chart__cursor" :x1="colX(activeIdx)" :x2="colX(activeIdx)" :y1="pad.t" :y2="height - pad.b" />
          <circle v-for="(s, si) in series" :key="'c' + si" :cx="colX(activeIdx)" :cy="s.pts[activeIdx].y" r="4" :fill="s.color" stroke="#fff" stroke-width="1.5" />
        </g>
      </template>

      <!-- 竖向柱状 -->
      <template v-if="kind === 'bar'">
        <rect
          v-for="(b, bi) in bars"
          :key="'b' + bi"
          :x="b.x"
          :y="b.y"
          :width="b.w"
          :height="b.h"
          :rx="Math.min(6, b.w / 2)"
          :fill="b.color"
          :opacity="activeIdx >= 0 && activeIdx !== b.i ? 0.45 : 1"
        />
      </template>

      <!-- 横向条（TOP 榜） -->
      <template v-if="kind === 'hbar'">
        <g v-for="(r, ri) in hRows" :key="'h' + ri">
          <text class="svg-chart__hlabel" :x="4" :y="r.cy + 4">{{ r.label }}</text>
          <rect :x="pad.l" :y="r.y" :width="Math.max(2, r.w)" :height="r.h" rx="4" :fill="r.color" :opacity="activeIdx >= 0 && activeIdx !== ri ? 0.5 : 1" />
          <text class="svg-chart__hvalue" :x="pad.l + r.w + 8" :y="r.cy + 4">{{ r.valueLabel }}</text>
        </g>
      </template>

      <!-- X 轴文字 -->
      <g v-if="kind !== 'hbar'" class="svg-chart__xtick">
        <text v-for="t in xTicks" :key="'x' + t.i" :x="t.x" :y="height - 5" text-anchor="middle">{{ t.label }}</text>
      </g>

      <!-- 悬停热区 -->
      <rect
        v-for="(c, ci) in hoverCols"
        :key="'hv' + ci"
        class="svg-chart__hit"
        :x="c.x"
        :y="pad.t"
        :width="c.w"
        :height="height - pad.t - pad.b"
        @mouseenter="activeIdx = ci"
        @mouseleave="activeIdx = -1"
      />
    </svg>

    <!-- 环形图 -->
    <div v-else class="svg-chart__donut">
      <svg :width="Math.min(W, 220)" :height="donutSize" role="img" :aria-label="ariaLabel">
        <g :transform="'translate(' + Math.min(W, 220) / 2 + ',' + donutSize / 2 + ')'">
          <circle r="78" fill="none" stroke="#EEF1F6" :stroke-width="donutStroke" />
          <circle
            v-for="(d, di) in donutSegs"
            :key="'d' + di"
            r="78"
            fill="none"
            :stroke="d.color"
            :stroke-width="donutStroke"
            :stroke-dasharray="d.dash"
            :stroke-dashoffset="d.offset"
            transform="rotate(-90)"
            @mouseenter="activeIdx = di"
            @mouseleave="activeIdx = -1"
          />
          <text text-anchor="middle" y="-2" class="svg-chart__donut-num">{{ donutTotal }}</text>
          <text text-anchor="middle" y="16" class="svg-chart__donut-cap">{{ activeIdx >= 0 && donutSegs[activeIdx] ? donutSegs[activeIdx].label : '合计' }}</text>
        </g>
      </svg>
      <ul class="svg-chart__legend">
        <li v-for="(d, di) in donutSegs" :key="'lg' + di" :class="{ on: activeIdx === di }" @mouseenter="activeIdx = di" @mouseleave="activeIdx = -1">
          <i :style="{ background: d.color }" />
          <span class="svg-chart__legend-label">{{ d.label }}</span>
          <span class="svg-chart__legend-val">{{ d.value }}<em>{{ d.pct }}%</em></span>
        </li>
      </ul>
    </div>

    <!-- 折线/柱状的图例 -->
    <ul v-if="showLegend && kind !== 'doughnut' && legendItems.length > 1" class="svg-chart__legend svg-chart__legend--top">
      <li v-for="(l, li) in legendItems" :key="'lt' + li">
        <i :style="{ background: l.color }" />
        <span class="svg-chart__legend-label">{{ l.label }}</span>
      </li>
    </ul>

    <p v-if="isEmpty" class="svg-chart__empty">暂无数据</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";

type Dataset = {
  label?: string;
  data: number[];
  borderColor?: string;
  backgroundColor?: string | string[];
};

const props = withDefaults(
  defineProps<{
    kind: "line" | "bar" | "hbar" | "doughnut";
    data: { labels?: string[]; datasets?: Dataset[] };
    height?: number;
    showLegend?: boolean;
  }>(),
  { height: 260, showLegend: true },
);

const FALLBACK = ["#6366F1", "#F59E0B", "#2563EB", "#10B981", "#8B5CF6", "#EF4444", "#0EA5E9", "#EC4899"];

const wrapEl = ref<HTMLElement | null>(null);
const W = ref(640);
let ro: ResizeObserver | null = null;

onMounted(() => {
  if (!wrapEl.value) return;
  const apply = () => {
    const w = wrapEl.value ? wrapEl.value.clientWidth : 0;
    if (w > 40) W.value = w;
  };
  apply();
  ro = new ResizeObserver(apply);
  ro.observe(wrapEl.value);
});
onUnmounted(() => {
  ro?.disconnect();
  ro = null;
});

const labels = computed<string[]>(() => (props.data?.labels || []).map((l) => String(l)));
const datasets = computed<Dataset[]>(() => props.data?.datasets || []);
const isEmpty = computed(() =>
  !labels.value.length || !datasets.value.length || datasets.value.every((d) => !(d.data || []).some((n) => Number(n) > 0)),
);
const ariaLabel = computed(() => labels.value.slice(0, 4).join("、") + (labels.value.length > 4 ? " 等" : ""));

const colorOf = (d: Dataset, i: number) => {
  const own = Array.isArray(d.backgroundColor) ? d.backgroundColor[0] : d.backgroundColor;
  return String(d.borderColor || own || FALLBACK[i % FALLBACK.length]);
};

/** 顶部留白给图例 */
const legendH = computed(() => (props.showLegend && datasets.value.length > 1 && props.kind !== "doughnut" ? 22 : 0));
const hbarLabelW = computed(() => Math.min(230, Math.max(120, W.value * 0.34)));

const pad = computed(() => {
  if (props.kind === "hbar") return { l: hbarLabelW.value, r: 56, t: 10 + legendH.value, b: 8 };
  return { l: 46, r: 14, t: 10 + legendH.value, b: 26 };
});

const plotW = computed(() => Math.max(10, W.value - pad.value.l - pad.value.r));
const plotH = computed(() => Math.max(10, props.height - pad.value.t - pad.value.b));

function niceMax(v: number): number {
  if (!isFinite(v) || v <= 0) return 4;
  const exp = Math.pow(10, Math.floor(Math.log10(v)));
  const f = v / exp;
  const nf = f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10;
  return nf * exp;
}

const rawMax = computed(() => {
  let m = 0;
  datasets.value.forEach((d) => (d.data || []).forEach((n) => (m = Math.max(m, Number(n) || 0))));
  return m;
});
const yMax = computed(() => niceMax(rawMax.value));

const yTicks = computed(() => {
  const top = props.kind === "hbar" ? yMax.value : yMax.value;
  const steps = 4;
  const out: { v: number; y: number; label: string }[] = [];
  for (let i = 0; i <= steps; i++) {
    const v = (top / steps) * i;
    const y = pad.value.t + plotH.value - (v / top) * plotH.value;
    out.push({ v, y, label: fmtNum(v) });
  }
  return out;
});

function fmtNum(n: number): string {
  if (!isFinite(n)) return "0";
  if (Math.abs(n) >= 100000) return (n / 10000).toFixed(0) + "万";
  return Number.isInteger(n) ? String(n) : String(Number(n.toFixed(1)));
}

const colX = (i: number) => {
  const n = Math.max(1, labels.value.length);
  const step = plotW.value / n;
  return pad.value.l + step * i + step / 2;
};

const xTicks = computed(() => {
  const n = labels.value.length;
  if (!n) return [] as { i: number; x: number; label: string }[];
  const maxShow = Math.max(2, Math.floor(plotW.value / 78));
  const step = Math.max(1, Math.ceil(n / maxShow));
  const out: { i: number; x: number; label: string }[] = [];
  for (let i = 0; i < n; i += step) out.push({ i, x: colX(i), label: shortLabel(labels.value[i], step > 1 ? 8 : 10) });
  return out;
});

function shortLabel(s: string, max: number): string {
  if (!s) return "";
  return s.length > max ? s.slice(0, max) + "…" : s;
}

const yOf = (v: number) => pad.value.t + plotH.value - (Math.max(0, v) / yMax.value) * plotH.value;

const gradients = computed(() => datasets.value.map((d, i) => ({ id: "svgc-grad-" + uid() + "-" + i, color: colorOf(d, i) })));

const series = computed(() =>
  datasets.value.map((d, i) => {
    const pts = (d.data || []).map((n, idx) => ({ x: colX(idx), y: yOf(Number(n) || 0) }));
    let linePath = "";
    if (pts.length) {
      linePath = "M " + pts[0].x + " " + pts[0].y;
      for (let k = 1; k < pts.length; k++) {
        const p0 = pts[k - 1];
        const p1 = pts[k];
        const mx = (p0.x + p1.x) / 2;
        linePath += " C " + mx + " " + p0.y + ", " + mx + " " + p1.y + ", " + p1.x + " " + p1.y;
      }
    }
    const baseY = pad.value.t + plotH.value;
    const areaPath = pts.length
      ? linePath + " L " + pts[pts.length - 1].x + " " + baseY + " L " + pts[0].x + " " + baseY + " Z"
      : "";
    return { pts, linePath, areaPath, color: colorOf(d, i), gradId: gradients.value[i].id, label: d.label || "" };
  }),
);

const bars = computed(() => {
  const n = labels.value.length;
  const sCount = Math.max(1, datasets.value.length);
  const step = plotW.value / Math.max(1, n);
  const groupW = step * 0.68;
  const barW = Math.max(2, groupW / sCount - (sCount > 1 ? 3 : 0));
  const out: { i: number; x: number; y: number; h: number; w: number; color: string }[] = [];
  datasets.value.forEach((d, si) => {
    (d.data || []).forEach((v, idx) => {
      const val = Math.max(0, Number(v) || 0);
      const y = yOf(val);
      const x = pad.value.l + step * idx + (step - groupW) / 2 + si * (barW + 3);
      out.push({ i: idx, x, y, h: Math.max(1, pad.value.t + plotH.value - y), w: barW, color: colorOf(d, si) });
    });
  });
  return out;
});

const hRows = computed(() => {
  const n = labels.value.length;
  const rowH = plotH.value / Math.max(1, n);
  const d = datasets.value[0];
  const barH = Math.max(6, Math.min(20, rowH * 0.56));
  return (d?.data || []).map((v, i) => {
    const val = Math.max(0, Number(v) || 0);
    const w = (val / yMax.value) * plotW.value;
    return {
      cy: pad.value.t + rowH * i + rowH / 2,
      y: pad.value.t + rowH * i + (rowH - barH) / 2,
      h: barH,
      w,
      color: colorOf(d, i),
      label: shortLabel(labels.value[i] || "", Math.floor(hbarLabelW.value / 7.2)),
      valueLabel: fmtNum(val),
    };
  });
});

const hoverCols = computed(() => {
  const n = labels.value.length;
  const step = plotW.value / Math.max(1, n);
  return Array.from({ length: n }, (_, i) => ({ x: pad.value.l + step * i, w: step }));
});

const activeIdx = ref(-1);

const legendItems = computed(() => datasets.value.map((d, i) => ({ label: d.label || "序列 " + (i + 1), color: colorOf(d, i) })));

const donutSize = computed(() => Math.max(140, Math.min(200, props.height - 40)));
const donutStroke = 26;
const donutTotal = computed(() => (datasets.value[0]?.data || []).reduce((a, b) => a + (Number(b) || 0), 0));

const donutSegs = computed(() => {
  const d = datasets.value[0];
  const vals = (d?.data || []).map((n) => Math.max(0, Number(n) || 0));
  const total = vals.reduce((a, b) => a + b, 0) || 1;
  const C = 2 * Math.PI * 78;
  let acc = 0;
  return vals.map((v, i) => {
    const len = (v / total) * C;
    const seg = {
      label: labels.value[i] || "—",
      value: v,
      pct: Math.round((v / total) * 1000) / 10,
      color: Array.isArray(d?.backgroundColor) ? String((d!.backgroundColor as string[])[i % (d!.backgroundColor as string[]).length]) : colorOf(d || { data: [] }, i),
      dash: len + " " + Math.max(0, C - len),
      offset: -acc,
    };
    acc += len;
    return seg;
  });
});

let uidCounter = 0;
function uid() {
  uidCounter += 1;
  return uidCounter;
}
</script>

<style scoped>
.svg-chart {
  position: relative;
  width: 100%;
  min-height: 120px;
}
.svg-chart__svg {
  display: block;
  overflow: visible;
}
.svg-chart__grid line {
  stroke: #EDF0F5;
  stroke-width: 1;
}
.svg-chart__ytick text,
.svg-chart__xtick text {
  fill: #94A3B8;
  font-size: 11px;
  font-family: inherit;
}
.svg-chart__xtick text {
  font-variant-numeric: tabular-nums;
}
.svg-chart__hlabel {
  fill: #475569;
  font-size: 11.5px;
}
.svg-chart__hvalue {
  fill: #1E293B;
  font-size: 11.5px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.svg-chart__cursor {
  stroke: #CBD5E1;
  stroke-width: 1;
  stroke-dasharray: 3 3;
}
.svg-chart__hit {
  fill: transparent;
  cursor: crosshair;
}
.svg-chart__donut {
  display: flex;
  align-items: center;
  gap: 18px;
  height: 100%;
  flex-wrap: wrap;
}
.svg-chart__donut-num {
  fill: #0F172A;
  font-size: 20px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.svg-chart__donut-cap {
  fill: #94A3B8;
  font-size: 11px;
}
.svg-chart__legend {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 7px;
  min-width: 150px;
  flex: 1;
}
.svg-chart__legend--top {
  position: absolute;
  top: 0;
  left: 46px;
  flex-direction: row;
  gap: 16px;
  flex-wrap: wrap;
}
.svg-chart__legend li {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 12px;
  color: #475569;
  cursor: default;
  border-radius: 6px;
  padding: 1px 4px;
}
.svg-chart__legend li.on {
  background: #F1F5F9;
  color: #0F172A;
}
.svg-chart__legend i {
  width: 9px;
  height: 9px;
  border-radius: 3px;
  flex: none;
}
.svg-chart__legend-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.svg-chart__legend-val {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  color: #1E293B;
  white-space: nowrap;
}
.svg-chart__legend-val em {
  font-style: normal;
  font-weight: 500;
  color: #94A3B8;
  margin-left: 4px;
}
.svg-chart__empty {
  position: absolute;
  inset: 0;
  margin: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #A3AEC2;
  font-size: 13px;
}
</style>
