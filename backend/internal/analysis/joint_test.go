package analysis

import (
	"catcare/internal/model"
	"context"
	"fmt"
	"math"
	"testing"
	"time"
)

func jointFixture(n int) ([]model.Record, []model.Box, []model.Litter, time.Time) {
	rows, _, ls, now := sharedFixture(false)
	rows = nil
	ids := []uint{}
	for c := uint(1); c <= uint(n); c++ {
		ids = append(ids, c)
		for day := 0; day < 7; day++ {
			r := sample(c*100+uint(day), c, now.AddDate(0, 0, day-12), 40*float64(c))
			r.Mode = "baseline"
			r.IdentityMethod = "isolated"
			r.Integrity = "complete"
			r.DiameterCM = p(1 + 3*float64(c))
			r.BoxID = 10 + c
			r.FormedAt = new(time.Time)
			*r.FormedAt = r.OccurredAt.Add(-10 * time.Minute)
			rows = append(rows, r)
		}
	}
	return rows, []model.Box{{ID: 1, LitterID: 1, CatIDs: ids}}, ls, now
}
func jointItem(id uint, at time.Time, g, d float64, ids []uint) model.Record {
	r := sharedItem(id, at, g, d)
	r.EligibleCatIDs = append([]uint{}, ids...)
	r.CollectionComplete = true
	return r
}
func TestJointEnumeratesTwoThroughTenCats(t *testing.T) {
	for n := 2; n <= 10; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			rows, bs, ls, now := jointFixture(n)
			for k := 0; k < 2; k++ {
				for c := 1; c <= n; c++ {
					rows = append(rows, jointItem(uint(10000+k*n+c), now.Add(-time.Duration(k)*2*time.Hour), 40*float64(c), 1+3*float64(c), bs[0].CatIDs))
				}
			}
			start := time.Now()
			rep := AnalyzePools(rows, nil, bs, ls, now)
			j := rep.JointAnalyses[0]
			if j.Evaluated != 1<<n || j.Events != 2*n || j.Collections != 2 || j.State != "compatible" || len(j.CandidateIDs) != 0 {
				t.Fatalf("normal %d-cat mixture: %+v", n, j)
			}
			t.Logf("%d cats: %d subsets, %d samples, %s", n, j.Evaluated, j.Events, time.Since(start))
		})
	}
}
func TestConfirmedFollowUpsIdentifyConcurrentChanges(t *testing.T) {
	rows, bs, ls, now := jointFixture(3)
	for k := 0; k < 12; k++ {
		for c := uint(1); c <= 3; c++ {
			g, d := 40*float64(c), 1+3*float64(c)
			if c == 2 {
				g, d = 15, 2
			}
			if c == 3 {
				g, d = 200, 16
			}
			r := jointItem(uint(10000+k*3+int(c)), now.Add(-time.Duration(k/6)*2*time.Hour), g, d, bs[0].CatIDs)
			r.Mode = "observed"
			r.IdentityMethod = "witnessed"
			id := c
			r.CatID = &id
			rows = append(rows, r)
			// Shared collections provide temporal support, labels remain separate evidence.
			if c == 1 {
				s := jointItem(r.ID+1000, r.OccurredAt.Add(time.Hour), g, d, bs[0].CatIDs)
				rows = append(rows, s)
			}
		}
	}
	rep := AnalyzePools(rows, nil, bs, ls, now)
	j := rep.JointAnalyses[0]
	if !sameCats(j.BestChangedIDs, []uint{2, 3}) || !containsID(j.CandidateIDs, 2) || !containsID(j.CandidateIDs, 3) || j.Evaluated != 8 {
		t.Fatalf("confirmed concurrent changes must survive: %+v", j)
	}
	if len(j.CandidateIDs) != 2 {
		t.Fatalf("independent labels should narrow the source set: %+v", j)
	}
	for _, r := range rows {
		if r.Mode == "shared" && r.CatID != nil {
			t.Fatal("inferred identity mutation")
		}
	}
}
func TestUnlabelledConcurrentChangesRetainMultiCatAlternatives(t *testing.T) {
	rows, bs, ls, now := jointFixture(3)
	for k := 0; k < 12; k++ {
		for c := 1; c <= 3; c++ {
			g, d := 40.0, 4.0
			if c == 2 {
				g, d = 15, 2
			}
			if c == 3 {
				g, d = 200, 16
			}
			rows = append(rows, jointItem(uint(10000+k*3+c), now.Add(-time.Duration(k/6)*2*time.Hour), g, d, bs[0].CatIDs))
		}
	}
	j := AnalyzePools(rows, nil, bs, ls, now).JointAnalyses[0]
	retained := false
	for _, a := range j.Alternatives {
		if sameCats(a.CatIDs, []uint{2, 3}) {
			retained = true
		}
	}
	if len(j.BestChangedIDs) < 2 || !retained || !containsID(j.CandidateIDs, 2) || !containsID(j.CandidateIDs, 3) {
		t.Fatalf("two distinct anomalous modes need multi-cat alternatives: %+v", j)
	}
}
func TestOverlappingBoxesCombineEvidenceWithoutIntersectionShortcut(t *testing.T) {
	rows, _, ls, now := jointFixture(3)
	bs := []model.Box{{ID: 1, LitterID: 1, CatIDs: []uint{1, 2}}, {ID: 2, LitterID: 1, CatIDs: []uint{2, 3}}}
	for k := 0; k < 8; k++ {
		for bi, b := range bs {
			g, d := 40.0, 4.0
			if bi == 1 {
				g, d = 120, 10
			}
			if k%2 == 1 {
				g, d = 15, 2
			}
			r := jointItem(uint(1000+k*2+bi), now.Add(-time.Duration(k/4)*2*time.Hour), g, d, b.CatIDs)
			r.BoxID = b.ID
			rows = append(rows, r)
		}
	}
	rep := AnalyzePools(rows, nil, bs, ls, now)
	if len(rep.JointAnalyses) != 1 {
		t.Fatal("overlapping roster was split")
	}
	j := rep.JointAnalyses[0]
	if len(j.CatIDs) != 3 || len(j.BoxIDs) != 2 || j.Evaluated != 8 || !containsID(j.CandidateIDs, 2) {
		t.Fatalf("joint constraints lost: %+v", j)
	}
}
func TestUnrelatedOverlapDoesNotVetoDistinctCatHypothesis(t *testing.T) {
	rows, bs, ls, now := jointFixture(3)
	for i := range rows {
		if *rows[i].CatID == 2 {
			rows[i].Grams = p(40.0)
			rows[i].DiameterCM = p(4.0)
		}
	}
	for k := 0; k < 12; k++ {
		r := jointItem(uint(2000+k), now.Add(-time.Duration(k/6)*2*time.Hour), 15, 2, bs[0].CatIDs)
		id := uint(3)
		r.CatID = &id
		r.Mode = "observed"
		r.IdentityMethod = "witnessed"
		rows = append(rows, r)
	}
	j := AnalyzePools(rows, nil, bs, ls, now).JointAnalyses[0]
	if !sameCats(j.BestChangedIDs, []uint{3}) || !containsID(j.CandidateIDs, 3) {
		t.Fatalf("A/B overlap must not reject C evidence: %+v", j)
	}
}
func TestCollectionSplitCannotManufacturePeriods(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	last := now.Add(-4 * time.Hour)
	for i := 0; i < 12; i++ {
		r := sharedItem(uint(1000+i), now.Add(time.Duration(i%2)*time.Hour), 15, 2)
		r.BatchID = fmt.Sprint(i)
		r.LastCleanedAt = &last
		r.LastCleaningComplete = true
		r.CollectionComplete = true
		rows = append(rows, r)
	}
	j := AnalyzePools(rows, nil, bs, ls, now).JointAnalyses[0]
	if j.Collections != 1 || j.Periods != 1 || j.State == "provisional" {
		t.Fatalf("one cleaning split in submissions: %+v", j)
	}
}
func TestFollowUpPrioritizesDisambiguationAndKeepsNormalComponent(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	for i := 0; i < 18; i++ {
		g, d := 40.0, 4.0
		if i%3 == 1 {
			g, d = 80, 7
		}
		if i%3 == 2 {
			g, d = 15, 2
		}
		rows = append(rows, jointItem(uint(900+i), now.Add(-time.Duration(i/9)*2*time.Hour), g, d, bs[0].CatIDs))
	}
	rep := AnalyzePools(rows, nil, bs, ls, now)
	j := rep.JointAnalyses[0]
	if len(j.FollowUps) == 0 || j.FollowUps[0].InformationGain <= 0 || !containsID(j.CandidateIDs, j.FollowUps[0].CatID) {
		t.Fatalf("ambiguous source should suggest informative observation: %+v", j)
	}
	w := jointWindow{profiles: poolKind(rep, "urine").Profiles, rows: rows[14:]}
	if err := makeOptions(context.Background(), &w, 0); err != nil {
		t.Fatal(err)
	}
	hasPartial := false
	for _, o := range w.candidates[1] {
		if o.rho < 1 && math.IsInf(o.values[2], 0) == false {
			hasPartial = true
		}
	}
	if !hasPartial {
		t.Fatal("changed models erased healthy component")
	}
}
func TestExcludedSourcesPreventFalseExclusion(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	for i := 0; i < 8; i++ {
		r := jointItem(uint(1000+i), now, 15, 2, bs[0].CatIDs)
		if i == 0 {
			r.Integrity = "mixed"
		}
		rows = append(rows, r)
	}
	j := AnalyzePools(rows, nil, bs, ls, now).JointAnalyses[0]
	if j.Excluded != 1 || len(j.CandidateIDs) != 2 || j.State == "provisional" {
		t.Fatal(j)
	}
}
func TestJointCancellationAndImmediateSymptoms(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	r := sharedItem(999, now, 15, 2)
	r.Blood = true
	rows = append(rows, r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AnalyzePoolsContext(ctx, rows, nil, bs, ls, now, nil); err != context.Canceled {
		t.Fatal(err)
	}
	direct := Immediate(rows, now, "UTC")
	if len(direct.Pools) != 0 || len(direct.Alerts) == 0 || direct.Alerts[0].RecordID != 999 {
		t.Fatal("immediate path waited for statistical training")
	}
}
func TestChangeDoesNotRequireBothUrineAndStoolToChange(t *testing.T) {
	rows, bs, ls, now := jointFixture(2)
	base := append([]model.Record{}, rows...)
	for _, r := range base {
		r.ID += 500
		r.Kind = "stool"
		r.DiameterCM = nil
		r.Looseness = nil
		r.LengthCM = p(6.0)
		r.WidthCM = p(1.0)
		r.Shape = "formed"
		r.Moisture = p(3)
		r.Size = p(3)
		rows = append(rows, r)
	}
	for k := 0; k < 12; k++ {
		for _, kind := range []string{"urine", "stool"} {
			r := jointItem(uint(1000+2*k), now, 15, 2, bs[0].CatIDs)
			id := uint(2)
			r.CatID = &id
			r.Mode = "observed"
			r.IdentityMethod = "witnessed"
			r.Kind = kind
			if kind == "stool" {
				r.ID++
				r.Grams = p(80.0)
				r.DiameterCM = nil
				r.Looseness = nil
				r.LengthCM = p(6.0)
				r.WidthCM = p(1.0)
				r.Shape = "formed"
				r.Moisture = p(3)
				r.Size = p(3)
			}
			rows = append(rows, r)
		}
	}
	j := AnalyzePools(rows, nil, bs, ls, now).JointAnalyses[0]
	if !sameCats(j.BestChangedIDs, []uint{2}) || !containsID(j.CandidateIDs, 2) {
		t.Fatalf("urine-specific change must survive normal stool: %+v", j)
	}
}
func TestAllHypothesesRespectConfirmedIdentity(t *testing.T) {
	rows, bs, ls, now := sharedFixture(false)
	fs := poolKind(AnalyzePools(rows, nil, bs, ls, now), "urine").Profiles
	r := sharedItem(1000, now, 80, 7)
	id := uint(2)
	r.CatID = &id
	r.IdentityMethod = "witnessed"
	r.Mode = "observed"
	for _, h := range []windowModel{{name: "healthy"}, {name: "common_shift", shift: map[string]float64{"grams": .1}}, {name: "unexplained"}} {
		a := profileWindow([]model.Record{r}, fs, 0, h, []float64{.99, .01})
		b := profileWindow([]model.Record{r}, fs, 0, h, []float64{.01, .99})
		if math.Abs(a-b) > 1e-9 {
			t.Fatal("confirmed labels must condition every competing hypothesis", h, a, b)
		}
	}
}
func TestCancellationDuringSubsetEnumeration(t *testing.T) {
	rows, bs, ls, now := jointFixture(10)
	for i := 0; i < 20; i++ {
		rows = append(rows, jointItem(uint(10000+i), now, 15, 2, bs[0].CatIDs))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := AnalyzePoolsContext(ctx, rows, nil, bs, ls, now, func(p int, s string) {
		if p >= 25 {
			cancel()
		}
	})
	if err != context.Canceled {
		t.Fatal("subset fitting ignored cancellation", err)
	}
}
