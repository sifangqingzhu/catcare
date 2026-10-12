<script setup>
import { computed } from 'vue';
const props=defineProps({pools:Array,boxes:Array,cats:Array});
const visible=computed(()=>(props.pools||[]).filter(p=>p.observations?.length));
const catName=id=>props.cats.find(c=>c.id===id)?.name||'猫咪';
const boxName=id=>props.boxes.find(b=>b.id===id)?.name||'历史猫砂盆';
const labels={healthy:'正常波动与使用比例',cat_change:'单猫部分变化',common_shift:'共同偏移',unexplained:'多猫变化 / 解释不足'};
</script>
<template>
  <details class="panel batch-analysis-panel">
    <summary>共享池批次差异 <small>近3天 · {{ visible.length }}个观察池</small></summary>
    <p v-if="!visible.length" class="baseline-rule">还没有共享采集。先建立正常基准，再逐块记录多个清理批次。</p>
    <article v-for="pool in visible" :key="`${pool.box_id}:${pool.kind}:${pool.litter_key}`" class="batch-analysis-item">
      <strong>{{ boxName(pool.box_id) }} · {{ pool.kind === 'urine' ? '尿团' : '大便' }}</strong>
      <p>{{ pool.batch_analysis.events }}份可比较事件 · {{ pool.batch_analysis.batches }}批次 · {{ pool.skipped }}份完整性或测量不足</p>
      <p>{{ pool.batch_analysis.reason }}</p>
      <p v-if="pool.batch_analysis.anchor_cat_id">优先排查：{{ catName(pool.batch_analysis.anchor_cat_id) }}（暂定候选）</p>
      <div class="batch-hypotheses" v-if="pool.batch_analysis.hypotheses?.length">
        <span v-for="(h,i) in pool.batch_analysis.hypotheses" :key="i" :class="{ preferred:h.name === pool.batch_analysis.best }">
          {{ labels[h.name] }}{{ h.cat_id ? ` · ${catName(h.cat_id)}` : '' }}<b>证据改善 {{ h.gain }}</b>
        </span>
      </div>
      <small v-if="pool.batch_analysis.hypotheses?.length">时间假设{{ pool.batch_analysis.time_stable ? '结论一致' : '结论不稳定' }}；模型证据未校准，不是概率。异常定位仍待真实验证。</small>
      <p v-if="pool.observations.some(o=>o.status==='time_unresolved'||o.status==='measurement_change')" class="baseline-rule">部分样本存放时间或采集条件未覆盖；保留变化线索，无法作可靠单猫定位。</p>
    </article>
  </details>
</template>
