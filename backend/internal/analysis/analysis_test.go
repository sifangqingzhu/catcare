package analysis

import (
	"catcare/internal/model"
	"testing"
	"time"
)

func p[T any](x T) *T { return &x }
func sample(id, cat uint, at time.Time, g float64) model.Record {
	return model.Record{ID: id, CatID: p(cat), Kind: "urine", BoxID: 1, LitterKey: "1:v1", EligibleCatIDs: []uint{1, 2}, OccurredAt: at, Count: 1, Grams: p(g), DiameterCM: p(4.5), Looseness: p(2)}
}
func TestOverlapStaysUnknown(t *testing.T) {
	now := time.Now()
	rs := []model.Record{}
	for i := 0; i < 8; i++ {
		for cat := uint(1); cat <= 2; cat++ {
			rs = append(rs, sample(uint(i*2)+cat, cat, now.AddDate(0, 0, -i-4), 42))
		}
	}
	u := sample(99, 1, now.Add(-time.Hour), 42)
	u.CatID = nil
	rs = append(rs, u)
	r := Analyze(rs, nil, now)
	for _, m := range r.Matches {
		if m.RecordID == 99 {
			if m.Status != "unknown" || len(m.Candidates) != 2 {
				t.Fatalf("overlap wrongly resolved: %+v", m)
			}
			return
		}
	}
	t.Fatal("unknown record missing")
}
func TestIndividualAndCommonChanges(t *testing.T) {
	for _, common := range []bool{false, true} {
		t.Run(map[bool]string{false: "individual", true: "common"}[common], func(t *testing.T) {
			now := time.Now()
			rs := []model.Record{}
			for cat := uint(1); cat <= 2; cat++ {
				for i := 0; i < 8; i++ {
					rs = append(rs, sample(cat*100+uint(i), cat, now.AddDate(0, 0, -i-4), 42))
				}
				g := 42.0
				if cat == 1 || common {
					g = 80
				}
				rs = append(rs, sample(cat*100+99, cat, now.Add(-time.Hour), g))
			}
			r := Analyze(rs, nil, now)
			n := 0
			for _, a := range r.Alerts {
				if a.Metric == "grams" {
					n++
					if a.Common != common {
						t.Fatalf("common detection: %+v", a)
					}
				}
			}
			want := 1
			if common {
				want = 2
			}
			if n != want {
				t.Fatalf("got %d alerts want %d", n, want)
			}
		})
	}
}
func TestUnknownDoesNotPolluteOrImplicateCat(t *testing.T) {
	now := time.Now()
	rs := []model.Record{}
	for i := 0; i < 8; i++ {
		r := sample(uint(i+1), 1, now.AddDate(0, 0, -i-4), 80)
		r.CatID = nil
		rs = append(rs, r)
	}
	rs = append(rs, sample(99, 1, now.Add(-time.Hour), 10))
	out := Analyze(rs, nil, now)
	if len(out.Alerts) != 0 {
		t.Fatal("unknown baseline polluted individual assessment")
	}
}
func TestLitterVersionAndBoxIsolation(t *testing.T) {
	now := time.Now()
	for _, change := range []string{"litter", "box"} {
		rs := []model.Record{}
		for i := 0; i < 8; i++ {
			rs = append(rs, sample(uint(i+1), 1, now.AddDate(0, 0, -i-4), 42))
		}
		r := sample(99, 1, now.Add(-time.Hour), 100)
		if change == "litter" {
			r.LitterKey = "1:v2"
		} else {
			r.BoxID = 2
		}
		rs = append(rs, r)
		out := Analyze(rs, nil, now)
		if len(out.Alerts) != 0 {
			t.Fatalf("%s mixed incompatible baselines", change)
		}
	}
}
func TestInsufficientEvidenceAndFutureData(t *testing.T) {
	now := time.Now()
	rs := []model.Record{}
	for i := 0; i < 10; i++ {
		rs = append(rs, sample(uint(i+1), 1, now.AddDate(0, 0, i+1), 42))
	}
	u := sample(99, 1, now.Add(-time.Hour), 42)
	u.CatID = nil
	rs = append(rs, u)
	out := Analyze(rs, nil, now)
	if len(out.Matches) != 1 || len(out.Matches[0].Candidates) != 0 {
		t.Fatal("future measurements leaked into matching")
	}
}
func TestMixedCollectionAndEmergency(t *testing.T) {
	now := time.Now()
	r := sample(99, 1, now.Add(-time.Hour), 42)
	r.CatID = nil
	r.Count = 4
	r.Straining = true
	r.NoOutput = true
	r.Count = 0
	r.Grams = nil
	out := Analyze([]model.Record{r}, nil, now)
	if len(out.Alerts) != 1 || out.Alerts[0].Level != "urgent" || out.Alerts[0].CatID != nil || out.Matches[0].Status != "unknown" {
		t.Fatalf("emergency must be box-level for unknown identity: %+v", out)
	}
}
func TestZeroMADFinite(t *testing.T) {
	m, s := robust([]float64{42, 42, 42}, 5)
	if m != 42 || s != 5 {
		t.Fatal("zero MAD must use measurement floor")
	}
}
func TestMissingCandidateBaselineCannotGiveConfidentMatch(t *testing.T) {
	now := time.Now()
	rs := []model.Record{}
	for i := 0; i < 8; i++ {
		rs = append(rs, sample(uint(i+1), 1, now.AddDate(0, 0, -i-4), 42))
	}
	u := sample(99, 1, now.Add(-time.Hour), 42)
	u.CatID = nil
	m := match(u, rs, nil)
	if m.Status != "unknown" {
		t.Fatal("missing baseline for second cat must prevent confident suggestion")
	}
}

func TestStoolShapeChange(t *testing.T) {
	now := time.Now()
	rs := []model.Record{}
	for i := 0; i < 8; i++ {
		r := sample(uint(i+1), 1, now.AddDate(0, 0, -i-4), 18)
		r.Kind = "stool"
		r.DiameterCM = nil
		r.Looseness = nil
		r.Shape = "formed"
		r.Moisture = p(3)
		r.Size = p(3)
		rs = append(rs, r)
	}
	r := rs[0]
	r.ID = 99
	r.OccurredAt = now.Add(-time.Hour)
	r.Shape = "watery"
	rs = append(rs, r)
	out := Analyze(rs, nil, now)
	found := false
	for _, a := range out.Alerts {
		if a.Metric == "shape" {
			found = true
			if a.CatID == nil || *a.CatID != 1 {
				t.Fatal("wrong shape alert identity")
			}
		}
	}
	if !found {
		t.Fatal("shape change must be visible")
	}
}
