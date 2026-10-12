package analysis

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"strings"
	"time"

	"catcare/internal/model"
)

type Combination struct {
	CatIDs []uint  `json:"cat_ids"`
	Gain   float64 `json:"gain"`
	Gap    float64 `json:"gap"`
}
type FollowUp struct {
	CatID           uint    `json:"cat_id"`
	BoxID           uint    `json:"box_id"`
	Kind            string  `json:"kind"`
	InformationGain float64 `json:"information_gain"`
	WaitHours       float64 `json:"wait_hours"`
	Reason          string  `json:"reason"`
}
type JointComparison struct {
	ID               string        `json:"id"`
	State            string        `json:"state"`
	Reason           string        `json:"reason"`
	CatIDs           []uint        `json:"cat_ids"`
	BoxIDs           []uint        `json:"box_ids"`
	BestChangedIDs   []uint        `json:"best_changed_ids"`
	CandidateIDs     []uint        `json:"candidate_ids"`
	Alternatives     []Combination `json:"alternatives"`
	AlternativeCount int           `json:"alternative_count"`
	FollowUps        []FollowUp    `json:"follow_ups"`
	Evaluated        int           `json:"evaluated"`
	Events           int           `json:"events"`
	Collections      int           `json:"collections"`
	Periods          int           `json:"periods"`
	Excluded         int           `json:"excluded"`
	TimeStable       bool          `json:"time_stable"`
	Best             string        `json:"best"`
	NormalGain       float64       `json:"normal_gain"`
	CommonGain       float64       `json:"common_gain"`
	UnexplainedGain  float64       `json:"unexplained_gain"`
	Validation       string        `json:"validation"`
}
type prototype struct {
	center     map[string]float64
	categories map[string]string
}
type jointOption struct {
	shift      map[string]float64
	categories map[string]string
	rho        float64
	values     []float64
	probs      []float64
}
type jointWindow struct {
	pool         int
	profiles     []Fingerprint
	rows         []model.Record
	candidates   [][]jointOption
	healthy      [][]float64
	healthyProbs [][]float64
	offsets      []float64
	indices      []int
}
type subsetFit struct {
	mask    uint
	score   float64
	choices [][]int
}

// A prior completely cleaned interval is one collection even when split across
// submissions. Without that metadata, identical collection minutes still merge.
func collectionKey(r model.Record) string {
	t := r.OccurredAt.UTC().Truncate(time.Minute)
	if r.LastCleaningComplete && r.LastCleanedAt != nil {
		t = r.LastCleanedAt.UTC()
	}
	return fmt.Sprintf("%d:%s", r.BoxID, t.Format(time.RFC3339Nano))
}
func observationPeriods(rows []model.Record) (int, int) {
	collections := map[string]time.Time{}
	for _, r := range rows {
		if r.Mode != "shared" {
			continue
		}
		key := collectionKey(r)
		if old, ok := collections[key]; !ok || r.OccurredAt.Before(old) {
			collections[key] = r.OccurredAt
		}
	}
	times := []time.Time{}
	for _, t := range collections {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	count := 0
	last := time.Time{}
	for _, t := range times {
		if count == 0 || t.Sub(last) >= time.Hour {
			count++
			last = t
		}
	}
	return len(collections), count
}
func sharedValid(r model.Record, fs []Fingerprint) bool {
	if r.Count != 1 || r.NoOutput || r.Grams == nil || r.Integrity != "complete" {
		return false
	}
	if r.CatID != nil && !knownIdentity(r) {
		return false
	}
	matched := r.CatID == nil
	for _, f := range fs {
		if f.Quality.State != "ready" || f.End == nil || r.OccurredAt.Before(*f.End) {
			return false
		}
		if r.CatID != nil && f.CatID != *r.CatID {
			continue
		}
		matched = true
		if f.End == nil || r.OccurredAt.Before(*f.End) {
			return false
		}
		l, _, _ := itemLikelihood(r, f, 0, nil)
		if math.IsInf(l, -1) {
			return false
		}
	}
	return matched && len(fs) > 0
}
func sameCats(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	s := map[uint]bool{}
	for _, c := range a {
		s[c] = true
	}
	for _, c := range b {
		if !s[c] {
			return false
		}
	}
	return true
}

// Changed categories are represented separately from continuous locations.
// Missing age-bin training remains missing for both healthy/changed hypotheses.
func changedLikelihood(r model.Record, f Fingerprint, scenario int, p prototype) float64 {
	// Keep the absolute weighing noise floor when the changed center is smaller.
	// Otherwise a large cat's narrow log-scale could spuriously win every small clump.
	scales := map[string]float64{}
	for k, v := range f.Scales {
		scales[k] = v
	}
	if shift, ok := p.center["grams"]; ok {
		scales["grams"] = math.Max(scales["grams"], math.Max(0.05, math.Log1p(5/math.Exp(f.Centers["grams"]+shift))))
	}
	f.Scales = scales
	l, _, _ := itemLikelihood(r, f, scenario, p.center)
	if math.IsInf(l, -1) {
		return l
	}
	// Evaluate each age point again when categorical transitions matter, rather
	// than pretending an ordinal level is an equally spaced numerical distance.
	pts, _ := ages(r, f.TimeModel, scenario)
	scores := []float64{}
	for _, a := range pts {
		formed := r.OccurredAt.Add(-time.Duration(a.hours * float64(time.Hour)))
		probe := r
		probe.FormedAt = &formed
		base, _, _ := itemLikelihood(probe, f, 0, p.center)
		for k, v := range categories(r) {
			counts := f.CategoryAges[ageBin(a.hours)][k]
			total := 0
			for _, n := range counts {
				total += n
			}
			target, ok := p.categories[k]
			if total < 6 || !ok {
				continue
			}
			old := float64(counts[v]+1) / float64(total+5)
			next := 0.1
			if v == target {
				next = 0.6
			}
			base += math.Log(next / old)
		}
		scores = append(scores, math.Log(a.weight)+base)
	}
	return logsum(scores)
}
func makeOptions(ctx context.Context, w *jointWindow, scenario int) error {
	w.healthy = make([][]float64, len(w.profiles))
	w.candidates = make([][]jointOption, len(w.profiles))
	for c, f := range w.profiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		base := make([]float64, len(w.rows))
		for i, r := range w.rows {
			base[i], _, _ = itemLikelihood(r, f, scenario, nil)
		}
		w.healthy[c] = base
		seeds := []prototype{}
		seen := map[string]bool{}
		// Include novel samples and labelled personal changes before normal quantiles.
		ordered := append([]model.Record{}, w.rows...)
		sort.SliceStable(ordered, func(i, j int) bool {
			_, _, a := itemLikelihood(ordered[i], f, scenario, nil)
			_, _, b := itemLikelihood(ordered[j], f, scenario, nil)
			return a > b
		})
		for _, r := range ordered {
			pts, _ := ages(r, f.TimeModel, scenario)
			if len(pts) == 0 {
				continue
			}
			age := 0.0
			for _, p := range pts {
				age += p.hours * p.weight
			}
			center := map[string]float64{}
			parts := []string{}
			for _, k := range f.Keys {
				if v, ok := continuous(r)[k]; ok {
					center[k] = v - f.TimeModel.Slopes[k]*age - f.Centers[k]
					parts = append(parts, fmt.Sprintf("%s:%.1f", k, center[k]/f.Scales[k]))
				}
			}
			cat := map[string]string{}
			for k, v := range categories(r) {
				counts := f.CategoryAges[ageBin(age)][k]
				total, most := 0, ""
				for value, count := range counts {
					total += count
					if most == "" || count > counts[most] || (count == counts[most] && value < most) {
						most = value
					}
				}
				if total >= 6 && v != most {
					cat[k] = v
				}
			}
			for _, k := range []string{"shape", "moisture", "size", "looseness"} {
				parts = append(parts, k+cat[k])
			}
			key := strings.Join(parts, "|")
			if !seen[key] {
				seen[key] = true
				seeds = append(seeds, prototype{center, cat})
			}
			if len(seeds) >= 12 {
				break
			}
		}
		for _, shift := range shiftChoices(w.rows, w.profiles, c, scenario) {
			seeds = append(seeds, prototype{shift, nil})
		}
		for _, seed := range seeds {
			if err := ctx.Err(); err != nil {
				return err
			}
			meaningful := len(seed.categories) > 0
			for k, shift := range seed.center {
				meaningful = meaningful || math.Abs(shift) >= 2*f.Scales[k]
			}
			if !meaningful {
				continue
			}
			change := make([]float64, len(w.rows))
			for i, r := range w.rows {
				change[i] = changedLikelihood(r, f, scenario, seed)
			}
			for _, rho := range []float64{0.25, 0.5, 0.75, 1} {
				v := make([]float64, len(base))
				for i := range v {
					v[i] = logsum([]float64{math.Log(1-rho) + base[i], math.Log(rho) + change[i]})
				}
				w.candidates[c] = append(w.candidates[c], jointOption{shift: seed.center, categories: seed.categories, rho: rho, values: v})
			}
		}
	}
	// Scale each row once; EM repeatedly reuses positive densities, avoiding
	// expensive log/exp work inside every iteration of every subset fit.
	w.offsets = make([]float64, len(w.rows))
	for r := range w.rows {
		top := math.Inf(-1)
		for c := range w.profiles {
			top = math.Max(top, w.healthy[c][r])
			for _, o := range w.candidates[c] {
				top = math.Max(top, o.values[r])
			}
		}
		w.offsets[r] = top
	}
	w.healthyProbs = make([][]float64, len(w.profiles))
	for c := range w.profiles {
		w.healthyProbs[c] = make([]float64, len(w.rows))
		for r := range w.rows {
			w.healthyProbs[c][r] = math.Exp(math.Max(-700, w.healthy[c][r]-w.offsets[r]))
		}
		for k := range w.candidates[c] {
			w.candidates[c][k].probs = make([]float64, len(w.rows))
			for r := range w.rows {
				w.candidates[c][k].probs[r] = math.Exp(math.Max(-700, w.candidates[c][k].values[r]-w.offsets[r]))
			}
		}
	}
	// Retain a bounded dictionary per cat before subset search. Scores are
	// approximate profiled fits, not exhaustive continuous-parameter integration.
	selected := make([]int, len(w.profiles))
	for i := range selected {
		selected[i] = -1
	}
	for c := range w.candidates {
		type ranked struct {
			index int
			score float64
		}
		ranks := []ranked{}
		for k := range w.candidates[c] {
			if err := ctx.Err(); err != nil {
				return err
			}
			selected[c] = k
			ranks = append(ranks, ranked{k, matrixScore(*w, selected)})
		}
		selected[c] = -1
		sort.SliceStable(ranks, func(i, j int) bool { return ranks[i].score > ranks[j].score })
		bank := []jointOption{}
		selectedOptions, prototypes := map[int]bool{}, map[string]bool{}
		// Preserve each distinct residual mode before adding other fractions.
		// Unary top-k alone can discard a second anomaly that only helps a pair.
		for _, r := range ranks {
			o := w.candidates[c][r.index]
			parts := []string{}
			for _, k := range w.profiles[c].Keys {
				parts = append(parts, fmt.Sprintf("%s:%.4f", k, o.shift[k]))
			}
			for _, k := range []string{"shape", "moisture", "size", "looseness"} {
				parts = append(parts, k+o.categories[k])
			}
			key := strings.Join(parts, "|")
			if prototypes[key] {
				continue
			}
			prototypes[key] = true
			selectedOptions[r.index] = true
			bank = append(bank, w.candidates[c][r.index])
			if len(bank) >= 24 {
				break
			}
		}
		for _, r := range ranks {
			if len(bank) >= 24 {
				break
			}
			if !selectedOptions[r.index] {
				bank = append(bank, w.candidates[c][r.index])
			}
		}
		w.candidates[c] = bank
	}
	return nil
}

// Profile likelihood with soft proportions. Identity-confirmed follow-ups are
// conditional on that cat; healthy observations do not exclude intermittent change.
func matrixScore(w jointWindow, selected []int) float64 {
	n, m := len(w.profiles), len(w.rows)
	if n == 0 || m == 0 {
		return 0
	}
	weights := make([]float64, n)
	for c := range weights {
		weights[c] = 1 / float64(n)
	}
	value := func(c, r int) float64 {
		if selected[c] < 0 {
			return w.healthyProbs[c][r]
		}
		return w.candidates[c][selected[c]].probs[r]
	}
	terms := make([]float64, n)
	for it := 0; it < 12; it++ {
		counts := make([]float64, n)
		for c := range counts {
			counts[c] = 0.5
		}
		for r, row := range w.rows {
			if row.CatID != nil {
				continue
			}
			for c := range terms {
				terms[c] = weights[c] * value(c, r)
			}
			den := 0.0
			for _, v := range terms {
				den += v
			}
			for c := range counts {
				counts[c] += terms[c] / den
			}
		}
		total := 0.0
		for _, v := range counts {
			total += v
		}
		for c := range weights {
			weights[c] = counts[c] / total
		}
	}
	score := 0.0
	for r, row := range w.rows {
		if row.CatID != nil {
			for c, f := range w.profiles {
				if f.CatID == *row.CatID {
					score += math.Log(value(c, r)) + w.offsets[r]
					break
				}
			}
		} else {
			for c := range terms {
				terms[c] = weights[c] * value(c, r)
			}
			den := 0.0
			for _, v := range terms {
				den += v
			}
			score += math.Log(den) + w.offsets[r]
		}
	}
	for c, opt := range selected {
		if opt >= 0 {
			score -= 0.5 * float64(len(w.candidates[c][opt].shift)+len(w.candidates[c][opt].categories)+1) * math.Log(math.Max(2, float64(m)))
		}
	}
	return score
}
func fitSubset(ctx context.Context, mask uint, windows []jointWindow, warm *subsetFit) (subsetFit, error) {
	fit := subsetFit{mask: mask, choices: make([][]int, len(windows))}
	for wi, w := range windows {
		fit.choices[wi] = make([]int, len(w.profiles))
		for c, global := range w.indices {
			fit.choices[wi][c] = -1
			if mask&(1<<global) != 0 && warm != nil && warm.mask&(1<<global) != 0 && wi < len(warm.choices) {
				fit.choices[wi][c] = warm.choices[wi][c]
			}
		}
	}
	// A changed cat needs at least one changed endpoint/window, not a forced
	// abnormal component in every urine/stool/box window it participates in.
	selectedCount := func(global int) int {
		n := 0
		for wi, w := range windows {
			for c, g := range w.indices {
				if g == global && fit.choices[wi][c] >= 0 {
					n++
				}
			}
		}
		return n
	}
	for global := 0; global < 10; global++ {
		if mask&(1<<global) == 0 || selectedCount(global) > 0 {
			continue
		}
		bestGain, bestWindowIndex, bestCatIndex := math.Inf(-1), -1, -1
		for wi, w := range windows {
			for c, g := range w.indices {
				if g != global || len(w.candidates[c]) == 0 {
					continue
				}
				before := matrixScore(w, fit.choices[wi])
				fit.choices[wi][c] = 0
				gain := matrixScore(w, fit.choices[wi]) - before
				fit.choices[wi][c] = -1
				if gain > bestGain {
					bestGain, bestWindowIndex, bestCatIndex = gain, wi, c
				}
			}
		}
		if bestWindowIndex < 0 {
			fit.score = math.Inf(-1)
			return fit, nil
		}
		fit.choices[bestWindowIndex][bestCatIndex] = 0
	}
	// Every subset is enumerated. Parameter profiling uses bounded coordinate
	// ascent seeded by the neighboring subset, rather than exact integration.
	for round := 0; round < 2; round++ {
		for wi, w := range windows {
			for c, global := range w.indices {
				if mask&(1<<global) == 0 {
					continue
				}
				if err := ctx.Err(); err != nil {
					return fit, err
				}
				current := fit.choices[wi][c]
				best, opt := matrixScore(w, fit.choices[wi]), current
				// Healthy measurements can coexist with change elsewhere; a healthy row is
				// never used as a hard exclusion of intermittent abnormality.
				if current < 0 || selectedCount(global) > 1 {
					fit.choices[wi][c] = -1
					if score := matrixScore(w, fit.choices[wi]); score > best {
						best, opt = score, -1
					}
				}
				for k := range w.candidates[c] {
					fit.choices[wi][c] = k
					if score := matrixScore(w, fit.choices[wi]); score > best {
						best, opt = score, k
					}
				}
				fit.choices[wi][c] = opt
			}
		}
	}
	for wi, w := range windows {
		fit.score += matrixScore(w, fit.choices[wi])
	}
	return fit, nil
}
func maskCats(mask uint, ids []uint) []uint {
	out := []uint{}
	for i, id := range ids {
		if mask&(1<<i) != 0 {
			out = append(out, id)
		}
	}
	return out
}
func containsID(ids []uint, id uint) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func analyzeJoint(ctx context.Context, rep *Report, rs []model.Record, now time.Time, progress func(int, string)) error {
	rep.JointAnalyses = []JointComparison{}
	// Connected components join overlapping box rosters, never unrelated households.
	parent := map[uint]uint{}
	var root func(uint) uint
	root = func(x uint) uint {
		p, ok := parent[x]
		if !ok {
			parent[x] = x
			return x
		}
		if p != x {
			parent[x] = root(p)
		}
		return parent[x]
	}
	for _, p := range rep.Pools {
		for _, f := range p.Profiles {
			root(f.CatID)
		}
		for i := 1; i < len(p.Profiles); i++ {
			parent[root(p.Profiles[i].CatID)] = root(p.Profiles[0].CatID)
		}
	}
	components := map[uint][]int{}
	for i, p := range rep.Pools {
		if len(p.Profiles) > 0 {
			r := root(p.Profiles[0].CatID)
			components[r] = append(components[r], i)
		}
	}
	keys := []uint{}
	for k := range components {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for ci, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		j, err := jointComponent(ctx, rep, rs, components[key], now, func(done, total int) {
			if progress != nil {
				progress(20+75*(ci*1024+done*1024/max(1, total))/max(1, len(keys)*1024), "联合多猫组合比较")
			}
		})
		if err != nil {
			return err
		}
		rep.JointAnalyses = append(rep.JointAnalyses, j)
		for _, pi := range components[key] {
			p := &rep.Pools[pi]
			p.BatchAnalysis.State = j.State
			p.BatchAnalysis.Reason = j.Reason
			p.BatchAnalysis.Batches = j.Collections
			p.BatchAnalysis.Events = j.Events
			p.BatchAnalysis.Best = j.Best
			p.BatchAnalysis.TimeStable = j.TimeStable
			p.BatchAnalysis.Suspects = []Suspect{}
			for rank, id := range j.CandidateIDs {
				p.BatchAnalysis.Suspects = append(p.BatchAnalysis.Suspects, Suspect{CatID: id, Rank: rank + 1, Reason: "联合候选范围，未校准，不确认归属"})
			}
			if j.State == "provisional" && len(j.CandidateIDs) == 1 {
				id := j.CandidateIDs[0]
				p.BatchAnalysis.AnchorCatID = &id
			}
			for ai := range rep.Alerts {
				a := &rep.Alerts[ai]
				if a.Metric == "shared_pool" && a.BoxID == p.BoxID {
					a.Suspects = p.BatchAnalysis.Suspects
					a.Localization = j.State
					a.AnchorCatID = p.BatchAnalysis.AnchorCatID
					a.Common = j.Best == "common_shift"
					a.Detail = j.Reason
				}
			}
		}
		if j.NormalGain >= 3 && j.State != "compatible" && len(j.BoxIDs) > 0 {
			found := false
			for _, a := range rep.Alerts {
				if a.Metric == "shared_pool" && containsID(j.BoxIDs, a.BoxID) {
					found = true
				}
			}
			if !found {
				rep.Alerts = append(rep.Alerts, Alert{BoxID: j.BoxIDs[0], Level: "watch", Title: "联合批次出现分布变化", Detail: j.Reason, Metric: "shared_pool", Localization: j.State})
			}
		}
	}
	return nil
}
func jointComponent(ctx context.Context, rep *Report, rs []model.Record, poolIDs []int, now time.Time, progress func(int, int)) (JointComparison, error) {
	j := JointComparison{State: "insufficient", Reason: "需要完整基准、存放时间覆盖和足够的独立样本", CatIDs: []uint{}, BoxIDs: []uint{}, BestChangedIDs: []uint{}, CandidateIDs: []uint{}, Alternatives: []Combination{}, FollowUps: []FollowUp{}, Validation: "unvalidated"}
	cats, boxes := map[uint]bool{}, map[uint]bool{}
	for _, pi := range poolIDs {
		p := rep.Pools[pi]
		boxes[p.BoxID] = true
		for _, f := range p.Profiles {
			cats[f.CatID] = true
		}
	}
	for c := range cats {
		j.CatIDs = append(j.CatIDs, c)
	}
	for b := range boxes {
		j.BoxIDs = append(j.BoxIDs, b)
	}
	sort.Slice(j.CatIDs, func(a, b int) bool { return j.CatIDs[a] < j.CatIDs[b] })
	sort.Slice(j.BoxIDs, func(a, b int) bool { return j.BoxIDs[a] < j.BoxIDs[b] })
	j.ID = fmt.Sprintf("joint-%d", j.CatIDs[0])
	n := len(j.CatIDs)
	if n < 2 || n > 10 {
		j.Reason = "联合组合支持每个连通使用群2—10只猫；其他规模保留逐块观察"
		return j, nil
	}
	lookup := map[uint]int{}
	for i, c := range j.CatIDs {
		lookup[c] = i
	}
	windows := []jointWindow{}
	allRows := []model.Record{}
	seen := map[string]bool{}
	complete := true
	// Prefer independent identity confirmation when the same event was measured again.
	ordered := append([]model.Record{}, rs...)
	sort.SliceStable(ordered, func(i, k int) bool {
		return ordered[i].CatID != nil && knownIdentity(ordered[i]) && !(ordered[k].CatID != nil && knownIdentity(ordered[k]))
	})
	for _, pi := range poolIDs {
		p := rep.Pools[pi]
		w := jointWindow{pool: pi, profiles: p.Profiles}
		for _, f := range w.profiles {
			w.indices = append(w.indices, lookup[f.CatID])
		}
		for _, r := range ordered {
			if r.BoxID != p.BoxID || r.Kind != p.Kind || r.LitterKey != p.LitterKey || r.OccurredAt.After(now) || now.Sub(r.OccurredAt) > 3*24*time.Hour || (r.Mode != "shared" && !(r.CatID != nil && knownIdentity(r) && r.Mode != "baseline")) || !sameCats(r.EligibleCatIDs, profileIDs(p.Profiles)) {
				continue
			}
			event := eventKey(r)
			if seen[event] {
				continue
			}
			seen[event] = true
			if !sharedValid(r, p.Profiles) {
				j.Excluded++
				continue
			}
			w.rows = append(w.rows, r)
			allRows = append(allRows, r)
			if r.Mode == "shared" && !r.CollectionComplete {
				complete = false
			}
		}
		if len(w.rows) > 0 {
			windows = append(windows, w)
		}
	}
	j.Events = len(allRows)
	j.Collections, j.Periods = observationPeriods(allRows)
	if j.Events < max(4, n) || len(windows) == 0 {
		return j, nil
	}
	// Refuse unbounded enumeration rather than silently truncating and pretending
	// omitted observations cannot contain a conflicting explanation.
	if j.Events > 240 {
		j.State = "limited"
		j.Reason = "近3天超过240个独立样本；仍保留逐块报警，联合拟合需缩小观察窗口"
		return j, nil
	}
	total := 1 << n
	scenarios := make([][]subsetFit, 3)
	scenarioBest := make([]uint, 3)
	commonGains := make([]float64, 3)
	otherGains := make([]float64, 3)
	allTimesKnown := true
	for _, r := range allRows {
		if r.FormedAt == nil {
			allTimesKnown = false
		}
	}
	for scenario := 0; scenario < 3; scenario++ {
		if scenario > 0 && allTimesKnown {
			scenarios[scenario] = scenarios[0]
			scenarioBest[scenario] = scenarioBest[0]
			commonGains[scenario] = commonGains[0]
			otherGains[scenario] = otherGains[0]
			if progress != nil {
				progress((scenario+1)*total, 3*total)
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return j, err
		}
		for wi := range windows {
			if err := makeOptions(ctx, &windows[wi], scenario); err != nil {
				return j, err
			}
		}
		fits := make([]subsetFit, total)
		for mask := 0; mask < total; mask++ {
			warm := (*subsetFit)(nil)
			if mask > 0 {
				previous := uint(mask) & (uint(mask) - 1)
				warm = &fits[previous]
			}
			fit, err := fitSubset(ctx, uint(mask), windows, warm)
			if err != nil {
				return j, err
			}
			fits[mask] = fit
			if progress != nil {
				progress(scenario*total+mask+1, 3*total)
			}
		}
		base := fits[0].score
		best := 0
		for i := range fits {
			if fits[i].score > fits[best].score {
				best = i
			}
		}
		scenarioBest[scenario] = uint(best)
		common, other := 0.0, 0.0
		for _, w := range windows {
			if err := ctx.Err(); err != nil {
				return j, err
			}
			common += bestCommon(w.rows, w.profiles, scenario)
			other += bestWindow(w.rows, w.profiles, scenario, windowModel{name: "unexplained"})
		}
		commonGains[scenario] = common - base
		otherGains[scenario] = other - base
		scenarios[scenario] = fits
	}
	j.Evaluated = total
	j.TimeStable = scenarioBest[0] == scenarioBest[1] && scenarioBest[1] == scenarioBest[2]
	winnerClasses := []string{}
	for s := 0; s < 3; s++ {
		gain := scenarios[s][scenarioBest[s]].score - scenarios[s][0].score
		winner := "joint_change"
		if gain < 3 {
			winner = "healthy"
		} else if commonGains[s] >= gain-2 {
			winner = "common_shift"
		} else if otherGains[s] >= gain-2 {
			winner = "unexplained"
		}
		winnerClasses = append(winnerClasses, winner)
	}
	j.TimeStable = j.TimeStable && winnerClasses[0] == winnerClasses[1] && winnerClasses[1] == winnerClasses[2]
	fits := scenarios[0]
	base := fits[0].score
	bestMask := scenarioBest[0]
	best := fits[bestMask].score
	j.NormalGain = round2(best - base)
	j.CommonGain = round2(commonGains[0])
	j.UnexplainedGain = round2(otherGains[0])
	j.Best = "joint_change"
	j.BestChangedIDs = maskCats(bestMask, j.CatIDs)
	if j.NormalGain < 3 {
		j.State = "compatible"
		j.Best = "healthy"
		j.Reason = "正常混合、使用比例和时间模型可解释样本；不证明所有猫都健康"
		return j, nil
	}
	if j.CommonGain >= j.NormalGain-2 {
		j.State = "common_shift"
		j.Best = "common_shift"
		j.CandidateIDs = append([]uint{}, j.CatIDs...)
		j.Reason = "共同变化仍是合理解释；无法单凭这些测量指定某只猫"
	} else if j.UnexplainedGain >= j.NormalGain-2 {
		j.State = "unexplained"
		j.Best = "unexplained"
		j.CandidateIDs = append([]uint{}, j.CatIDs...)
		j.Reason = "异常组合仍不能排除未知来源或模型未覆盖，保留全部候选"
	} else {
		j.State = "candidate_set"
		j.Reason = "联合模型支持变化，保留所有合理组合中的猫；候选范围内不代表每只都异常"
	}
	credible := []subsetFit{}
	union := uint(0)
	for scenario, fs := range scenarios {
		winner := fs[scenarioBest[scenario]].score
		for _, fit := range fs {
			if fit.mask > 0 && winner-fit.score <= 4 {
				union |= fit.mask
				if scenario == 0 {
					credible = append(credible, fit)
				}
			}
		}
	}
	sort.Slice(credible, func(i, k int) bool {
		if credible[i].score == credible[k].score {
			return credible[i].mask < credible[k].mask
		}
		return credible[i].score > credible[k].score
	})
	j.AlternativeCount = len(credible)
	for _, fit := range credible {
		if len(j.Alternatives) >= 12 {
			break
		}
		j.Alternatives = append(j.Alternatives, Combination{maskCats(fit.mask, j.CatIDs), round2(fit.score - base), round2(best - fit.score)})
	}
	if j.Best == "joint_change" {
		j.CandidateIDs = maskCats(union, j.CatIDs)
	}
	if j.Excluded > 0 {
		j.CandidateIDs = append([]uint{}, j.CatIDs...)
		j.Reason += " 部分样本未覆盖，不能据此排除其他猫。"
	}
	validated := true
	for _, w := range windows {
		for _, f := range w.profiles {
			q := f.Quality
			if q.NormalValidation != "assessed" || q.ValidationErrors > 0 || q.ValidationCoverage < (q.ValidationEvents+1)/2 {
				validated = false
			}
		}
	}
	if j.Best == "joint_change" && j.TimeStable && validated && complete && j.Periods >= 2 && j.Excluded == 0 && union == bestMask && bestMask != 0 {
		if bits.OnesCount(bestMask) == 1 {
			j.State = "provisional"
			j.Reason = "跨时段及不同存放假设支持单猫暂定候选；身份与异常准确率仍未验证"
		} else {
			j.State = "provisional_combination"
			j.Reason = "跨时段支持多猫同时变化的暂定组合，不能当作身份确认或诊断"
		}
	}
	if !j.TimeStable {
		j.Reason += " 存放假设改变候选组合，需要追加身份已确认的样本。"
	}
	if j.Periods < 2 {
		j.Reason += " 当前独立观察时段不足；拆分同次清理不增加时段证据。"
	}
	// Rebuild the uniform-age dictionaries for the diagnostic action model.
	for wi := range windows {
		if err := makeOptions(ctx, &windows[wi], 0); err != nil {
			return j, err
		}
	}
	j.FollowUps = followUps(ctx, j, credible, windows, now)
	return j, ctx.Err()
}
func profileIDs(fs []Fingerprint) []uint {
	out := []uint{}
	for _, f := range fs {
		out = append(out, f.CatID)
	}
	return out
}

// Deterministic predictive quadrature approximates expected entropy reduction.
// These internal model weights are not presented as calibrated probabilities.
func followUps(ctx context.Context, j JointComparison, fits []subsetFit, windows []jointWindow, now time.Time) []FollowUp {
	out := []FollowUp{}
	if len(fits) == 0 {
		return out
	}
	if len(fits) > 24 {
		fits = fits[:24]
	}
	prior := make([]float64, len(fits))
	sum := 0.0
	for i, f := range fits {
		prior[i] = math.Exp(f.score - fits[0].score)
		sum += prior[i]
	}
	entropy := 0.0
	for i := range prior {
		prior[i] /= sum
		entropy -= prior[i] * math.Log(prior[i])
	}
	for wi, w := range windows {
		for c, f := range w.profiles {
			if !containsID(j.CandidateIDs, f.CatID) {
				continue
			}
			if ctx.Err() != nil {
				return out
			}
			age := math.Max(f.TimeModel.MinHours, 0.17)
			if age > f.TimeModel.MaxHours+0.25 {
				continue
			}
			expected := 0.0
			mass := 0.0
			for hi, fit := range fits {
				opt := fit.choices[wi][c]
				rho := 0.0
				p := prototype{map[string]float64{}, map[string]string{}}
				if opt >= 0 {
					o := w.candidates[c][opt]
					rho = o.rho
					p = prototype{o.shift, o.categories}
				}
				for branch, branchWeight := range []float64{1 - rho, rho} {
					if branchWeight <= 0 {
						continue
					}
					center := map[string]float64{}
					for _, k := range f.Keys {
						center[k] = f.Centers[k] + f.TimeModel.Slopes[k]*age
						if branch == 1 {
							center[k] += p.center[k]
						}
					}
					for point := 0; point < 1+2*len(f.Keys); point++ {
						if ctx.Err() != nil {
							return out
						}
						r := model.Record{Kind: w.rows[0].Kind, BoxID: w.rows[0].BoxID, Count: 1, Integrity: "complete", OccurredAt: now, LitterDepthCM: f.TimeModel.DepthCM}
						formed := now.Add(-time.Duration(age * float64(time.Hour)))
						r.FormedAt = &formed
						for ki, k := range f.Keys {
							v := center[k]
							if point > 0 && (point-1)/2 == ki {
								sign := 1.0
								if (point-1)%2 == 1 {
									sign = -1
								}
								v += sign * f.Scales[k]
							}
							x := math.Exp(v)
							switch k {
							case "grams":
								r.Grams = &x
							case "diameter":
								r.DiameterCM = &x
							case "length":
								r.LengthCM = &x
							case "width":
								r.WidthCM = &x
							case "thickness":
								r.ThicknessCM = &x
							}
						}
						for _, k := range []string{"shape", "moisture", "size", "looseness"} {
							counts := f.CategoryAges[ageBin(age)][k]
							v := ""
							largest := 0
							for category, count := range counts {
								if count > largest || count == largest && category < v {
									largest = count
									v = category
								}
							}
							if branch == 1 && p.categories[k] != "" {
								v = p.categories[k]
							}
							if v == "" {
								continue
							}
							level, _ := strconv.Atoi(v)
							switch k {
							case "shape":
								r.Shape = v
							case "moisture":
								r.Moisture = &level
							case "size":
								r.Size = &level
							case "looseness":
								r.Looseness = &level
							}
						}
						logWeights := []float64{}
						for ai, a := range fits {
							l, _, _ := itemLikelihood(r, f, 0, nil)
							choice := a.choices[wi][c]
							if choice >= 0 {
								o := w.candidates[c][choice]
								changed := changedLikelihood(r, f, 0, prototype{o.shift, o.categories})
								l = logsum([]float64{math.Log(1-o.rho) + l, math.Log(o.rho) + changed})
							}
							logWeights = append(logWeights, math.Log(prior[ai])+l)
						}
						den := logsum(logWeights)
						h := 0.0
						for _, v := range logWeights {
							prob := math.Exp(v - den)
							if prob > 0 {
								h -= prob * math.Log(prob)
							}
						}
						weight := prior[hi] * branchWeight / float64(1+2*len(f.Keys))
						expected += weight * h
						mass += weight
					}
				}
			}
			gain := math.Max(0, entropy-expected/math.Max(mass, 1e-12))
			out = append(out, FollowUp{f.CatID, w.rows[0].BoxID, w.rows[0].Kind, round2(gain), round2(age), "目击确认下一份独立样本的身份，保留形成时间；一份正常样本不能排除间歇变化"})
		}
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].InformationGain == out[k].InformationGain {
			return out[i].CatID < out[k].CatID
		}
		return out[i].InformationGain > out[k].InformationGain
	})
	seen := map[uint]bool{}
	unique := []FollowUp{}
	for _, a := range out {
		if !seen[a.CatID] {
			seen[a.CatID] = true
			unique = append(unique, a)
		}
		if len(unique) >= 3 {
			break
		}
	}
	return unique
}
