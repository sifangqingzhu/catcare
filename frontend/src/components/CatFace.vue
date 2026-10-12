<script setup>
defineProps({ color: { default: "sage" }, sleepy: Boolean });
const colors = {
  sage: "#dce8d9",
  peach: "#f4d9c7",
  lavender: "#e3def0",
  blue: "#d6e6eb",
};
</script>
<template>
  <svg viewBox="0 0 80 80" class="cat-face" role="img" aria-label="猫咪头像">
    <circle cx="40" cy="40" r="39" :fill="colors[color] || colors.sage" />
    <path
      d="M17 39 15 15 34 28h12l19-13-2 24c12 30-56 35-46 0"
      fill="#faf7f0"
      stroke="#535b51"
      stroke-width="1.3"
      stroke-linejoin="round"
    />
    <path d="m20 22 3 14 8-5m29-9-3 14-8-5" fill="#e9c9b8" />
    <path
      d="M34 29v10m12-10v10"
      stroke="#c4b5a2"
      stroke-width="3"
      stroke-linecap="round"
    />
    <circle v-if="!sleepy" cx="29" cy="43" r="2.4" fill="#535b51" />
    <circle v-if="!sleepy" cx="51" cy="43" r="2.4" fill="#535b51" />
    <path
      v-if="sleepy"
      d="m25 43 4 2 4-2m14 0 4 2 4-2"
      fill="none"
      stroke="#535b51"
      stroke-width="1.7"
    />
    <path
      d="m37 49 3 3 3-3m-3 3v4m0-1q-4 4-7 0m7 0q4 4 7 0M15 47l10 2m-9 4 9-1m40-5-10 2m9 4-9-1"
      fill="none"
      stroke="#535b51"
      stroke-width="1.2"
      stroke-linecap="round"
    />
  </svg>
</template>
