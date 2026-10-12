export const samplingDefaults = () => ({
  formed_at: "", last_cleaned_at: "", last_cleaning_complete: false,
  collection_complete: false, integrity: "complete", event_id: "",
  identity_method: "unknown", baseline_version: 1,
  length_cm: "", width_cm: "", thickness_cm: "", litter_depth_cm: "",
});
export const samplingPayload = (r) => {
  const number = (v) => v === "" || v === null || v === undefined ? null : Number(v);
  const time = (v) => v ? new Date(v).toISOString() : null;
  return {
    formed_at: time(r.formed_at), last_cleaned_at: time(r.last_cleaned_at),
    last_cleaning_complete: !!r.last_cleaning_complete, collection_complete: !!r.collection_complete,
    integrity: r.integrity || "unknown", event_id: r.event_id || "",
    identity_method: r.mode === "shared" ? "unknown" : r.identity_method || "unknown",
    baseline_version: Number(r.baseline_version || 1),
    length_cm: number(r.length_cm), width_cm: number(r.width_cm),
    thickness_cm: number(r.thickness_cm), litter_depth_cm: number(r.litter_depth_cm),
  };
};
