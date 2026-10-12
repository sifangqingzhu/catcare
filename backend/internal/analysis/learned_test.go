package analysis

import (
	"catcare/internal/model"
	"fmt"
	"math"
	"testing"
	"time"
)

func agingFixture() ([]model.Record, []model.Box, []model.Litter, time.Time) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	rows := []model.Record{}
	id := uint(1)
	for cat := uint(1); cat <= 2; cat++ {
		for day := 0; day < 7; day++ {
			formed := now.AddDate(0, 0, day-12)
			g, d := 40.0, 4.0
			if cat == 2 {
				g, d = 80, 7
			}
			for _, h := range []float64{0, 24} {
				r := sample(id, cat, formed.Add(time.Duration(h)*time.Hour), g*math.Exp(-0.015*h))
				id++
				r.Mode = "baseline"
				r.Integrity = "complete"
				r.IdentityMethod = "isolated"
				r.FormedAt = &formed
				r.EventID = fmt.Sprintf("cat%d-day%d", cat, day)
				r.DiameterCM = p(d * math.Exp(-0.004*h))
				r.BoxID = 10 + cat
				rows = append(rows, r)
			}
		}
	}
	return rows, []model.Box{{ID: 1, CatIDs: []uint{1, 2}, LitterID: 1}}, []model.Litter{{ID: 1, Version: 1}}, now
}
func poolKind(rep Report, kind string) Pool {
	for _, p := range rep.Pools {
		if p.Kind == kind {
			return p
		}
	}
	return Pool{}
}
func TestRepeatedMeasurementsCannotFakeSevenDays(t *testing.T) {
	rows, _, _, now := agingFixture()
	formed := rows[0].OccurredAt
	for i := range rows {
		rows[i].CatID = new(uint)
		*rows[i].CatID = 1
		rows[i].EventID = "one-physical-event"
		rows[i].FormedAt = &formed
	}
	f := fingerprint(1, "urine", "1:v1", rows, now)
	if f.Ready || f.Samples != 1 || f.Days != 1 {
		t.Fatalf("repeated aging observations are one event: %+v", f)
	}
}
func TestPairedAgingExplainsKnownAndUnknownStorage(t *testing.T) {
	rows, bs, ls, now := agingFixture()
	for i, cat := range []uint{1, 2} {
		g, d := 40.0, 4.0
		if cat == 2 {
			g, d = 80, 7
		}
		r := sharedItem(uint(901+i), now, g*math.Exp(-0.015*12), d*math.Exp(-0.004*12))
		r.FormedAt = nil
		last := r.OccurredAt.Add(-24 * time.Hour)
		r.LastCleanedAt = &last
		r.LastCleaningComplete = true
		r.CollectionComplete = true
		rows = append(rows, r)
	}
	rep := AnalyzePools(rows, nil, bs, ls, now)
	pool := poolKind(rep, "urine")
	if rep.SharedAnomalies != 0 || sharedAlert(rep) != nil {
		t.Fatalf("ordinary aging must not be called abnormal: %+v", pool)
	}
	for _, f := range pool.Profiles {
		if f.TimeModel.Status != "paired" || f.TimeModel.PairedEvents < 3 || math.Abs(f.TimeModel.Slopes["grams"]+0.015) > 1e-8 {
			t.Fatalf("derive curve from paired evidence: %+v", f.TimeModel)
		}
	}
	for _, o := range pool.Observations {
		if o.Status != "within_baseline" || o.TimeStatus != "interval" {
			t.Fatalf("integrate unknown ages: %+v", o)
		}
	}
}
func TestTimeCoverageAndIncompleteCleaningRejectLocalization(t *testing.T) {
	for _, mode := range []string{"too_old", "unknown", "incomplete_cleaning", "changed_depth"} {
		t.Run(mode, func(t *testing.T) {
			rows, bs, ls, now := agingFixture()
			r := sharedItem(901, now, 15, 2)
			r.FormedAt = nil
			last := r.OccurredAt.Add(-48 * time.Hour)
			if mode == "too_old" {
				r.LastCleanedAt = &last
				r.LastCleaningComplete = true
			}
			if mode == "incomplete_cleaning" {
				last = r.OccurredAt.Add(-12 * time.Hour)
				r.LastCleanedAt = &last
			}
			if mode == "changed_depth" {
				for i := range rows {
					rows[i].LitterDepthCM = p(4.0)
				}
				formed := r.OccurredAt.Add(-12 * time.Hour)
				r.FormedAt = &formed
				r.LitterDepthCM = p(8.0)
			}
			rows = append(rows, r)
			pool := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine")
			if pool.BatchAnalysis.AnchorCatID != nil || pool.BatchAnalysis.Events != 0 || pool.Observations[0].Status == "anomaly" {
				t.Fatalf("unsupported time/conditions must defer calibrated inference: %+v", pool)
			}
		})
	}
}
func TestMixedSamplesDoNotTrainOrProduceSingleCatEvidence(t *testing.T) {
	rows, bs, ls, now := agingFixture()
	r := sharedItem(901, now, 999, 30)
	r.Integrity = "mixed"
	rows = append(rows, r)
	rep := AnalyzePools(rows, nil, bs, ls, now)
	pool := poolKind(rep, "urine")
	if pool.Skipped != 1 || pool.Anomalies != 0 || pool.Observations[0].Status != "unusable" || pool.BatchAnalysis.AnchorCatID != nil {
		t.Fatalf("mixed samples remain unassigned: %+v", pool)
	}
}
func TestBaselineVersionRebuildAndNoFutureLeakage(t *testing.T) {
	rows, _, _, now := agingFixture()
	original := fingerprint(1, "urine", "1:v1", rows, now)
	newer := rows[0]
	newer.ID = 3000
	newer.BaselineVersion = 2
	newer.EventID = "new-version"
	newer.OccurredAt = now.Add(-time.Hour)
	formed := newer.OccurredAt
	newer.FormedAt = &formed
	newer.Grams = p(100.0)
	rows = append(rows, newer)
	later := fingerprint(1, "urine", "1:v1", rows, now)
	past := fingerprint(1, "urine", "1:v1", rows, now.Add(-24*time.Hour))
	if original.Version != 1 || !original.Ready || later.Version != 2 || later.Ready || past.Version != 1 {
		t.Fatalf("explicit generation rebuilds without contaminating past: old %+v new %+v past %+v", original, later, past)
	}
}
func TestNormalValidationSeparatesCoverageFromAccuracy(t *testing.T) {
	rows, bs, ls, now := agingFixture()
	pool := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine")
	for _, f := range pool.Profiles {
		if f.Quality.NormalValidation != "assessed" || f.Quality.ValidationEvents != 7 || f.Quality.ValidationErrors != 0 || f.Quality.LocalizationValidation != "unvalidated" {
			t.Fatalf("normal holdout is not abnormal validation: %+v", f.Quality)
		}
	}
}
func TestMultipleBatchesSupportOnlyProvisionalCandidate(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	for i := 0; i < 12; i++ {
		g, d := 40.0, 4.0
		if i%3 == 2 {
			g, d = 15, 2
		}
		r := sharedItem(uint(900+i), now, g, d)
		r.BatchID = fmt.Sprintf("batch-%d", i/6)
		r.CollectionComplete = true
		rows = append(rows, r)
	}
	rep := AnalyzePools(rows, nil, bs, ls, now)
	a := sharedAlert(rep)
	pool := poolKind(rep, "urine")
	if a == nil || a.AnchorCatID == nil || *a.AnchorCatID != 2 || a.Localization != "provisional" || pool.BatchAnalysis.Batches != 2 {
		t.Fatalf("multiple batches rank a provisional candidate: alert %+v analysis %+v", a, pool.BatchAnalysis)
	}
	for _, r := range rows {
		if r.Mode == "shared" && r.CatID != nil {
			t.Fatal("never promote inferred identities")
		}
	}
}
func TestPartialChangeRetainsNormalSamplesFromSameCat(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	for i := 0; i < 18; i++ {
		g, d := 40.0, 4.0
		if i%3 == 1 {
			g, d = 80, 7
		}
		if i%3 == 2 {
			g, d = 15, 2
		}
		r := sharedItem(uint(900+i), now, g, d)
		r.BatchID = fmt.Sprintf("batch-%d", i/9)
		r.CollectionComplete = true
		rows = append(rows, r)
	}
	pool := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine")
	hasPartial := false
	for _, h := range pool.BatchAnalysis.Hypotheses {
		if h.Name == "cat_change" && h.Gain > 0 && h.ChangedFraction < 1 {
			hasPartial = true
		}
	}
	if !hasPartial {
		t.Fatalf("normal + changed samples must coexist: %+v", pool.BatchAnalysis)
	}
}
func TestCollisionWithAnotherHealthyCatNeverConfirmsIdentity(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	for i := 0; i < 12; i++ {
		r := sharedItem(uint(900+i), now, 40, 4)
		r.BatchID = fmt.Sprintf("batch-%d", i/6)
		r.CollectionComplete = true
		rows = append(rows, r)
	}
	pool := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine")
	if pool.BatchAnalysis.AnchorCatID != nil {
		t.Fatalf("identical observations cannot establish changed source: %+v", pool.BatchAnalysis)
	}
}
func TestEventDuplicationCannotManufactureBatchEvidence(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	for i := 0; i < 20; i++ {
		r := sharedItem(uint(900+i), now, 15, 2)
		r.EventID = "same-event"
		r.BatchID = fmt.Sprintf("batch%d", i)
		r.CollectionComplete = true
		rows = append(rows, r)
	}
	b := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine").BatchAnalysis
	if b.Events != 1 || b.AnchorCatID != nil {
		t.Fatalf("repeated event must not manufacture independent evidence: %+v", b)
	}
}
func TestAgePriorSensitivityRetainsMultipleCandidates(t *testing.T) {
	rows, bs, ls, now := agingFixture()
	for i := range rows {
		h, _ := knownAge(rows[i])
		g := 40.0
		if *rows[i].CatID == 2 {
			g = 80
		}
		rows[i].Grams = p(g * math.Exp(-0.035*h))
		rows[i].DiameterCM = nil
	}
	r := sharedItem(900, now, 38, 4)
	r.DiameterCM = nil
	r.FormedAt = nil
	last := r.OccurredAt.Add(-24 * time.Hour)
	r.LastCleanedAt = &last
	r.LastCleaningComplete = true
	rows = append(rows, r)
	pool := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine")
	recent := []float64{}
	older := []float64{}
	for _, f := range pool.Profiles {
		a, _, _ := itemLikelihood(r, f, 1, nil)
		b, _, _ := itemLikelihood(r, f, 2, nil)
		recent = append(recent, a)
		older = append(older, b)
	}
	if !(recent[0] > recent[1] && older[1] > older[0]) || len(pool.Observations[0].CandidateIDs) != 2 {
		t.Fatalf("storage assumptions change healthy source; keep both: recent %v old %v result %+v", recent, older, pool.Observations)
	}
}
func TestGapsExtendSamplingAndBadSamplesCannotCompleteBaseline(t *testing.T) {
	rows, _, _, now := sharedFixture(false)
	for i := range rows {
		rows[i].OccurredAt = rows[i].OccurredAt.AddDate(0, 0, i-14)
		formed := rows[i].OccurredAt.Add(-10 * time.Minute)
		rows[i].FormedAt = &formed
	}
	f := fingerprint(1, "urine", "1:v1", rows, now)
	if !f.Ready || f.Days != 7 {
		t.Fatalf("seven independent days may extend beyond a calendar week: days %d, samples %d", f.Days, f.Samples)
	}
	for i := range rows {
		if *rows[i].CatID == 1 {
			rows[i].Integrity = "fragmented"
		}
	}
	f = fingerprint(1, "urine", "1:v1", rows, now)
	if f.Ready || f.Samples != 0 {
		t.Fatalf("fragments never train: %+v", f)
	}
}
func TestInconsistentBaselineConditionsDowngradeQuality(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	rows[0].LitterDepthCM = p(4.0)
	pool := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine")
	if pool.Profiles[0].Quality.State != "conditions_mixed" || pool.Status != "quality_limited" {
		t.Fatalf("inconsistent training conditions must be visible: state %s pool %s", pool.Profiles[0].Quality.State, pool.Status)
	}
}
func TestStoolCategoriesAndDimensionsStaySeparateFromUrine(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	for i := range rows {
		rows[i].Kind = "stool"
		rows[i].DiameterCM = nil
		rows[i].Looseness = nil
		rows[i].Moisture = p(3)
		rows[i].Size = p(3)
		rows[i].Shape = "formed"
		rows[i].LengthCM = p(6.0)
		rows[i].WidthCM = p(1.0)
	}
	r := sharedItem(900, now, 15, 2)
	r.Kind = "stool"
	r.DiameterCM = nil
	r.Looseness = nil
	r.Moisture = p(5)
	r.Size = p(1)
	r.Shape = "watery"
	r.LengthCM = p(1.0)
	r.WidthCM = p(0.5)
	rows = append(rows, r)
	rep := AnalyzePools(rows, nil, bs, ls, now)
	stool := poolKind(rep, "stool")
	urine := poolKind(rep, "urine")
	if !stool.Profiles[0].Ready || stool.Profiles[0].Categories["shape"]["formed"] != 7 || stool.Anomalies != 1 || urine.Profiles[0].Ready {
		t.Fatalf("stool's categorical and physical model stays separate: stool ready %v anomalies %d urine ready %v", stool.Profiles[0].Ready, stool.Anomalies, urine.Profiles[0].Ready)
	}
}
