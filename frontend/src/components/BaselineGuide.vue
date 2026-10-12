<script setup>
import { computed } from "vue";
import { Layers, Plus, Check, FlaskConical } from "lucide-vue-next";
const props = defineProps({ pools: Array, cats: Array, canWrite: Boolean });
const emit = defineEmits(["record", "collect"]);
const progress = computed(() =>
  props.cats
    .filter((c) => !c.archived)
    .map((c) => {
      const profiles = (props.pools || []).flatMap((p) =>
        p.profiles
          .filter((f) => f.cat_id === c.id)
          .map((f) => ({ ...f, kind: p.kind })),
      );
      const urine = profiles.filter((f) => f.kind === "urine"),
        stool = profiles.filter((f) => f.kind === "stool");
      const days = (xs) => (xs.length ? Math.min(...xs.map((f) => f.days)) : 0);
      return {
        cat: c,
        urine: days(urine),
        stool: days(stool),
        ready: profiles.length > 0 && profiles.every((f) => f.ready),
      };
    }),
);
</script>
<template>
  <section class="panel baseline-guide">
    <div class="panel-heading">
      <div>
        <h3><FlaskConical :size="17" />先认识每只猫的正常样子</h3>
        <p>
          前7天建议单猫隔离采样，保持同一猫砂配置和称量方式。尿团、大便分别建基线，之后共盆采集无需归属。
        </p>
      </div>
      <button
        v-if="canWrite"
        class="btn secondary small"
        @click="emit('record')"
      >
        <Plus :size="15" />记录隔离基线
      </button>
    </div>
    <div class="baseline-progress" v-if="progress.length">
      <article v-for="p in progress" :key="p.cat.id">
        <strong
          >{{ p.cat.name
          }}<span class="pill" :class="p.ready ? 'sage' : 'amber'">{{
            p.ready ? "基线可用" : "建立基线中"
          }}</span></strong
        >
        <div>
          <span>尿团 {{ p.urine }} / 7天</span
          ><span>大便 {{ p.stool }} / 7天</span>
        </div>
        <div class="shared-meter">
          <span
            :style="{ width: (Math.min(p.urine, p.stool) / 7) * 100 + '%' }"
          />
        </div>
      </article>
    </div>
    <p v-else class="baseline-empty">
      先添加猫咪、猫砂配置和猫砂盆，就可以开始7天基线采样。
    </p>
    <div class="baseline-footer">
      <Layers :size="16" /><span
        >共享阶段先检测异常，再比较“哪只猫的基线簇改变”最能解释整批数据；证据不足时只报警，不强行定位。</span
      ><button class="text-btn" v-if="canWrite" @click="emit('collect')">
        开始共享采集<Plus :size="14" />
      </button>
    </div>
  </section>
</template>
