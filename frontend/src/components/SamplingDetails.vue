<script setup>
defineProps({ sample: Object, baseline: Boolean, cleaning: Boolean });
</script>
<template>
  <details class="sampling-details">
    <summary>{{ baseline ? '形成时间、身份与基准版本' : '形成时间与尺寸（可选）' }}</summary>
    <div class="form-grid sampling-grid">
      <label>样本形成时间<input type="datetime-local" v-model="sample.formed_at" /></label>
      <label>原始事件编号<input v-model.trim="sample.event_id" maxlength="64" placeholder="同份重复称量用同一编号" /></label>
      <label>长轴 / 便条长度（cm）<input type="number" min="0.1" max="50" step="0.1" v-model="sample.length_cm" /></label>
      <label>短轴 / 便条直径（cm）<input type="number" min="0.1" max="50" step="0.1" v-model="sample.width_cm" /></label>
      <label>厚度（cm）<input type="number" min="0.1" max="50" step="0.1" v-model="sample.thickness_cm" /></label>
      <template v-if="baseline">
        <label>身份确认方式<select v-model="sample.identity_method"><option value="isolated">单猫隔离</option><option value="witnessed">人工目击</option><option value="device">设备确认</option></select></label>
        <label>基准版本<input type="number" min="1" max="10000" v-model.number="sample.baseline_version" /></label>
      </template>
      <template v-if="cleaning">
        <label>上次清理时间<input type="datetime-local" v-model="sample.last_cleaned_at" @change="!sample.last_cleaned_at && (sample.last_cleaning_complete = false)" /></label>
        <label>铺砂厚度（cm）<input type="number" min="0.1" max="30" step="0.1" v-model="sample.litter_depth_cm" /></label>
        <label class="check-label"><input type="checkbox" v-model="sample.last_cleaning_complete" :disabled="!sample.last_cleaned_at" />上次已彻底清理</label>
        <label class="check-label"><input type="checkbox" v-model="sample.collection_complete" />本次已完整采集</label>
      </template>
    </div>
    <p class="form-hint">不知道形成时间就留空，不猜时间。重复称量不是新样本；不同碎片合并后再称。{{ baseline ? '同猫砂也可提高基准版本重新采样；7天完成不代表定位已验证。' : '' }}</p>
  </details>
</template>
