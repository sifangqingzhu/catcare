<script setup>
import { computed } from 'vue';
const props=defineProps({joints:Array,job:Object,boxes:Array,cats:Array,canWrite:Boolean});
const emit=defineEmits(['verify']);
const visible=computed(()=>props.joints||[]);
const catName=id=>props.cats.find(c=>c.id===id)?.name||'历史猫咪';
const names=(ids,separator='、')=>ids.map(catName).join(separator);
const boxName=id=>props.boxes.find(b=>b.id===id)?.name||'历史猫砂盆';
</script>
<template>
  <details class="panel batch-analysis-panel">
    <summary>联合差异分析 <small>{{ job?.has_result ? '已完成 · 近3天' : '后台更新中' }}</small></summary>
    <p v-if="!job?.has_result" class="baseline-rule">后台正在建立基准与比较异常组合，不影响记录保存。</p>
    <p v-else-if="!visible.length" class="baseline-rule">还没有可比较的猫砂盆使用群。先建立基准，再逐块记录。</p>
    <article v-for="j in visible" :key="j.id" class="batch-analysis-item">
      <strong>{{ names(j.cat_ids) }}</strong><p>{{ j.box_ids.map(boxName).join(' · ') }}</p>
      <p>{{ j.events }}份独立样本 · {{ j.collections }}轮清理 · {{ j.periods }}个观察时段 · 已比较{{ j.evaluated }}种猫咪组合</p>
      <p>{{ j.reason }}</p>
      <p v-if="j.best_changed_ids.length">变化组合模型内最支持：<b>{{ names(j.best_changed_ids,' + ') }}</b></p>
      <p v-if="j.candidate_ids.length">仍需排查的范围：<b>{{ names(j.candidate_ids) }}</b></p>
      <div class="batch-hypotheses" v-if="j.alternatives.length">
        <span v-for="(a,i) in j.alternatives" :key="i">{{ names(a.cat_ids,' + ') }}<b>证据改善 {{ a.gain }} · 距首名 {{ a.gap }}</b></span>
      </div>
      <p v-if="j.alternative_count > j.alternatives.length">展示前{{ j.alternatives.length }}种 / 共{{ j.alternative_count }}种合理组合；排查范围已包含其余组合。</p>
      <p v-if="j.evaluated">正常解释为比较起点 · 共同偏移改善 {{ j.common_gain }} · 未知解释改善 {{ j.unexplained_gain }}</p>
      <div v-for="a in j.follow_ups" :key="`${a.cat_id}:${a.kind}`" class="follow-up-row">
        <span>建议复核 {{ catName(a.cat_id) }} 的{{ a.kind === 'urine' ? '尿团' : '大便' }}<small>目击形成后约{{ Math.round(a.wait_hours*60) }}分钟测量 · 近似信息增益{{ a.information_gain }}</small></span>
        <button v-if="canWrite" class="text-btn" @click="emit('verify',a)">记录复核样本</button>
      </div>
      <small>“同时变化组合”与“可能来源范围”含义不同。模型证据未校准；候选不是身份确认，异常定位仍待真实验证。</small>
    </article>
  </details>
</template>
