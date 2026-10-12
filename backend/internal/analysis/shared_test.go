package analysis

import (
	"catcare/internal/model"
	"testing"
	"time"
)

func sharedFixture(overlapping bool) ([]model.Record, []model.Box, []model.Litter, time.Time) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	rs := []model.Record{}
	for cat := uint(1); cat <= 2; cat++ {
		for day := 0; day < 7; day++ {
			g, d := 40.0, 4.0
			if cat == 2 {
				g = 80
				d = 7
				if overlapping {
					g = 42
					d = 4.1
				}
			}
			r := sample(cat*100+uint(day), cat, now.AddDate(0, 0, day-12), g)
			r.Mode = "baseline"
			r.DiameterCM = p(d)
			r.BoxID = 10 + cat
			rs = append(rs, r)
		}
	}
	return rs, []model.Box{{ID: 1, CatIDs: []uint{1, 2}, LitterID: 1}}, []model.Litter{{ID: 1, Version: 1}}, now
}
func sharedItem(id uint, now time.Time, g, d float64) model.Record {
	r := sample(id, 1, now.Add(-time.Hour), g)
	r.Mode = "shared"
	r.CatID = nil
	r.DiameterCM = p(d)
	r.BatchID = "collection"
	return r
}
func sharedAlert(r Report) *Alert {
	for i := range r.Alerts {
		if r.Alerts[i].Metric == "shared_pool" {
			return &r.Alerts[i]
		}
	}
	return nil
}
func TestSharedNormalNeedsNoIdentity(t *testing.T) {
	rs, bs, ls, now := sharedFixture(false)
	rs = append(rs, sharedItem(901, now, 40, 4), sharedItem(902, now, 80, 7))
	r := AnalyzePools(rs, nil, bs, ls, now)
	if r.SharedCount != 2 || r.SharedAnomalies != 0 || sharedAlert(r) != nil {
		t.Fatalf("healthy union should remain quiet: %+v", r)
	}
	for _, m := range r.Matches {
		if m.RecordID >= 900 {
			t.Fatal("shared samples must not require attribution")
		}
	}
}
func TestDifferentialAnchorsChangedCatWithoutAssigningItems(t *testing.T) {
	rs, bs, ls, now := sharedFixture(false)
	rs = append(rs, sharedItem(901, now, 40, 4), sharedItem(902, now, 40, 4), sharedItem(903, now, 15, 2), sharedItem(904, now, 15, 2))
	r := AnalyzePools(rs, nil, bs, ls, now)
	a := sharedAlert(r)
	if a == nil || a.AnchorCatID == nil || *a.AnchorCatID != 2 || a.Localization != "supported" || a.CatID != nil {
		t.Fatalf("whole-mixture difference should prioritize cat 2, preserving no attribution: %+v", a)
	}
	if r.SharedAnomalies != 2 {
		t.Fatal("two novel items should alarm")
	}
	for _, x := range rs {
		if x.Mode == "shared" && x.CatID != nil {
			t.Fatal("algorithm changed item identity")
		}
	}
}
func TestOverlapAlarmsWithoutLockingCat(t *testing.T) {
	rs, bs, ls, now := sharedFixture(true)
	rs = append(rs, sharedItem(901, now, 40, 4), sharedItem(902, now, 40, 4), sharedItem(903, now, 15, 2), sharedItem(904, now, 15, 2))
	a := sharedAlert(AnalyzePools(rs, nil, bs, ls, now))
	if a == nil || a.AnchorCatID != nil || a.Localization != "ambiguous" {
		t.Fatalf("overlap must remain ambiguous even when anomaly exists: %+v", a)
	}
}
func TestIncompleteWeekCannotProduceFalseSharedDiagnosis(t *testing.T) {
	rs, bs, ls, now := sharedFixture(false)
	rs = rs[:len(rs)-1]
	rs = append(rs, sharedItem(901, now, 15, 2))
	r := AnalyzePools(rs, nil, bs, ls, now)
	if sharedAlert(r) != nil {
		t.Fatal("missing seventh day must defer calibrated detection")
	}
	found := false
	for _, pool := range r.Pools {
		for _, o := range pool.Observations {
			if o.RecordID == 901 {
				found = true
				if o.Status != "need_baseline" {
					t.Fatal("incomplete baseline status lost")
				}
			}
		}
	}
	if !found {
		t.Fatal("shared observation should still be saved/displayed")
	}
}
func TestFingerprintFrozenAndCannotLearnFromPool(t *testing.T) {
	rs, _, _, now := sharedFixture(false)
	f := fingerprint(1, "urine", "1:v1", rs, now)
	r := sample(999, 1, now.AddDate(0, 0, -1), 999)
	r.Mode = "baseline"
	rs = append(rs, r)
	for i := 0; i < 20; i++ {
		rs = append(rs, sharedItem(uint(1000+i), now, 999, 30))
	}
	later := fingerprint(1, "urine", "1:v1", rs, now)
	if !f.Ready || f.Days != 7 || later.Metrics["grams"].Median != 40 || later.Samples != 7 {
		t.Fatalf("later and shared samples must not contaminate frozen week: %+v", later)
	}
}
func TestSharedLitterSwitchRequiresNewBaseline(t *testing.T) {
	rs, bs, ls, now := sharedFixture(false)
	ls[0].Version = 2
	r := sharedItem(901, now, 15, 2)
	r.LitterKey = "1:v2"
	rs = append(rs, r)
	out := AnalyzePools(rs, nil, bs, ls, now)
	if sharedAlert(out) != nil {
		t.Fatal("litter version change must not reuse previous fingerprint")
	}
}
func TestSingleNovelItemAlarmsBeforeLocalization(t *testing.T) {
	rs, bs, ls, now := sharedFixture(false)
	rs = append(rs, sharedItem(901, now, 15, 2))
	a := sharedAlert(AnalyzePools(rs, nil, bs, ls, now))
	if a == nil || a.AnchorCatID != nil || a.Localization != "insufficient" {
		t.Fatal("a single novel item must alarm but cannot establish a cat")
	}
}
func TestAggregateCollectionCannotPretendToBeOneItem(t *testing.T) {
	rs, bs, ls, now := sharedFixture(false)
	r := sharedItem(901, now, 100, 10)
	r.Count = 4
	rs = append(rs, r)
	out := AnalyzePools(rs, nil, bs, ls, now)
	if sharedAlert(out) != nil || out.SharedAnomalies != 0 {
		t.Fatal("aggregate weight must not be compared with an individual fingerprint")
	}
}

func TestEntireCollectionShiftCannotSingleOutOneCat(t *testing.T) {
	rs, bs, ls, now := sharedFixture(false)
	rs = append(rs, sharedItem(901, now, 120, 12), sharedItem(902, now, 120, 12), sharedItem(903, now, 160, 15), sharedItem(904, now, 160, 15))
	a := sharedAlert(AnalyzePools(rs, nil, bs, ls, now))
	if a == nil || a.AnchorCatID != nil || !a.Common {
		t.Fatal("a collection-wide shift cannot implicate one cat without healthy control evidence")
	}
}

func TestCalibrationDayUsesConfiguredTimezone(t *testing.T) {
	loc := time.FixedZone("Asia/Tokyo", 9*3600)
	start := time.Date(2026, 10, 1, 0, 30, 0, 0, loc)
	rs := []model.Record{}
	for day := 0; day < 7; day++ {
		r := sample(uint(day+1), 1, start.AddDate(0, 0, day).UTC(), 40)
		r.Mode = "baseline"
		rs = append(rs, r)
	}
	f := fingerprint(1, "urine", "1:v1", rs, start.AddDate(0, 0, 8), loc)
	if !f.Ready || f.Days != 7 || f.Start.In(loc).Format("2006-01-02 15:04") != "2026-10-01 00:00" {
		t.Fatalf("calibration must respect local days: %+v", f)
	}
}
