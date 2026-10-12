package analysis

import (
	"math"
	"time"

	"catcare/internal/model"
)

type MetricBaseline struct {
	Median  float64 `json:"median"`
	Scale   float64 `json:"scale"`
	Samples int     `json:"samples"`
}
type Fingerprint struct {
	Version       int                                  `json:"version"`
	Quality       BaselineQuality                      `json:"quality"`
	TimeModel     AgingModel                           `json:"time_model"`
	Categories    map[string]map[string]int            `json:"categories"`
	CategoryAges  map[string]map[string]map[string]int `json:"-"`
	Keys          []string                             `json:"-"`
	Centers       map[string]float64                   `json:"-"`
	Scales        map[string]float64                   `json:"-"`
	Covariance    [][]float64                          `json:"-"`
	Training      []model.Record                       `json:"-"`
	Measurements  []model.Record                       `json:"-"`
	RecordedDates []string                             `json:"recorded_dates"`
	CatID         uint                                 `json:"cat_id"`
	Days          int                                  `json:"days"`
	Samples       int                                  `json:"samples"`
	Ready         bool                                 `json:"ready"`
	Start         *time.Time                           `json:"start"`
	End           *time.Time                           `json:"end"`
	Metrics       map[string]MetricBaseline            `json:"metrics"`
	Shapes        map[string]int                       `json:"shapes"`
}
type PoolObservation struct {
	RecordID     uint    `json:"record_id"`
	Status       string  `json:"status"`
	Novelty      float64 `json:"novelty"`
	TimeStatus   string  `json:"time_status"`
	Reason       string  `json:"reason"`
	CandidateIDs []uint  `json:"candidate_ids"`
}
type Suspect struct {
	CatID  uint    `json:"cat_id"`
	Gain   float64 `json:"gain"`
	Reason string  `json:"reason"`
	Rank   int     `json:"rank"`
}
type Pool struct {
	BoxID         uint              `json:"box_id"`
	Kind          string            `json:"kind"`
	LitterKey     string            `json:"litter_key"`
	Status        string            `json:"status"`
	Profiles      []Fingerprint     `json:"profiles"`
	Inspected     int               `json:"inspected"`
	Anomalies     int               `json:"anomalies"`
	Skipped       int               `json:"skipped"`
	Observations  []PoolObservation `json:"observations"`
	BatchAnalysis BatchComparison   `json:"batch_analysis"`
	ModelVersion  string            `json:"model_version"`
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
