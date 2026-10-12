package analysis

import (
	"catcare/internal/model"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type MetricBaseline struct {
	Median  float64 `json:"median"`
	Scale   float64 `json:"scale"`
	Samples int     `json:"samples"`
}
type Fingerprint struct {
	CatID   uint                      `json:"cat_id"`
	Days    int                       `json:"days"`
	Samples int                       `json:"samples"`
	Ready   bool                      `json:"ready"`
	Start   *time.Time                `json:"start"`
	End     *time.Time                `json:"end"`
	Metrics map[string]MetricBaseline `json:"metrics"`
	Shapes  map[string]int            `json:"shapes"`
}
type PoolObservation struct {
	RecordID uint    `json:"record_id"`
	Status   string  `json:"status"`
	Novelty  float64 `json:"novelty"`
}
type Suspect struct {
	CatID           uint    `json:"cat_id"`
	Score           float64 `json:"score"`
	Gain            float64 `json:"gain"`
	RemainingNormal int     `json:"remaining_normal"`
	Reason          string  `json:"reason"`
}
type Pool struct {
	BoxID        uint              `json:"box_id"`
	Kind         string            `json:"kind"`
	LitterKey    string            `json:"litter_key"`
	Status       string            `json:"status"`
	Profiles     []Fingerprint     `json:"profiles"`
	Inspected    int               `json:"inspected"`
	Anomalies    int               `json:"anomalies"`
	Skipped      int               `json:"skipped"`
	Observations []PoolObservation `json:"observations"`
}

// A fingerprint is frozen to the first seven consecutive calendar days of explicitly
// isolated, healthy baseline observations. Shared samples can never update it.
func fingerprint(cat uint, kind, key string, rs []model.Record, cutoff time.Time, locations ...*time.Location) Fingerprint {
	f := Fingerprint{CatID: cat, Metrics: map[string]MetricBaseline{}, Shapes: map[string]int{}}
	samples := []model.Record{}
	for _, r := range rs {
		if r.Mode == "baseline" && r.CatID != nil && *r.CatID == cat && r.Kind == kind && r.LitterKey == key && r.Count == 1 && !r.NoOutput && !r.Blood && !r.Straining && r.OccurredAt.Before(cutoff) {
			samples = append(samples, r)
		}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].OccurredAt.Before(samples[j].OccurredAt) })
	if len(samples) == 0 {
		return f
	}
	location := time.UTC
	if len(locations) > 0 && locations[0] != nil {
		location = locations[0]
	}
	first := samples[0].OccurredAt.In(location)
	start := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 7)
	f.Start = &start
	f.End = &end
	dates := map[string]bool{}
	values := map[string][]float64{}
	for _, r := range samples {
		if !r.OccurredAt.Before(end) {
			continue
		}
		if len(features(r)) == 0 && r.Shape == "" {
			continue
		}
		dates[r.OccurredAt.In(location).Format("2006-01-02")] = true
		f.Samples++
		for k, v := range features(r) {
			values[k] = append(values[k], v)
		}
		if r.Shape != "" {
			f.Shapes[r.Shape]++
		}
	}
	f.Days = len(dates)
	for k, vs := range values {
		if len(vs) >= 6 {
			m, s := robust(vs, floors[k])
			f.Metrics[k] = MetricBaseline{m, s, len(vs)}
		}
	}
	f.Ready = f.Days >= 7 && !cutoff.Before(end) && (len(f.Metrics) > 0 || len(f.Shapes) > 0)
	return f
}

// loss compares one physical item against a healthy cluster. Novelty uses the
// maximum standardized discrepancy so extra unchanged dimensions cannot dilute it.
func loss(r model.Record, f Fingerprint, center map[string]float64, shape string) (float64, float64, int) {
	sum, maxZ, n := 0.0, 0.0, 0
	for k, v := range features(r) {
		b, ok := f.Metrics[k]
		if !ok {
			continue
		}
		mu := b.Median
		if center != nil {
			if c, ok := center[k]; ok {
				mu = c
			}
		}
		z := math.Min(10, math.Abs(v-mu)/b.Scale)
		sum += z * z
		maxZ = math.Max(maxZ, z)
		n++
	}
	if r.Kind == "stool" && r.Shape != "" && len(f.Shapes) > 0 {
		z := 0.0
		if center != nil {
			if r.Shape != shape {
				z = 4
			}
		} else {
			total := 0
			for _, c := range f.Shapes {
				total += c
			}
			if float64(f.Shapes[r.Shape])/float64(total) < 0.1 {
				z = 4
			}
		}
		sum += z * z
		maxZ = math.Max(maxZ, z)
		n++
	}
	if n == 0 {
		return math.Inf(1), math.Inf(1), 0
	}
	return sum / float64(n), maxZ, n
}
func nearest(r model.Record, fs []Fingerprint, replace int, center map[string]float64, shape string) (float64, float64, int, int) {
	best, novel := math.Inf(1), math.Inf(1)
	winner, n := -1, 0
	for i, f := range fs {
		c := map[string]float64(nil)
		s := ""
		if i == replace {
			c = center
			s = shape
		}
		l, z, e := loss(r, f, c, s)
		if l < best {
			best = l
			winner = i
			n = e
		}
		novel = math.Min(novel, z)
	}
	return best, novel, winner, n
}
func overlap(a, b Fingerprint) bool {
	evidence := 0
	maxZ := 0.0
	for k, x := range a.Metrics {
		if y, ok := b.Metrics[k]; ok {
			evidence++
			maxZ = math.Max(maxZ, math.Abs(x.Median-y.Median)/math.Max(x.Scale, y.Scale))
		}
	}
	return evidence == 0 || maxZ < 2
}
func differential(observed, abnormal []model.Record, fs []Fingerprint) ([]Suspect, string, *uint) {
	center := map[string]float64{}
	values := map[string][]float64{}
	shapes := map[string]int{}
	for _, r := range abnormal {
		for k, v := range features(r) {
			values[k] = append(values[k], v)
		}
		if r.Shape != "" {
			shapes[r.Shape]++
		}
	}
	for k, vs := range values {
		center[k] = median(vs)
	}
	shape := ""
	most := 0
	for k, n := range shapes {
		if n > most {
			shape = k
			most = n
		}
	}
	before := 0.0
	for _, r := range observed {
		l, _, _, _ := nearest(r, fs, -1, nil, "")
		before += l
	}
	before /= float64(len(observed))
	suspects := []Suspect{}
	gainTotal := 0.0
	for changed, f := range fs {
		after := 0.0
		normal := map[int]bool{}
		for _, r := range observed {
			l, _, _, _ := nearest(r, fs, changed, center, shape)
			after += l
			healthy, nov, winner, _ := nearest(r, fs, -1, nil, "")
			if winner >= 0 && winner != changed && nov < 1.5 {
				second := math.Inf(1)
				for i, other := range fs {
					if i == winner {
						continue
					}
					v, _, _ := loss(r, other, nil, "")
					second = math.Min(second, v)
				}
				if second-healthy >= 1 {
					normal[winner] = true
				}
			}
		}
		after /= float64(len(observed))
		gain := math.Max(0, before-after)
		gainTotal += gain
		suspects = append(suspects, Suspect{CatID: f.CatID, Gain: gain, RemainingNormal: len(normal), Reason: fmt.Sprintf("假设该猫的基线簇发生变化，整体残差 %.2f → %.2f；其余 %d 只猫有可区分的正常样本支持。", before, after, len(normal))})
	}
	for i := range suspects {
		if gainTotal > 0 {
			suspects[i].Score = math.Round(100 * suspects[i].Gain / gainTotal)
		}
		suspects[i].Gain = math.Round(suspects[i].Gain*100) / 100
	}
	sort.Slice(suspects, func(i, j int) bool {
		if suspects[i].Score == suspects[j].Score {
			return suspects[i].CatID < suspects[j].CatID
		}
		return suspects[i].Score > suspects[j].Score
	})
	if len(suspects) < 2 || len(abnormal) < 2 || len(observed) < len(fs) {
		return suspects, "insufficient", nil
	}
	separated := true
	for i := range fs {
		for j := i + 1; j < len(fs); j++ {
			if overlap(fs[i], fs[j]) {
				separated = false
			}
		}
	}
	gap := suspects[0].Score - suspects[1].Score
	if separated && suspects[0].Score >= 70 && gap >= 20 && suspects[0].RemainingNormal == len(fs)-1 && suspects[0].Gain >= 1 {
		id := suspects[0].CatID
		return suspects, "supported", &id
	}
	return suspects, "ambiguous", nil
}

// AnalyzePools detects novelty before considering identities. A counterfactual
// mixture comparison then ranks which changed cluster best explains the WHOLE
// collection, including the other healthy-looking items. It never assigns CatID.
func AnalyzePools(rs []model.Record, weights []model.Weight, boxes []model.Box, litters []model.Litter, now time.Time, locations ...*time.Location) Report {
	rep := Analyze(rs, weights, now)
	rep.Pools = []Pool{}
	type group struct {
		box       uint
		kind, key string
		cats      []uint
		records   []model.Record
	}
	groups := map[string]*group{}
	keyFor := func(box uint, kind, key string, ids []uint) string {
		parts := []string{}
		for _, id := range ids {
			parts = append(parts, fmt.Sprint(id))
		}
		return fmt.Sprintf("%d:%s:%s:%s", box, kind, key, strings.Join(parts, ","))
	}
	for _, b := range boxes {
		if b.Archived {
			continue
		}
		for _, l := range litters {
			if l.ID != b.LitterID {
				continue
			}
			for _, kind := range []string{"urine", "stool"} {
				k := fmt.Sprintf("%d:v%d", l.ID, l.Version)
				groups[keyFor(b.ID, kind, k, b.CatIDs)] = &group{b.ID, kind, k, b.CatIDs, []model.Record{}}
			}
		}
	}
	for _, r := range rs {
		if r.Mode != "shared" || r.OccurredAt.After(now) || now.Sub(r.OccurredAt) > 7*24*time.Hour {
			continue
		}
		rep.SharedCount++
		if now.Sub(r.OccurredAt) > 3*24*time.Hour {
			continue
		}
		k := keyFor(r.BoxID, r.Kind, r.LitterKey, r.EligibleCatIDs)
		if groups[k] == nil {
			groups[k] = &group{r.BoxID, r.Kind, r.LitterKey, r.EligibleCatIDs, []model.Record{}}
		}
		groups[k].records = append(groups[k].records, r)
	}
	keys := []string{}
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		cutoff := now
		for _, r := range g.records {
			if r.OccurredAt.Before(cutoff) {
				cutoff = r.OccurredAt
			}
		}
		pool := Pool{BoxID: g.box, Kind: g.kind, LitterKey: g.key, Status: "need_baseline", Profiles: []Fingerprint{}, Observations: []PoolObservation{}}
		ready := len(g.cats) > 0
		for _, cat := range g.cats {
			f := fingerprint(cat, g.kind, g.key, rs, cutoff, locations...)
			pool.Profiles = append(pool.Profiles, f)
			if !f.Ready {
				ready = false
			}
		}
		valid, abnormal := []model.Record{}, []model.Record{}
		maxNovel := 0.0
		for _, r := range g.records {
			o := PoolObservation{RecordID: r.ID, Status: "need_baseline"}
			if !ready {
				pool.Observations = append(pool.Observations, o)
				continue
			}
			if r.Count != 1 || r.NoOutput {
				pool.Skipped++
				o.Status = "aggregate_only"
				pool.Observations = append(pool.Observations, o)
				continue
			}
			l, z, _, n := nearest(r, pool.Profiles, -1, nil, "")
			if math.IsInf(l, 1) || n == 0 {
				pool.Skipped++
				o.Status = "insufficient_metrics"
				pool.Observations = append(pool.Observations, o)
				continue
			}
			valid = append(valid, r)
			pool.Inspected++
			o.Status = "within_baseline"
			o.Novelty = math.Round(z*10) / 10
			if z >= 3.5 {
				pool.Anomalies++
				rep.SharedAnomalies++
				o.Status = "anomaly"
				abnormal = append(abnormal, r)
				maxNovel = math.Max(maxNovel, z)
			}
			pool.Observations = append(pool.Observations, o)
		}
		if ready {
			pool.Status = "ready"
		}
		if len(abnormal) > 0 {
			pool.Status = "anomaly"
			suspects, localization, anchor := differential(valid, abnormal, pool.Profiles)
			latest := abnormal[0]
			for _, r := range abnormal {
				if r.OccurredAt.After(latest.OccurredAt) {
					latest = r
				}
			}
			kindName := "尿团"
			if g.kind == "stool" {
				kindName = "大便"
			}
			detail := fmt.Sprintf("近3天逐块检测 %d 个%s，其中 %d 个超出全部已完成的7天单猫基线簇。保留共享采集，不要求每块归属。", pool.Inspected, kindName, pool.Anomalies)
			if anchor != nil {
				detail += " 差异模型支持优先排查候选猫；这不是身份确认或诊断。"
			} else {
				detail += " 当前差异证据不足以锁定单猫，请继续逐块采样或短期分开复核。"
			}
			common := len(valid) == len(abnormal) && len(g.cats) > 1
			if common {
				detail += " 本次全部有效样本均偏离：可能存在共同变化、配置不一致或采样偏差。"
			}
			rep.Alerts = append(rep.Alerts, Alert{BoxID: g.box, RecordID: latest.ID, Level: "watch", Title: "共享池出现异常" + kindName, Detail: detail, Metric: "shared_pool", Z: math.Round(maxNovel*10) / 10, Common: common, Suspects: suspects, Localization: localization, AnchorCatID: anchor})
		}
		rep.Pools = append(rep.Pools, pool)
	}
	sort.SliceStable(rep.Alerts, func(i, j int) bool { return rep.Alerts[i].Level == "urgent" && rep.Alerts[j].Level != "urgent" })
	return rep
}
