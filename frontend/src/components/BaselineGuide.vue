<script setup>
import { computed, ref } from "vue";
import { Layers, Plus, FlaskConical } from "lucide-vue-next";
const props = defineProps({
  pools: Array,
  cats: Array,
  boxes: Array,
  litters: Array,
  records: Array,
  canWrite: Boolean,
  compact: Boolean,
  showDetails: Boolean,
  timezone: { type: String, default: "Asia/Tokyo" },
});
const emit = defineEmits(["record", "collect"]);
const currentPools = computed(() =>
  (props.pools || []).filter((pool) => {
    const box = props.boxes?.find((b) => b.id === pool.box_id),
      litter = props.litters?.find((l) => l.id === box?.litter_id);
    return (
      box &&
      !box.archived &&
      litter &&
      pool.litter_key === `${litter.id}:v${litter.version}`
    );
  }),
);
const dateKey = (time) => {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-CA", {
      timeZone: props.timezone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    })
      .formatToParts(new Date(time))
      .filter((p) => p.type !== "literal")
      .map((p) => [p.type, p.value]),
  );
  return `${parts.year}-${parts.month}-${parts.day}`;
};
const shortDate = (time) =>
  new Intl.DateTimeFormat("zh-CN", {
    timeZone: props.timezone,
    month: "2-digit",
    day: "2-digit",
  }).format(new Date(time));
const status = (f) =>
  f.ready
    ? "基准可用"
    : !f.samples
      ? "未开始"
      : new Date(f.end).getTime() <= Date.now()
        ? f.days < 7
          ? "缺少采样日"
          : "量化样本不足"
        : f.days >= 7
          ? "等待7天窗口结束"
          : "采样中";
const profiles = computed(() => {
  const unique = new Map();
  for (const pool of currentPools.value)
    for (const f of pool.profiles) {
      const key = `${f.cat_id}:${pool.kind}:${pool.litter_key}`;
      if (!unique.has(key))
        unique.set(key, { ...f, kind: pool.kind, litter_key: pool.litter_key });
    }
  return [...unique.values()].map((f) => {
    const samples = (props.records || []).filter(
      (r) =>
        r.mode === "baseline" &&
        r.cat_id === f.cat_id &&
        r.kind === f.kind &&
        r.litter_key === f.litter_key &&
        r.count === 1 &&
        !r.blood &&
        !r.straining &&
        !r.no_output &&
        f.start &&
        new Date(r.occurred_at) >= new Date(f.start) &&
        new Date(r.occurred_at) < new Date(f.end),
    );
    const dates = new Set(
      f.recorded_dates || samples.map((r) => dateKey(r.occurred_at)),
    );
    const days = Array.from({ length: 7 }, (_, i) => {
      const time = f.start
        ? new Date(f.start).getTime() + i * 86400000 + 12 * 3600000
        : null;
      return {
        label: time ? shortDate(time) : `第${i + 1}天`,
        recorded: time && dates.has(dateKey(time)),
      };
    });
    return {
      ...f,
      calendar: days,
      range: f.start
        ? `${shortDate(f.start)} — ${shortDate(new Date(f.end).getTime() - 1)}`
        : "尚未开始",
      status: status(f),
    };
  });
});
const progress = computed(() =>
  (props.cats || [])
    .filter((c) => !c.archived)
    .map((cat) => {
      const fs = profiles.value.filter((f) => f.cat_id === cat.id),
        urine = fs.filter((f) => f.kind === "urine"),
        stool = fs.filter((f) => f.kind === "stool");
      const summarize = (xs) => ({
        days: xs.length ? Math.min(...xs.map((f) => f.days)) : 0,
        samples: xs.reduce((n, f) => n + f.samples, 0),
        range:
          xs.length === 1
            ? xs[0].range
            : xs.length
              ? "多个猫砂版本分别统计"
              : "尚未开始",
      });
      return {
        cat,
        urine: summarize(urine),
        stool: summarize(stool),
        ready: fs.length > 0 && fs.every((f) => f.ready),
        started: fs.some((f) => f.samples > 0),
        incomplete: fs.some(
          (f) =>
            f.samples > 0 &&
            !f.ready &&
            new Date(f.end).getTime() <= Date.now(),
        ),
      };
    }),
);
const catName = (id) => props.cats.find((c) => c.id === id)?.name || "猫咪";
const detailCat = ref(0);
const detailId = computed(() =>
  progress.value.some((p) => p.cat.id === detailCat.value)
    ? detailCat.value
    : progress.value[0]?.cat.id,
);
const visibleProfiles = computed(() =>
  profiles.value.filter((f) => f.cat_id === detailId.value),
);
const configName = (key) => {
  const [id, version] = key.split(":");
  return `${props.litters.find((l) => l.id === Number(id))?.name || "猫砂配置"} · ${version}`;
};
const metric = (f, k, unit) =>
  f.metrics[k] ? `${f.metrics[k].median.toFixed(1)} ${unit}` : "样本不足";
</script>
<template>
  <section
    class="panel baseline-guide"
    :class="{ 'baseline-compact': compact }"
  >
    <div class="panel-heading">
      <div>
        <h3><FlaskConical :size="17" />7天单猫基准</h3>
        <p>
          先隔离记录，再共享观察。尿团、大便分别覆盖7个采样日；同一天多条记录仍算1天。
        </p>
      </div>
      <button
        v-if="canWrite"
        class="btn secondary small"
        @click="emit('record')"
      >
        <Plus :size="15" />记录隔离基准
      </button>
    </div>
    <div class="baseline-progress" v-if="progress.length">
      <article v-for="p in progress" :key="p.cat.id">
        <strong
          >{{ p.cat.name
          }}<span class="pill" :class="p.ready ? 'sage' : 'amber'">{{
            p.ready
              ? "基准可用"
              : p.incomplete
                ? "记录不完整"
                : p.started
                  ? "采样中"
                  : "未开始"
          }}</span
          ><button
            v-if="canWrite"
            class="text-btn baseline-cat-action"
            @click="emit('record', p.cat.id)"
          >
            记基准
          </button></strong
        >
        <div class="baseline-kind-progress">
          <div>
            <span
              >尿团 <b>{{ p.urine.days }} / 7天</b></span
            ><small>{{ p.urine.samples }} 条样本 · {{ p.urine.range }}</small>
          </div>
          <div>
            <span
              >大便 <b>{{ p.stool.days }} / 7天</b></span
            ><small>{{ p.stool.samples }} 条样本 · {{ p.stool.range }}</small>
          </div>
        </div>
        <div class="shared-meter">
          <span
            :style="{
              width: (Math.min(p.urine.days, p.stool.days) / 7) * 100 + '%',
            }"
          />
        </div>
      </article>
    </div>
    <p v-else class="baseline-empty">
      先添加猫咪、猫砂和猫砂盆，再开始隔离采样。
    </p>
    <details class="baseline-details" :open="showDetails">
      <summary>查看7天覆盖情况与基准数值</summary>
      <p class="baseline-rule">
        从第一条隔离样本所在日期开始，保留最初7天；需要完整覆盖和足够的量化样本才能就绪。更换猫砂版本后重新建立。共享记录不进入基准。
      </p>
      <div class="baseline-detail-tabs">
        <button
          v-for="p in progress"
          :key="p.cat.id"
          :class="{ active: detailId === p.cat.id }"
          @click="detailCat = p.cat.id"
          :aria-label="'查看' + p.cat.name + '基准'"
        >
          {{ p.cat.name }}
        </button>
      </div>
      <article
        class="baseline-profile-detail"
        v-for="f in visibleProfiles"
        :key="`${f.cat_id}:${f.kind}:${f.litter_key}`"
      >
        <div class="baseline-detail-heading">
          <strong
            >{{ catName(f.cat_id) }} ·
            {{ f.kind === "urine" ? "尿团" : "大便" }}</strong
          ><span>{{ configName(f.litter_key) }}</span
          ><span class="pill" :class="f.ready ? 'sage' : 'amber'">{{
            f.status
          }}</span>
        </div>
        <div class="baseline-calendar">
          <span
            v-for="(day, i) in f.calendar"
            :key="i"
            :class="{ recorded: day.recorded }"
            :title="day.recorded ? '有有效隔离样本' : '未记录有效隔离样本'"
            >{{ day.label
            }}<i>{{ day.recorded ? "已采样" : "待采样" }}</i></span
          >
        </div>
        <div class="baseline-values">
          <span
            >有效样本 <b>{{ f.samples }} 条</b></span
          ><span
            >典型重量 <b>{{ metric(f, "grams", "g") }}</b></span
          ><span v-if="f.kind === 'urine'"
            >典型直径 <b>{{ metric(f, "diameter", "cm") }}</b></span
          ><span v-else
            >典型干湿 <b>{{ metric(f, "moisture", "级") }}</b></span
          >
        </div>
        <p v-if="!f.ready && f.samples" class="baseline-rule">
          窗口：{{ f.range }}；目前 {{ f.days }} 个采样日，尚未满足全部条件。
        </p>
      </article>
      <p class="baseline-rule">
        典型数值取隔离样本的中位数，波动尺度采用
        MAD。它表示这只猫的历史样子，不是通用医学正常范围。
      </p>
    </details>
    <div v-if="!compact" class="baseline-footer">
      <Layers :size="16" /><span
        >共享后先报警，再用整批数据差异缩小异常猫的排查范围。</span
      ><button class="text-btn" v-if="canWrite" @click="emit('collect')">
        共享盆采集<Plus :size="14" />
      </button>
    </div>
  </section>
</template>
