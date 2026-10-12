<script setup>
import { computed } from "vue";
const props = defineProps({
  weights: Array,
  cats: Array,
  catId: [Number, String],
  days: { default: 30 },
});
const palette = {
  sage: "#527f6d",
  peach: "#c99872",
  lavender: "#9180b2",
  blue: "#6094a4",
};
const chart = computed(() => {
  const end = Date.now(),
    start = end - props.days * 86400000;
  const ws = (props.weights || []).filter(
    (w) =>
      new Date(w.occurred_at).getTime() >= start &&
      new Date(w.occurred_at).getTime() <= end &&
      (!props.catId || w.cat_id === Number(props.catId)),
  );
  const values = ws.map((w) => w.kg);
  const lo = values.length
      ? Math.floor((Math.min(...values) - 0.15) * 10) / 10
      : 0,
    hi = values.length ? Math.ceil((Math.max(...values) + 0.15) * 10) / 10 : 1;
  const x = (t) => 64 + ((new Date(t).getTime() - start) / (end - start)) * 624,
    y = (v) => 170 - ((v - lo) / (hi - lo)) * 138;
  return {
    empty: !ws.length,
    lo,
    hi,
    lines: (props.cats || [])
      .filter((c) => !props.catId || c.id === Number(props.catId))
      .map((c) => ({
        cat: c,
        color: palette[c.color] || palette.sage,
        points: ws
          .filter((w) => w.cat_id === c.id)
          .sort((a, b) => new Date(a.occurred_at) - new Date(b.occurred_at))
          .map((w) => ({ x: x(w.occurred_at), y: y(w.kg), w })),
      })),
    ticks: [0, 1, 2, 3].map((i) => ({
      y: 170 - i * 46,
      value: (lo + ((hi - lo) * i) / 3).toFixed(1),
    })),
    dates: [0, 1, 2, 3, 4].map((i) => ({
      x: 64 + i * 156,
      label: new Date(start + ((end - start) * i) / 4).toLocaleDateString(
        "zh-CN",
        { month: "2-digit", day: "2-digit" },
      ),
    })),
  };
});
</script>
<template>
  <div class="chart-empty" v-if="chart.empty">
    还没有体重记录，先给猫咪称一次体重吧。
  </div>
  <template v-else
    ><svg
      class="weight-chart"
      viewBox="0 0 730 210"
      role="img"
      aria-label="猫咪近30天体重趋势，单位千克"
    >
      <text x="12" y="16" class="axis-text">kg</text>
      <g v-for="tick in chart.ticks" :key="tick.y">
        <line
          x1="64"
          x2="688"
          :y1="tick.y"
          :y2="tick.y"
          stroke="#eceee9"
          stroke-dasharray="4 5"
        />
        <text x="12" :y="tick.y + 4" class="axis-text">{{ tick.value }}</text>
      </g>
      <g v-for="line in chart.lines" :key="line.cat.id">
        <polyline
          :points="line.points.map((p) => `${p.x},${p.y}`).join(' ')"
          fill="none"
          :stroke="line.color"
          stroke-width="2.5"
          stroke-linejoin="round"
        />
        <circle
          v-for="p in line.points"
          :key="p.w.id"
          :cx="p.x"
          :cy="p.y"
          r="3.5"
          :fill="line.color"
          stroke="white"
          stroke-width="1.5"
        >
          <title>
            {{ line.cat.name }} ·
            {{ new Date(p.w.occurred_at).toLocaleDateString() }} ·
            {{ p.w.kg }} kg
          </title>
        </circle>
      </g>
      <text
        v-for="date in chart.dates"
        :key="date.x"
        :x="date.x"
        y="203"
        text-anchor="middle"
        class="axis-text"
      >
        {{ date.label }}
      </text>
    </svg>
    <div class="chart-legend">
      <span
        v-for="line in chart.lines.filter((l) => l.points.length)"
        :key="line.cat.id"
        ><i :style="{ background: line.color }" />{{ line.cat.name }}</span
      >
    </div></template
  >
</template>
