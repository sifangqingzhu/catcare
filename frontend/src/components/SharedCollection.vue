<script setup>
import { ref, computed } from "vue";
import { Plus, X, Trash2, Check, LoaderCircle, Layers } from "lucide-vue-next";
import { api } from "../api";
import SamplingDetails from './SamplingDetails.vue';
import { samplingDefaults, samplingPayload } from '../sampling';
const props = defineProps({
  tableId: Number,
  boxes: Array,
  litters: Array,
  pools: Array,
  cats: Array,
});
const emit = defineEmits(["close", "saved", "baseline"]);
const boxId = ref(props.boxes.find((b) => !b.archived)?.id || ""),
  notes = ref(""),
  busy = ref(false),
  error = ref("");
const now = new Date();
const occurredAt = ref(
  new Date(now.getTime() - now.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16),
);
let sequence = 0;
const newItem = () => ({
  ...samplingDefaults(),
  key: ++sequence,
  kind: "urine",
  grams: "",
  diameter_cm: "",
  looseness: 2,
  moisture: 3,
  shape: "formed",
  size: 3,
  blood: false,
});
const items = ref([newItem()]);
const conditions = ref(samplingDefaults());
const selected = computed(() =>
  props.boxes.find((b) => b.id === Number(boxId.value)),
);
const litter = computed(() =>
  props.litters.find((l) => l.id === selected.value?.litter_id),
);
const baselineStatus = computed(() => {
  const box = selected.value,
    l = litter.value;
  if (!box || !l) return { ready: false, text: "先选择猫砂盆" };
  const kinds = [...new Set(items.value.map((item) => item.kind))],
    summaries = [];
  let ready = true;
  for (const id of box.cat_ids)
    for (const kind of kinds) {
      const p = (props.pools || [])
        .find(
          (p) =>
            p.box_id === box.id &&
            p.kind === kind &&
            p.litter_key === `${l.id}:v${l.version}`,
        )
        ?.profiles.find((f) => f.cat_id === id);
      if (!p?.ready || p.quality?.state !== 'ready') ready = false;
      summaries.push(
        `${props.cats?.find((c) => c.id === id)?.name || "猫咪"} ${kind === "urine" ? "尿团" : "大便"} ${p?.days || 0}/7天`,
      );
    }
  return { ready, text: summaries.join(" · ") };
});
const num = (v) => (v === "" ? null : Number(v));
async function save() {
  busy.value = true;
  error.value = "";
  try {
    const records = items.value.map((r) => ({
      ...samplingPayload({ ...r, last_cleaned_at: conditions.value.last_cleaned_at, last_cleaning_complete: conditions.value.last_cleaning_complete, collection_complete: conditions.value.collection_complete, litter_depth_cm: conditions.value.litter_depth_cm, mode: 'shared' }),
      mode: "shared",
      kind: r.kind,
      cat_id: null,
      box_id: Number(boxId.value),
      occurred_at: new Date(occurredAt.value).toISOString(),
      count: 1,
      grams: num(r.grams),
      diameter_cm: r.kind === "urine" ? num(r.diameter_cm) : null,
      looseness: r.kind === "urine" ? Number(r.looseness) : null,
      moisture: r.kind === "stool" ? Number(r.moisture) : null,
      shape: r.kind === "stool" ? r.shape : "",
      size: r.kind === "stool" ? Number(r.size) : null,
      body_weight_kg: null,
      blood: r.blood,
      straining: false,
      no_output: false,
      notes: notes.value,
    }));
    await api(`/tables/${props.tableId}/batches`, "POST", { items: records });
    emit("saved");
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <div class="modal-backdrop" @click.self="!busy && emit('close')">
    <section
      class="modal collection-modal"
      role="dialog"
      aria-modal="true"
      aria-label="共享盆逐块采集"
    >
      <div class="modal-heading">
        <div>
          <span class="eyebrow">SHARED BOX · COLLECT FIRST</span>
          <h2>共享盆，记下每一块</h2>
        </div>
        <button
          class="icon-btn"
          :disabled="busy"
          @click="emit('close')"
          aria-label="关闭采集"
        >
          <X :size="20" />
        </button>
      </div>
      <form class="modal-form" @submit.prevent="save">
        <div class="form-grid">
          <label
            >采集猫砂盆<select v-model.number="boxId" required>
              <option disabled value="">先配置猫砂盆</option>
              <option
                v-for="b in boxes.filter((b) => !b.archived)"
                :key="b.id"
                :value="b.id"
              >
                {{ b.name }}
              </option>
            </select></label
          ><label
            >采集时间<input type="datetime-local" v-model="occurredAt" required
          /></label>
        </div>
        <div class="form-note">
          <Layers :size="17" />
          <p>
            无需判断归属，逐块称量，碎片合并。正常与异常样本共同用于差异分析。
          </p>
        </div>
        <div
          class="collection-baseline-status"
          :class="{ ready: baselineStatus.ready }"
        >
          <div>
            <strong
              >7天基准 ·
              {{ baselineStatus.ready ? "已采满 · 定位待验证" : "尚未就绪" }}</strong
            ><span>{{ baselineStatus.text }}</span>
          </div>
          <button type="button" class="text-btn" @click="emit('baseline')">
            查看基准
          </button>
        </div>
        <p class="form-hint">
          {{ litter?.name || "未选择猫砂" }} · 等待
          {{ litter?.wait_minutes ?? "—" }} 分钟称量 · 本次
          {{ items.length }} 块
        </p>
        <details class="sampling-details collection-conditions">
          <summary>清理间隔与采集条件</summary>
          <div class="form-grid sampling-grid">
            <label>上次清理时间<input type="datetime-local" v-model="conditions.last_cleaned_at" @change="!conditions.last_cleaned_at && (conditions.last_cleaning_complete = false)" /></label>
            <label>铺砂厚度（cm）<input type="number" min="0.1" max="30" step="0.1" v-model="conditions.litter_depth_cm" /></label>
            <label class="check-label"><input type="checkbox" v-model="conditions.last_cleaning_complete" :disabled="!conditions.last_cleaned_at" />上次已彻底清理</label>
            <label class="check-label"><input type="checkbox" v-model="conditions.collection_complete" />本次已完整采集</label>
          </div>
          <p class="form-hint">确认上次彻底清理，才能限定本批样本的存放时间。不完整采集会限制单猫定位。</p>
        </details>
        <article
          class="collection-item"
          v-for="(item, index) in items"
          :key="item.key"
        >
          <div class="collection-item-heading">
            <strong>第 {{ index + 1 }} 块</strong
            ><select
              v-model="item.kind"
              :aria-label="'第' + (index + 1) + '块类型'"
            >
              <option value="urine">尿团</option>
              <option value="stool">大便</option></select
            ><button
              type="button"
              class="icon-btn danger"
              v-if="items.length > 1"
              @click="items.splice(index, 1)"
              aria-label="移除采集项"
            >
              <Trash2 :size="15" />
            </button>
          </div>
          <div
            class="form-grid collection-metrics"
            :class="item.kind === 'urine' ? 'urine-metrics' : 'stool-metrics'"
          >
            <label
              >重量（g）<input
                type="number"
                min="0.1"
                max="2000"
                step="0.1"
                v-model="item.grams"
                required
                placeholder="逐块称重" /></label
            ><template v-if="item.kind === 'urine'"
              ><label
                >最大直径（cm，可选）<input
                  type="number"
                  min="0.1"
                  max="50"
                  step="0.1"
                  v-model="item.diameter_cm" /></label
              ><label
                >松散度<select v-model.number="item.looseness">
                  <option
                    v-for="(text, n) in [
                      '紧实',
                      '较紧实',
                      '适中',
                      '偏松散',
                      '易碎散',
                    ]"
                    :key="n"
                    :value="n + 1"
                  >
                    {{ n + 1 }} · {{ text }}
                  </option>
                </select></label
              ></template
            ><template v-else
              ><label
                >干湿度<select v-model.number="item.moisture">
                  <option
                    v-for="(text, n) in [
                      '干硬',
                      '偏干',
                      '适中',
                      '偏湿',
                      '稀水',
                    ]"
                    :key="n"
                    :value="n + 1"
                  >
                    {{ n + 1 }} · {{ text }}
                  </option>
                </select></label
              ><label
                >形态<select v-model="item.shape">
                  <option value="pellet">颗粒状</option>
                  <option value="formed">成形条状</option>
                  <option value="soft">软条状</option>
                  <option value="paste">糊状</option>
                  <option value="watery">水样</option>
                </select></label
              ><label
                >大小<select v-model.number="item.size">
                  <option
                    v-for="(text, n) in [
                      '很小',
                      '偏小',
                      '中等',
                      '偏大',
                      '很大',
                    ]"
                    :key="n"
                    :value="n + 1"
                  >
                    {{ n + 1 }} · {{ text }}
                  </option>
                </select></label
              ></template
            >
          </div>
          <div class="collection-flags"><label>完整性<select v-model="item.integrity"><option value="complete">完整单块</option><option value="fragmented">碎裂</option><option value="adhered">疑似粘连</option><option value="mixed">混合</option><option value="unknown">无法判断</option></select></label><label class="check-label"><input type="checkbox" v-model="item.blood" />发现血迹</label></div>
          <SamplingDetails :sample="item" />
        </article>
        <button
          type="button"
          class="btn secondary add-item"
          :disabled="items.length >= 30"
          @click="items.push(newItem())"
        >
          <Plus :size="16" />再添加一块（最多30块）</button
        ><label
          >本次采集备注<textarea
            v-model="notes"
            rows="1"
            maxlength="500"
            placeholder="例如：今日第一次清理，共发现2团尿、1块大便"
          />
        </label>
        <div v-if="error" class="error" role="alert">{{ error }}</div>
        <div class="modal-footer">
          <button
            type="button"
            class="btn secondary"
            :disabled="busy"
            @click="emit('close')"
          >
            取消</button
          ><button class="btn primary" :disabled="busy || !selected">
            <LoaderCircle v-if="busy" :size="16" class="spin" /><Check
              v-else
              :size="16"
            />保存整次采集
          </button>
        </div>
      </form>
    </section>
  </div>
</template>
