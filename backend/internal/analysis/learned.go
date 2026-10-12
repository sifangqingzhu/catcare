package analysis

// v2 keeps identities unknown. Healthy density, age uncertainty and window-level
// alternatives are separate. Every reported score is uncalibrated model evidence.
import (
	"catcare/internal/model"
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type BaselineQuality struct {
	State                  string   `json:"state"`
	Reasons                []string `json:"reasons"`
	DistinctEvents         int      `json:"distinct_events"`
	ConfirmedEvents        int      `json:"confirmed_events"`
	CompleteEvents         int      `json:"complete_events"`
	Separation             string   `json:"separation"`
	NormalValidation       string   `json:"normal_validation"`
	ValidationEvents       int      `json:"validation_events"`
	ValidationErrors       int      `json:"validation_errors"`
	ValidationCoverage     int      `json:"validation_coverage"`
	LocalizationValidation string   `json:"localization_validation"`
}
type AgingModel struct {
	Status       string             `json:"status"`
	MinHours     float64            `json:"min_hours"`
	MaxHours     float64            `json:"max_hours"`
	KnownEvents  int                `json:"known_events"`
	PairedEvents int                `json:"paired_events"`
	Slopes       map[string]float64 `json:"slopes"`
	DepthCM      *float64           `json:"depth_cm"`
}
type Hypothesis struct {
	Name            string  `json:"name"`
	CatID           uint    `json:"cat_id,omitempty"`
	Evidence        float64 `json:"evidence"`
	Gain            float64 `json:"gain"`
	ChangedFraction float64 `json:"changed_fraction,omitempty"`
}
type BatchComparison struct {
	State       string       `json:"state"`
	Reason      string       `json:"reason"`
	Batches     int          `json:"batches"`
	Events      int          `json:"events"`
	Best        string       `json:"best"`
	TimeStable  bool         `json:"time_stable"`
	Hypotheses  []Hypothesis `json:"hypotheses"`
	Suspects    []Suspect    `json:"suspects"`
	AnchorCatID *uint        `json:"anchor_cat_id,omitempty"`
}

var continuousKeys = []string{"grams", "diameter", "length", "width", "thickness"}

func continuous(r model.Record) map[string]float64 {
	out := map[string]float64{}
	for k, p := range map[string]*float64{"grams": r.Grams, "diameter": r.DiameterCM, "length": r.LengthCM, "width": r.WidthCM, "thickness": r.ThicknessCM} {
		if p != nil && *p > 0 {
			out[k] = math.Log(*p)
		}
	}
	return out
}
func categories(r model.Record) map[string]string {
	out := map[string]string{}
	if r.Kind == "urine" && r.Looseness != nil {
		out["looseness"] = strconv.Itoa(*r.Looseness)
	}
	if r.Kind == "stool" {
		if r.Moisture != nil {
			out["moisture"] = strconv.Itoa(*r.Moisture)
		}
		if r.Size != nil {
			out["size"] = strconv.Itoa(*r.Size)
		}
		if r.Shape != "" {
			out["shape"] = r.Shape
		}
	}
	return out
}
func eventKey(r model.Record) string {
	if r.EventID != "" {
		return "event:" + r.EventID
	}
	return fmt.Sprintf("record:%d", r.ID)
}
func sampleDate(r model.Record, loc *time.Location) string {
	t := r.OccurredAt
	if r.FormedAt != nil {
		t = *r.FormedAt
	}
	return t.In(loc).Format("2006-01-02")
}
func knownIdentity(r model.Record) bool {
	return r.IdentityMethod == "isolated" || r.IdentityMethod == "witnessed" || r.IdentityMethod == "device"
}
func knownAge(r model.Record) (float64, bool) {
	if r.FormedAt == nil || r.FormedAt.After(r.OccurredAt) {
		return 0, false
	}
	return r.OccurredAt.Sub(*r.FormedAt).Hours(), true
}
func measurementOK(r model.Record) bool {
	return r.Count == 1 && !r.NoOutput && r.Grams != nil && *r.Grams > 0 && !r.Blood && !r.Straining
}

// Seven independent sampled days, not seven measurements of one physical event.
// An explicit baseline version permits a new approved-data training window with
// the same litter. Later records never enter a completed version's training set.
func fingerprint(cat uint, kind, key string, rs []model.Record, cutoff time.Time, locations ...*time.Location) Fingerprint {
	loc := time.UTC
	if len(locations) > 0 && locations[0] != nil {
		loc = locations[0]
	}
	f := Fingerprint{CatID: cat, Version: 1, Metrics: map[string]MetricBaseline{}, Shapes: map[string]int{}, Categories: map[string]map[string]int{}, Centers: map[string]float64{}, Scales: map[string]float64{}, RecordedDates: []string{}, Quality: BaselineQuality{State: "collecting", Reasons: []string{}, Separation: "unassessed", NormalValidation: "insufficient", LocalizationValidation: "unvalidated"}}
	eligible := []model.Record{}
	for _, r := range rs {
		if r.Mode == "baseline" && r.CatID != nil && *r.CatID == cat && r.Kind == kind && r.LitterKey == key && r.OccurredAt.Before(cutoff) && measurementOK(r) && (r.Integrity == "complete" || r.Integrity == "unknown" || r.Integrity == "") && (knownIdentity(r) || r.IdentityMethod == "unknown" || r.IdentityMethod == "") {
			v := r.BaselineVersion
			if v == 0 {
				v = 1
			}
			if v > f.Version {
				f.Version = v
			}
			eligible = append(eligible, r)
		}
	}
	rows := []model.Record{}
	for _, r := range eligible {
		v := r.BaselineVersion
		if v == 0 {
			v = 1
		}
		if v == f.Version {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].OccurredAt.Equal(rows[j].OccurredAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].OccurredAt.Before(rows[j].OccurredAt)
	})
	unique := map[string]model.Record{}
	for _, r := range rows {
		k := eventKey(r)
		if _, ok := unique[k]; !ok {
			unique[k] = r
		}
	}
	events := []model.Record{}
	for _, r := range unique {
		events = append(events, r)
	}
	sort.Slice(events, func(i, j int) bool {
		a, b := sampleDate(events[i], loc), sampleDate(events[j], loc)
		if a == b {
			return events[i].ID < events[j].ID
		}
		return a < b
	})
	dates := map[string]bool{}
	for _, r := range events {
		d := sampleDate(r, loc)
		if !dates[d] && len(dates) >= 7 {
			continue
		}
		dates[d] = true
		f.Training = append(f.Training, r)
	}
	for d := range dates {
		f.RecordedDates = append(f.RecordedDates, d)
	}
	sort.Strings(f.RecordedDates)
	f.Days = len(dates)
	f.Samples = len(f.Training)
	f.Quality.DistinctEvents = f.Samples
	if f.Days > 0 {
		start, _ := time.ParseInLocation("2006-01-02", f.RecordedDates[0], loc)
		end, _ := time.ParseInLocation("2006-01-02", f.RecordedDates[f.Days-1], loc)
		end = end.AddDate(0, 0, 1)
		f.Start = &start
		f.End = &end
	}
	raw := map[string][]float64{}
	for _, r := range f.Training {
		if knownIdentity(r) {
			f.Quality.ConfirmedEvents++
		}
		if r.Integrity == "complete" {
			f.Quality.CompleteEvents++
		}
		for k, v := range features(r) {
			raw[k] = append(raw[k], v)
		}
		for k, p := range map[string]*float64{"length": r.LengthCM, "width": r.WidthCM, "thickness": r.ThicknessCM} {
			if p != nil {
				raw[k] = append(raw[k], *p)
			}
		}
		for k, v := range categories(r) {
			if f.Categories[k] == nil {
				f.Categories[k] = map[string]int{}
			}
			f.Categories[k][v]++
		}
		if r.Shape != "" {
			f.Shapes[r.Shape]++
		}
	}
	for k, vs := range raw {
		if len(vs) >= 6 {
			m, s := robust(vs, floors[k])
			f.Metrics[k] = MetricBaseline{m, s, len(vs)}
		}
	}
	f.Ready = f.Days >= 7 && f.End != nil && !cutoff.Before(*f.End) && f.Metrics["grams"].Samples >= 6
	if !f.Ready {
		f.Quality.Reasons = append(f.Quality.Reasons, "尚缺7个独立采样日、完整窗口或足够重量样本")
	} else {
		f.Quality.State = "ready"
	}
	if f.Quality.ConfirmedEvents < f.Samples || f.Quality.CompleteEvents < f.Samples {
		f.Quality.State = "legacy_quality"
		f.Quality.Reasons = append(f.Quality.Reasons, "存在身份方式或完整性未确认的旧样本；仅支持基础观察")
	}
	depths := []float64{}
	missingDepth := false
	for _, r := range f.Training {
		if r.LitterDepthCM == nil {
			missingDepth = true
		} else {
			depths = append(depths, *r.LitterDepthCM)
		}
	}
	if len(depths) > 0 {
		sort.Float64s(depths)
		if missingDepth || depths[len(depths)-1]-depths[0] > 0.5 {
			f.Quality.State = "conditions_mixed"
			f.Quality.Reasons = append(f.Quality.Reasons, "基准铺砂厚度不一致或部分缺失，请独立重建版本")
		}
	}
	frozenRows := []model.Record{}
	for _, r := range rows {
		if f.End != nil && r.OccurredAt.Before(*f.End) {
			frozenRows = append(frozenRows, r)
		}
	}
	f.TimeModel = fitAging(frozenRows, f.Training)
	f.Measurements = frozenRows
	initializeDensity(&f)
	return f
}

// Only paired measurements with a consistent formed timestamp and confirmed
// identity estimate aging. Between-cat variation can never train an aging slope.
func fitAging(rows, training []model.Record) AgingModel {
	a := AgingModel{Status: "untrained", Slopes: map[string]float64{}}
	allowed := map[string]bool{}
	groups := map[string][]model.Record{}
	depths := []float64{}
	for _, r := range training {
		allowed[eventKey(r)] = true
		if h, ok := knownAge(r); ok && r.Integrity == "complete" && knownIdentity(r) {
			if a.KnownEvents == 0 {
				a.MinHours = h
				a.MaxHours = h
			}
			a.MinHours = math.Min(a.MinHours, h)
			a.MaxHours = math.Max(a.MaxHours, h)
			a.KnownEvents++
		}
		if r.LitterDepthCM != nil {
			depths = append(depths, *r.LitterDepthCM)
		}
	}
	if len(depths) > 0 {
		d := median(depths)
		a.DepthCM = &d
	}
	for _, r := range rows {
		if allowed[eventKey(r)] && r.EventID != "" && knownIdentity(r) && r.Integrity == "complete" && r.FormedAt != nil {
			groups[r.EventID] = append(groups[r.EventID], r)
		}
	}
	slopes := map[string][]float64{}
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool { return group[i].OccurredAt.Before(group[j].OccurredAt) })
		first := group[0]
		h0, _ := knownAge(first)
		pairSlopes := map[string][]float64{}
		for _, r := range group[1:] {
			h, ok := knownAge(r)
			if !ok || !r.FormedAt.Equal(*first.FormedAt) || h-h0 < 1 || (r.LitterDepthCM == nil) != (first.LitterDepthCM == nil) || r.LitterDepthCM != nil && math.Abs(*r.LitterDepthCM-*first.LitterDepthCM) > 0.5 {
				continue
			}
			for k, v := range continuous(r) {
				if v0, yes := continuous(first)[k]; yes {
					pairSlopes[k] = append(pairSlopes[k], (v-v0)/(h-h0))
				}
			}
			a.MinHours = math.Min(a.MinHours, h)
			a.MaxHours = math.Max(a.MaxHours, h)
		}
		if len(pairSlopes["grams"]) > 0 {
			a.PairedEvents++
			for k, vs := range pairSlopes {
				slopes[k] = append(slopes[k], median(vs))
			}
		}
	}
	if a.PairedEvents >= 3 {
		a.Status = "paired"
		for k, vs := range slopes {
			if len(vs) >= 3 {
				a.Slopes[k] = median(vs)
			}
		}
	} else if a.KnownEvents > 0 {
		a.Status = "measured_range"
	}
	return a
}
func corrected(r model.Record, a AgingModel, h float64) map[string]float64 {
	out := continuous(r)
	for k, v := range out {
		out[k] = v - a.Slopes[k]*h
	}
	return out
}
func initializeDensity(f *Fingerprint) {
	f.CategoryAges = map[string]map[string]map[string]int{}
	seen := map[string]bool{}
	for _, r := range f.Measurements {
		h, ok := knownAge(r)
		if !ok || !knownIdentity(r) || r.Integrity != "complete" {
			continue
		}
		bin := ageBin(h)
		key := eventKey(r) + ":" + bin
		if seen[key] {
			continue
		}
		seen[key] = true
		if f.CategoryAges[bin] == nil {
			f.CategoryAges[bin] = map[string]map[string]int{}
		}
		for k, v := range categories(r) {
			if f.CategoryAges[bin][k] == nil {
				f.CategoryAges[bin][k] = map[string]int{}
			}
			f.CategoryAges[bin][k][v]++
		}
	}
	values := map[string][]float64{}
	for _, r := range f.Training {
		h, _ := knownAge(r)
		for k, v := range corrected(r, f.TimeModel, h) {
			values[k] = append(values[k], v)
		}
	}
	for _, k := range continuousKeys {
		if len(values[k]) >= 6 {
			floor := 0.08
			if k == "grams" {
				floor = math.Max(0.05, math.Log1p(5/math.Exp(median(values[k]))))
			}
			m, s := robust(values[k], floor)
			f.Keys = append(f.Keys, k)
			f.Centers[k] = m
			f.Scales[k] = s
		}
	}
	f.Covariance = identity(len(f.Keys))
}
func ageBin(h float64) string {
	for _, bound := range []float64{1, 6, 24, 72} {
		if h <= bound {
			return fmt.Sprint(bound)
		}
	}
	return "older"
}
func shareAging(fs []Fingerprint) {
	rows, training := []model.Record{}, []model.Record{}
	for _, f := range fs {
		rows = append(rows, f.Measurements...)
		training = append(training, f.Training...)
	}
	a := fitAging(rows, training)
	if a.Status != "paired" {
		return
	}
	for i := range fs {
		if fs[i].TimeModel.KnownEvents < 3 {
			continue
		}
		own := fs[i].TimeModel
		fs[i].TimeModel = a
		fs[i].TimeModel.KnownEvents = own.KnownEvents
		fs[i].TimeModel.DepthCM = own.DepthCM
		fs[i].Keys = nil
		fs[i].Centers = map[string]float64{}
		fs[i].Scales = map[string]float64{}
		initializeDensity(&fs[i])
	}
}
func identity(n int) [][]float64 {
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		m[i][i] = 1
	}
	return m
}

// Pooled standardized within-cat covariance. Half shrinkage toward the identity
// and a variance floor make the small low-dimensional matrices positive definite.
func shrinkCovariance(fs []Fingerprint) {
	keys := []string{}
	for _, k := range continuousKeys {
		all := len(fs) > 0
		for _, f := range fs {
			if _, ok := f.Centers[k]; !ok {
				all = false
			}
		}
		if all {
			keys = append(keys, k)
		}
	}
	n := len(keys)
	if n == 0 {
		return
	}
	rows := [][]float64{}
	for _, f := range fs {
		for _, r := range f.Training {
			h, _ := knownAge(r)
			vals := corrected(r, f.TimeModel, h)
			row := make([]float64, n)
			ok := true
			for i, k := range keys {
				v, has := vals[k]
				if !has {
					ok = false
					break
				}
				row[i] = math.Max(-4, math.Min(4, (v-f.Centers[k])/f.Scales[k]))
			}
			if ok {
				rows = append(rows, row)
			}
		}
	}
	cov := identity(n)
	if len(rows) >= 2*n+3 {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				v := 0.0
				for _, row := range rows {
					v += row[i] * row[j]
				}
				cov[i][j] = 0.5 * v / float64(len(rows))
				if i == j {
					cov[i][j] += 0.5
				}
			}
		}
	}
	for i := range fs {
		fs[i].Keys = append([]string{}, keys...)
		fs[i].Covariance = cov
	}
}
func gaussianLoss(z []float64, cov [][]float64) float64 {
	n := len(z)
	if n == 0 {
		return math.Inf(1)
	}
	l := make([][]float64, n)
	for i := 0; i < n; i++ {
		l[i] = make([]float64, n)
		for j := 0; j <= i; j++ {
			v := cov[i][j]
			for k := 0; k < j; k++ {
				v -= l[i][k] * l[j][k]
			}
			if i == j {
				l[i][j] = math.Sqrt(math.Max(v, 0.01))
			} else {
				l[i][j] = v / l[j][j]
			}
		}
	}
	y := make([]float64, n)
	sum, logdet := 0.0, 0.0
	for i := 0; i < n; i++ {
		v := z[i]
		for j := 0; j < i; j++ {
			v -= l[i][j] * y[j]
		}
		y[i] = v / l[i][i]
		sum += y[i] * y[i]
		logdet += 2 * math.Log(l[i][i])
	}
	return 0.5 * (sum + logdet + float64(n)*math.Log(2*math.Pi))
}

type agePoint struct{ hours, weight float64 }

func ages(r model.Record, a AgingModel, scenario int) ([]agePoint, string) {
	if a.KnownEvents < 3 {
		return nil, "time_untrained"
	}
	lo, hi := 0.0, 0.0
	if h, ok := knownAge(r); ok {
		lo = h
		hi = h
	} else if r.LastCleaningComplete && r.LastCleanedAt != nil && r.LastCleanedAt.Before(r.OccurredAt) {
		hi = r.OccurredAt.Sub(*r.LastCleanedAt).Hours()
	} else {
		return nil, "time_unknown"
	}
	tolerance := 1.0
	if a.Status == "paired" {
		tolerance = 0.25
	}
	if lo < a.MinHours-tolerance || hi > a.MaxHours+tolerance {
		return nil, "time_out_of_range"
	}
	if a.DepthCM != nil && (r.LitterDepthCM == nil || math.Abs(*r.LitterDepthCM-*a.DepthCM) > 0.5) {
		return nil, "conditions_changed"
	}
	if hi == lo {
		return []agePoint{{lo, 1}}, "known"
	}
	pts := []agePoint{}
	total := 0.0
	for i := 0; i < 9; i++ {
		h := lo + (hi-lo)*float64(i)/8
		w := 1.0
		if i == 0 || i == 8 {
			w = 0.5
		}
		if scenario == 1 {
			w *= math.Exp(-3 * h / math.Max(hi, 0.01))
		}
		if scenario == 2 {
			w *= math.Exp(3 * h / math.Max(hi, 0.01))
		}
		pts = append(pts, agePoint{h, w})
		total += w
	}
	for i := range pts {
		pts[i].weight /= total
	}
	return pts, "interval"
}
func logsum(xs []float64) float64 {
	if len(xs) == 0 {
		return math.Inf(-1)
	}
	m := xs[0]
	for _, x := range xs {
		m = math.Max(m, x)
	}
	if math.IsInf(m, -1) {
		return m
	}
	s := 0.0
	for _, x := range xs {
		s += math.Exp(x - m)
	}
	return m + math.Log(s)
}

// Submatrices marginalize missing optional numeric measurements; missing grams,
// unknown integrity and unsupported time never receive high-confidence evidence.
func itemLikelihood(r model.Record, f Fingerprint, scenario int, shift map[string]float64) (float64, string, float64) {
	if f.Quality.State == "conditions_mixed" {
		return math.Inf(-1), "conditions_changed", 0
	}
	if f.Quality.State != "ready" {
		return math.Inf(-1), "baseline_quality", 0
	}
	if r.Integrity != "complete" {
		return math.Inf(-1), "sample_" + r.Integrity, 0
	}
	if r.Grams == nil || r.Count != 1 {
		return math.Inf(-1), "missing_metrics", 0
	}
	pts, state := ages(r, f.TimeModel, scenario)
	if len(pts) == 0 {
		return math.Inf(-1), state, 0
	}
	vals := continuous(r)
	indices := []int{}
	keys := []string{}
	for i, k := range f.Keys {
		if _, ok := vals[k]; ok {
			indices = append(indices, i)
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return math.Inf(-1), "missing_metrics", 0
	}
	cov := make([][]float64, len(keys))
	for i, a := range indices {
		cov[i] = make([]float64, len(keys))
		for j, b := range indices {
			cov[i][j] = f.Covariance[a][b]
		}
	}
	scores := []float64{}
	bestZ := math.Inf(1)
	for _, p := range pts {
		z := []float64{}
		maxZ := 0.0
		scaleLog := 0.0
		for _, k := range keys {
			sd := f.Scales[k]
			v := (vals[k] - f.TimeModel.Slopes[k]*p.hours - f.Centers[k] - shift[k]) / sd
			z = append(z, v)
			maxZ = math.Max(maxZ, math.Abs(v))
			scaleLog += math.Log(sd)
		}
		bestZ = math.Min(bestZ, maxZ)
		nll := gaussianLoss(z, cov) + scaleLog
		for k, v := range categories(r) {
			counts := f.CategoryAges[ageBin(p.hours)][k]
			total := 0
			for _, n := range counts {
				total += n
			}
			if total >= 6 {
				prob := float64(counts[v]+1) / float64(total+5)
				nll -= math.Log(prob)
				if counts[v] == 0 && float64(total) >= 10 {
					bestZ = math.Max(bestZ, 3.5)
				}
			}
		}
		scores = append(scores, math.Log(p.weight)-nll)
	}
	return logsum(scores), state, bestZ
}

// Leave an entire event AND sampled day out; paired measurements never leak into
// the held-out side. This assesses healthy separability, not abnormal accuracy.
func validateProfiles(fs []Fingerprint, loc *time.Location) {
	for i := range fs {
		f := &fs[i]
		if !f.Ready {
			continue
		}
		f.Quality.Separation = "separable"
		for j := range fs {
			if i == j {
				continue
			}
			shared := 0
			maxZ := 0.0
			for _, k := range f.Keys {
				if v, ok := fs[j].Centers[k]; ok {
					shared++
					maxZ = math.Max(maxZ, math.Abs(f.Centers[k]-v)/math.Max(f.Scales[k], fs[j].Scales[k]))
				}
			}
			if shared == 0 || maxZ < 2 {
				f.Quality.Separation = "overlap"
			}
		}
	}
	for i := range fs {
		f := &fs[i]
		if !f.Ready || f.Quality.CompleteEvents < f.Samples || f.Quality.ConfirmedEvents < f.Samples {
			continue
		}
		for _, held := range f.Training {
			if _, ok := knownAge(held); !ok {
				continue
			}
			day := sampleDate(held, loc)
			models := make([]Fingerprint, len(fs))
			copy(models, fs)
			for j := range models {
				models[j].Training = []model.Record{}
				models[j].Measurements = []model.Record{}
				models[j].Keys = nil
				models[j].Centers = map[string]float64{}
				models[j].Scales = map[string]float64{}
				for _, r := range fs[j].Training {
					if sampleDate(r, loc) != day {
						models[j].Training = append(models[j].Training, r)
					}
				}
				for _, r := range fs[j].Measurements {
					if sampleDate(r, loc) != day {
						models[j].Measurements = append(models[j].Measurements, r)
					}
				}
				models[j].TimeModel = fitAging(models[j].Measurements, models[j].Training)
				initializeDensity(&models[j])
			}
			shareAging(models)
			shrinkCovariance(models)
			best, second := math.Inf(-1), math.Inf(-1)
			winner := -1
			for j, m := range models {
				l, _, _ := itemLikelihood(held, m, 0, nil)
				if l > best {
					second = best
					best = l
					winner = j
				} else if l > second {
					second = l
				}
			}
			if winner < 0 {
				continue
			}
			f.Quality.ValidationEvents++
			if best-second < 2 {
				continue
			}
			f.Quality.ValidationCoverage++
			if winner != i {
				f.Quality.ValidationErrors++
			}
		}
		if f.Quality.ValidationEvents >= 6 {
			f.Quality.NormalValidation = "assessed"
		}
		if f.Quality.Separation == "overlap" {
			f.Quality.Reasons = append(f.Quality.Reasons, "猫之间正常特征重叠；不得输出单猫定位")
		}
		if f.TimeModel.KnownEvents < 3 {
			f.Quality.Reasons = append(f.Quality.Reasons, "缺少形成时间，尚不能校正存放影响")
		}
	}
}

type windowModel struct {
	name  string
	cat   int
	shift map[string]float64
	rho   float64
}

func profileWindow(rows []model.Record, fs []Fingerprint, scenario int, h windowModel, weights []float64) float64 {
	score := 0.0
	for _, r := range rows {
		terms := []float64{}
		for i, f := range fs {
			base, _, _ := itemLikelihood(r, f, scenario, nil)
			value := base
			if h.name == "cat_change" && i == h.cat {
				changed, _, _ := itemLikelihood(r, f, scenario, h.shift)
				value = logsum([]float64{math.Log(1-h.rho) + base, math.Log(h.rho) + changed})
			}
			if h.name == "common_shift" {
				value, _, _ = itemLikelihood(r, f, scenario, h.shift)
			}
			if r.CatID != nil && knownIdentity(r) {
				if *r.CatID == f.CatID {
					terms = append(terms, value)
				}
			} else {
				terms = append(terms, math.Log(weights[i])+value)
			}
		}
		if h.name == "unexplained" {
			// Broad normalized contamination density in the recorded log-unit domain.
			unknown := math.Log(0.1)
			for k := range continuous(r) {
				span := math.Log(50 / 0.1)
				if k == "grams" {
					span = math.Log(2000 / 0.1)
				}
				unknown -= math.Log(span)
			}
			unknown -= float64(len(categories(r))) * math.Log(5)
			terms = append(terms, unknown)
			for i := 0; i < len(terms)-1; i++ {
				terms[i] += math.Log(0.9)
			}
		}
		s := logsum(terms)
		if math.IsInf(s, -1) {
			return s
		}
		score += s
	}
	// Penalize fitted alternative locations/fractions; large batches do not create
	// artificial certainty about the number of animals or elimination events.
	if h.name == "cat_change" {
		score -= 0.5 * float64(len(h.shift)+1) * math.Log(math.Max(2, float64(len(rows))))
	}
	if h.name == "common_shift" {
		score -= 0.5 * float64(len(h.shift)) * math.Log(math.Max(2, float64(len(rows))))
	}
	if h.name == "unexplained" {
		score -= math.Log(math.Max(2, float64(len(rows))))
	}
	return score
}
func bestWindow(rows []model.Record, fs []Fingerprint, scenario int, h windowModel) float64 {
	// Estimate mixture proportions softly, with a weak symmetric pseudocount.
	// Neither equal allocation nor a missing component establishes an identity.
	n := len(fs)
	if n == 0 {
		return math.Inf(-1)
	}
	matrix := make([][]float64, len(rows))
	for j, r := range rows {
		matrix[j] = make([]float64, n)
		for i, f := range fs {
			base, _, _ := itemLikelihood(r, f, scenario, nil)
			v := base
			if h.name == "cat_change" && i == h.cat {
				changed, _, _ := itemLikelihood(r, f, scenario, h.shift)
				v = logsum([]float64{math.Log(1-h.rho) + base, math.Log(h.rho) + changed})
			}
			if h.name == "common_shift" {
				v, _, _ = itemLikelihood(r, f, scenario, h.shift)
			}
			matrix[j][i] = v
		}
	}
	weights := make([]float64, n)
	for i := range weights {
		weights[i] = 1 / float64(n)
	}
	for iteration := 0; iteration < 12; iteration++ {
		totals := make([]float64, n)
		for i := range totals {
			totals[i] = 0.5
		}
		for rowIndex, row := range matrix {
			if rows[rowIndex].CatID != nil && knownIdentity(rows[rowIndex]) {
				continue
			}
			terms := make([]float64, n)
			for i := range row {
				terms[i] = math.Log(weights[i]) + row[i]
			}
			denom := logsum(terms)
			if math.IsInf(denom, -1) {
				return denom
			}
			for i := range terms {
				totals[i] += math.Exp(terms[i] - denom)
			}
		}
		sum := 0.0
		for _, v := range totals {
			sum += v
		}
		for i := range weights {
			weights[i] = totals[i] / sum
		}
	}
	return profileWindow(rows, fs, scenario, h, weights)
}
func shiftChoices(rows []model.Record, fs []Fingerprint, cat int, scenario int) []map[string]float64 {
	values := map[string][]float64{}
	f := fs[cat]
	for _, r := range rows {
		pts, _ := ages(r, f.TimeModel, scenario)
		if len(pts) == 0 {
			continue
		}
		age := 0.0
		for _, p := range pts {
			age += p.hours * p.weight
		}
		for _, k := range f.Keys {
			if v, ok := continuous(r)[k]; ok {
				values[k] = append(values[k], v-f.TimeModel.Slopes[k]*age-f.Centers[k])
			}
		}
	}
	out := []map[string]float64{}
	for _, q := range []float64{0, 0.25, 0.5, 0.75, 1} {
		s := map[string]float64{}
		for k, vs := range values {
			sort.Float64s(vs)
			s[k] = vs[int(math.Round(q*float64(len(vs)-1)))]
		}
		out = append(out, s)
	}
	return out
}
func bestChanged(rows []model.Record, fs []Fingerprint, scenario, cat int) (float64, windowModel) {
	best := math.Inf(-1)
	chosen := windowModel{}
	for _, shift := range shiftChoices(rows, fs, cat, scenario) {
		for _, rho := range []float64{0.25, 0.5, 0.75, 0.95} {
			h := windowModel{"cat_change", cat, shift, rho}
			s := bestWindow(rows, fs, scenario, h)
			if s > best {
				best = s
				chosen = h
			}
		}
	}
	return best, chosen
}
func bestCommon(rows []model.Record, fs []Fingerprint, scenario int) float64 {
	best := math.Inf(-1)
	for c := range fs {
		for _, shift := range shiftChoices(rows, fs, c, scenario) {
			best = math.Max(best, bestWindow(rows, fs, scenario, windowModel{"common_shift", -1, shift, 0}))
		}
	}
	return best
}
func compareBatches(rows []model.Record, fs []Fingerprint) BatchComparison {
	b := BatchComparison{State: "insufficient", Reason: "继续逐块采集；定位需要完整样本、时间覆盖和多个清理批次", Hypotheses: []Hypothesis{}, Suspects: []Suspect{}}
	unique := map[string]bool{}
	batches := map[string]bool{}
	valid := []model.Record{}
	for _, r := range rows {
		k := eventKey(r)
		if unique[k] {
			continue
		}
		unique[k] = true
		ok := r.Integrity == "complete" && r.Grams != nil && !r.Blood && !r.NoOutput && r.Count == 1
		for _, f := range fs {
			if !f.Ready || f.Quality.CompleteEvents < f.Samples || f.Quality.ConfirmedEvents < f.Samples {
				ok = false
			}
			l, _, _ := itemLikelihood(r, f, 0, nil)
			if math.IsInf(l, -1) {
				ok = false
			}
		}
		if ok {
			valid = append(valid, r)
			if r.BatchID != "" {
				batches[r.BatchID] = true
			}
		}
	}
	b.Events = len(valid)
	b.Batches = len(batches)
	if float64(len(valid)) < math.Max(4, float64(len(fs))) || len(fs) < 2 {
		return b
	}
	base := bestWindow(valid, fs, 0, windowModel{name: "healthy"})
	bestName, bestCat := "healthy", -1
	best := base
	b.Hypotheses = append(b.Hypotheses, Hypothesis{Name: "healthy", Evidence: round2(base)})
	gains := make([]float64, len(fs))
	for c := range fs {
		value, modelChoice := bestChanged(valid, fs, 0, c)
		gains[c] = value - base
		b.Hypotheses = append(b.Hypotheses, Hypothesis{"cat_change", fs[c].CatID, round2(value), round2(value - base), modelChoice.rho})
		if value > best {
			best = value
			bestName = "cat_change"
			bestCat = c
		}
	}
	common := bestCommon(valid, fs, 0)
	b.Hypotheses = append(b.Hypotheses, Hypothesis{Name: "common_shift", Evidence: round2(common), Gain: round2(common - base)})
	if common > best {
		best = common
		bestName = "common_shift"
		bestCat = -1
	}
	other := bestWindow(valid, fs, 0, windowModel{name: "unexplained"})
	b.Hypotheses = append(b.Hypotheses, Hypothesis{Name: "unexplained", Evidence: round2(other), Gain: round2(other - base)})
	if other > best {
		best = other
		bestName = "unexplained"
		bestCat = -1
	}
	b.Best = bestName
	b.TimeStable = true
	for scenario := 1; scenario <= 2; scenario++ {
		baseline := bestWindow(valid, fs, scenario, windowModel{name: "healthy"})
		winner, wCat := "healthy", -1
		score := baseline
		for c := range fs {
			s, _ := bestChanged(valid, fs, scenario, c)
			if s > score {
				score = s
				winner = "cat_change"
				wCat = c
			}
		}
		for _, name := range []string{"common_shift", "unexplained"} {
			s := bestCommon(valid, fs, scenario)
			if name == "unexplained" {
				s = bestWindow(valid, fs, scenario, windowModel{name: "unexplained"})
			}
			if s > score {
				score = s
				winner = name
				wCat = -1
			}
		}
		if winner != bestName || wCat != bestCat {
			b.TimeStable = false
		}
	}
	for c, f := range fs {
		if gains[c] > 0 {
			b.Suspects = append(b.Suspects, Suspect{CatID: f.CatID, Gain: round2(gains[c]), Reason: fmt.Sprintf("允许该猫部分样本变化，惩罚后对数证据改善 %.2f；未校准，不是概率", gains[c])})
		}
	}
	sort.Slice(b.Suspects, func(i, j int) bool {
		if b.Suspects[i].Gain == b.Suspects[j].Gain {
			return b.Suspects[i].CatID < b.Suspects[j].CatID
		}
		return b.Suspects[i].Gain > b.Suspects[j].Gain
	})
	for i := range b.Suspects {
		b.Suspects[i].Rank = i + 1
	}
	if best-base < 3 {
		b.State = "compatible"
		b.Reason = "正常波动、使用比例和时间模型可以解释目前样本；这不确认每只猫都健康"
		return b
	}
	b.State = "ambiguous"
	b.Reason = "整批分布发生变化，但证据不足以定位单猫"
	if bestName == "common_shift" {
		b.State = "common_shift"
		b.Reason = "共同偏移解释更强；环境或共同状态变化均可能，暂不指定单猫"
		return b
	}
	if bestName == "unexplained" {
		b.State = "unexplained"
		b.Reason = "单猫与共同变化均解释不足，可能多猫变化或训练未覆盖"
		return b
	}
	separated := true
	for _, f := range fs {
		if f.Quality.Separation != "separable" || f.Quality.NormalValidation != "assessed" || f.Quality.ValidationErrors > 0 || f.Quality.ValidationCoverage < f.Quality.ValidationEvents/2 {
			separated = false
		}
	}
	second := math.Max(base, math.Max(common, other))
	for c, g := range gains {
		if c != bestCat {
			second = math.Max(second, base+g)
		}
	}
	healthySupport := bestCat >= 0
	if healthySupport {
		for other := range fs {
			if other == bestCat {
				continue
			}
			supported := false
			for _, r := range valid {
				own, _, z := itemLikelihood(r, fs[other], 0, nil)
				different := math.Inf(-1)
				for c := range fs {
					if c != other {
						l, _, _ := itemLikelihood(r, fs[c], 0, nil)
						different = math.Max(different, l)
					}
				}
				if z < 1.5 && own-different >= 2 {
					supported = true
					break
				}
			}
			healthySupport = healthySupport && supported
		}
	}
	completeCollections := true
	for _, r := range valid {
		if !r.CollectionComplete {
			completeCollections = false
		}
	}
	if bestCat >= 0 && separated && healthySupport && completeCollections && b.TimeStable && b.Batches >= 2 && best-second >= 2 {
		id := fs[bestCat].CatID
		b.AnchorCatID = &id
		b.State = "provisional"
		b.Reason = "多个批次及时间假设支持优先排查候选；异常定位尚未真实验证，不确认身份"
	} else if !b.TimeStable {
		b.Reason = "存放时间假设改变了候选结论，保留多个候选"
	}
	return b
}
func round2(v float64) float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0
	}
	return math.Round(v*100) / 100
}

func AnalyzePools(rs []model.Record, weights []model.Weight, boxes []model.Box, litters []model.Litter, now time.Time, locations ...*time.Location) Report {
	rep, _ := AnalyzePoolsContext(context.Background(), rs, weights, boxes, litters, now, nil, locations...)
	return rep
}
func AnalyzePoolsContext(ctx context.Context, rs []model.Record, weights []model.Weight, boxes []model.Box, litters []model.Litter, now time.Time, progress func(int, string), locations ...*time.Location) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	rep := Analyze(rs, weights, now)
	rep.CalibrationDays = 7
	loc := time.UTC
	if len(locations) > 0 && locations[0] != nil {
		loc = locations[0]
	}
	rep.CalibrationTimezone = loc.String()
	rep.Pools = []Pool{}
	type group struct {
		box       uint
		kind, key string
		cats      []uint
		rows      []model.Record
	}
	groups := map[string]*group{}
	groupKey := func(box uint, kind, key string, cats []uint) string {
		ids := append([]uint{}, cats...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		parts := []string{}
		for _, id := range ids {
			parts = append(parts, strconv.Itoa(int(id)))
		}
		return fmt.Sprintf("%d:%s:%s:%s", box, kind, key, strings.Join(parts, ","))
	}
	for _, box := range boxes {
		if box.Archived {
			continue
		}
		for _, l := range litters {
			if box.LitterID == l.ID {
				for _, kind := range []string{"urine", "stool"} {
					key := fmt.Sprintf("%d:v%d", l.ID, l.Version)
					k := groupKey(box.ID, kind, key, box.CatIDs)
					groups[k] = &group{box.ID, kind, key, box.CatIDs, []model.Record{}}
				}
			}
		}
	}
	for _, r := range rs {
		if r.Mode != "shared" || r.OccurredAt.After(now) || now.Sub(r.OccurredAt) > 7*24*time.Hour {
			continue
		}
		rep.SharedCount += r.Count
		if now.Sub(r.OccurredAt) > 3*24*time.Hour {
			continue
		}
		k := groupKey(r.BoxID, r.Kind, r.LitterKey, r.EligibleCatIDs)
		if groups[k] == nil {
			groups[k] = &group{r.BoxID, r.Kind, r.LitterKey, r.EligibleCatIDs, []model.Record{}}
		}
		groups[k].rows = append(groups[k].rows, r)
	}
	keys := []string{}
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for index, key := range keys {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		if progress != nil {
			progress(5+15*index/max(1, len(keys)), "建立基准与时间覆盖")
		}
		g := groups[key]
		cutoff := now
		for _, r := range g.rows {
			if r.OccurredAt.Before(cutoff) {
				cutoff = r.OccurredAt
			}
		}
		pool := Pool{BoxID: g.box, Kind: g.kind, LitterKey: g.key, Status: "need_baseline", ModelVersion: "joint-mixture-v3", Profiles: []Fingerprint{}, Observations: []PoolObservation{}}
		ready := len(g.cats) > 0
		for _, c := range g.cats {
			f := fingerprint(c, g.kind, g.key, rs, cutoff, loc)
			pool.Profiles = append(pool.Profiles, f)
			ready = ready && f.Ready
		}
		shareAging(pool.Profiles)
		shrinkCovariance(pool.Profiles)
		validateProfiles(pool.Profiles, loc)
		if ready {
			pool.Status = "ready"
			for _, f := range pool.Profiles {
				if f.Quality.State != "ready" || f.TimeModel.KnownEvents < 3 {
					pool.Status = "quality_limited"
				}
			}
		}
		latest := model.Record{}
		maxZ := 0.0
		for _, r := range g.rows {
			o := PoolObservation{RecordID: r.ID, Status: "need_baseline", CandidateIDs: []uint{}}
			if !ready {
				pool.Observations = append(pool.Observations, o)
				continue
			}
			if r.Integrity != "complete" || r.Count != 1 || r.NoOutput || r.Grams == nil {
				o.Status = "unusable"
				o.Reason = "完整性或关键测量不足，保留采集，不作单块高置信度判断"
				pool.Skipped++
				pool.Observations = append(pool.Observations, o)
				continue
			}
			pool.Inspected++
			novel := math.Inf(1)
			supported := true
			for _, f := range pool.Profiles {
				l, t, z := itemLikelihood(r, f, 0, nil)
				o.TimeStatus = t
				if math.IsInf(l, -1) {
					supported = false
				}
				novel = math.Min(novel, z)
			}
			if supported {
				o.Status = "within_baseline"
				o.Novelty = round2(novel)
				if novel >= 3.5 {
					o.Status = "anomaly"
					o.Reason = "在覆盖的存放时间内偏离所有正常基准"
				}
				candidateSet := map[uint]bool{}
				for scenario := 0; scenario < 3; scenario++ {
					scores := []float64{}
					best := math.Inf(-1)
					for _, f := range pool.Profiles {
						l, _, _ := itemLikelihood(r, f, scenario, nil)
						scores = append(scores, l)
						best = math.Max(best, l)
					}
					for i, l := range scores {
						if best-l <= 2 {
							candidateSet[pool.Profiles[i].CatID] = true
						}
					}
				}
				for _, f := range pool.Profiles {
					if candidateSet[f.CatID] {
						o.CandidateIDs = append(o.CandidateIDs, f.CatID)
					}
				}
				if len(o.CandidateIDs) > 1 {
					o.Reason = "正常模型下多个来源合理；时间不确定时保留候选集合，不确认归属"
				}
			} else { // Uncorrected MAD is an observation warning only; never a time-adjusted verdict.
				_, z, _, _ := nearest(r, pool.Profiles, -1, nil, "")
				o.Novelty = round2(z)
				o.Status = "time_unresolved"
				o.Reason = "存放时间或条件未覆盖；原始尺寸变化只能作待复测线索"
				if z >= 3.5 {
					o.Status = "measurement_change"
				}
			}
			if o.Status == "anomaly" || o.Status == "measurement_change" {
				pool.Anomalies++
				rep.SharedAnomalies++
				maxZ = math.Max(maxZ, o.Novelty)
				if r.OccurredAt.After(latest.OccurredAt) {
					latest = r
				}
			}
			pool.Observations = append(pool.Observations, o)
		}
		pool.BatchAnalysis = BatchComparison{State: "pending_joint", Reason: "由联合组合模型比较，不强行单猫归属", Hypotheses: []Hypothesis{}, Suspects: []Suspect{}}
		b := pool.BatchAnalysis
		distributionChange := b.State == "ambiguous" || b.State == "provisional" || b.State == "common_shift" || b.State == "unexplained"
		if pool.Anomalies > 0 || distributionChange {
			pool.Status = "anomaly"
			if latest.ID == 0 && len(g.rows) > 0 {
				latest = g.rows[0]
			}
			detail := fmt.Sprintf("近3天 %d 块逐块测量，%d 块变化线索。%s", pool.Inspected, pool.Anomalies, b.Reason)
			rep.Alerts = append(rep.Alerts, Alert{BoxID: g.box, RecordID: latest.ID, Level: "watch", Title: "共享池出现变化", Detail: detail, Metric: "shared_pool", Z: round2(maxZ), Common: b.State == "common_shift", Suspects: b.Suspects, Localization: b.State, AnchorCatID: b.AnchorCatID})
		}
		rep.Pools = append(rep.Pools, pool)
	}
	sort.SliceStable(rep.Alerts, func(i, j int) bool { return rep.Alerts[i].Level == "urgent" && rep.Alerts[j].Level != "urgent" })
	if err := analyzeJoint(ctx, &rep, rs, now, progress); err != nil {
		return Report{}, err
	}
	if progress != nil {
		progress(100, "分析完成")
	}
	return rep, nil
}
