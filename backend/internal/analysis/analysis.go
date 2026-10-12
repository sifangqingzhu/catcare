// Package analysis deliberately never turns statistical matches into confirmed identities.
package analysis

import (
	"catcare/internal/model"
	"fmt"
	"math"
	"sort"
	"time"
)

type Candidate struct {
	CatID    uint    `json:"cat_id"`
	Score    float64 `json:"score"`
	Evidence int     `json:"evidence"`
}
type Match struct {
	RecordID   uint        `json:"record_id"`
	Status     string      `json:"status"`
	Candidates []Candidate `json:"candidates"`
	Reason     string      `json:"reason"`
}
type Alert struct {
	CatID        *uint     `json:"cat_id"`
	BoxID        uint      `json:"box_id"`
	RecordID     uint      `json:"record_id"`
	Level        string    `json:"level"`
	Title        string    `json:"title"`
	Detail       string    `json:"detail"`
	Metric       string    `json:"metric"`
	Z            float64   `json:"z"`
	Common       bool      `json:"common"`
	Suspects     []Suspect `json:"suspects,omitempty"`
	Localization string    `json:"localization,omitempty"`
	AnchorCatID  *uint     `json:"anchor_cat_id,omitempty"`
	Direction    float64   `json:"-"`
	Key          string    `json:"-"`
}
type Report struct {
	CalibrationDays     int               `json:"calibration_days"`
	CalibrationTimezone string            `json:"calibration_timezone"`
	Matches             []Match           `json:"matches"`
	Alerts              []Alert           `json:"alerts"`
	Confirmed           int               `json:"confirmed"`
	Unknown             int               `json:"unknown"`
	BaselineDays        int               `json:"baseline_days"`
	Pools               []Pool            `json:"pools"`
	SharedCount         int               `json:"shared_count"`
	SharedAnomalies     int               `json:"shared_anomalies"`
	JointAnalyses       []JointComparison `json:"joint_analyses"`
}

func median(xs []float64) float64 {
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	n := len(ys)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return ys[n/2]
	}
	return (ys[n/2-1] + ys[n/2]) / 2
}
func robust(xs []float64, floor float64) (float64, float64) {
	m := median(xs)
	ds := make([]float64, len(xs))
	for i, x := range xs {
		ds[i] = math.Abs(x - m)
	}
	return m, math.Max(floor, 1.4826*median(ds))
}
func features(r model.Record) map[string]float64 {
	f := map[string]float64{}
	if r.NoOutput || r.Count != 1 {
		return f
	}
	if r.Grams != nil {
		f["grams"] = *r.Grams
	}
	if r.Kind == "urine" {
		if r.DiameterCM != nil {
			f["diameter"] = *r.DiameterCM
		}
		if r.Looseness != nil {
			f["looseness"] = float64(*r.Looseness)
		}
	}
	if r.Kind == "stool" {
		if r.Moisture != nil {
			f["moisture"] = float64(*r.Moisture)
		}
		if r.Size != nil {
			f["size"] = float64(*r.Size)
		}
	}
	return f
}

var floors = map[string]float64{"grams": 5, "diameter": 0.5, "looseness": 0.6, "moisture": 0.6, "size": 0.6}
var names = map[string]string{"grams": "重量", "diameter": "直径", "looseness": "松散度", "moisture": "干湿度", "size": "大小"}

func baseline(cat uint, r model.Record, rs []model.Record, cutoff time.Time) map[string][]float64 {
	out := map[string][]float64{}
	for _, p := range rs {
		if p.CatID == nil || *p.CatID != cat || p.ID == r.ID || p.Kind != r.Kind || p.BoxID != r.BoxID || p.LitterKey != r.LitterKey || !p.OccurredAt.Before(cutoff) || p.OccurredAt.Before(cutoff.AddDate(0, 0, -28)) {
			continue
		}
		for k, v := range features(p) {
			out[k] = append(out[k], v)
		}
	}
	return out
}
func match(r model.Record, rs []model.Record, weights []model.Weight) Match {
	m := Match{RecordID: r.ID, Status: "unknown", Candidates: []Candidate{}, Reason: "缺少可区分证据，保留未知归属"}
	if r.CatID != nil {
		m.Status = "confirmed"
		m.Reason = "人工确认归属"
		return m
	}
	if r.Count != 1 || r.NoOutput {
		m.Reason = "混合收集或无产出记录无法按单次特征归属"
		return m
	}
	for _, cat := range r.EligibleCatIDs {
		b := baseline(cat, r, rs, r.OccurredAt)
		d, n := 0.0, 0
		for k, v := range features(r) {
			if len(b[k]) >= 6 {
				mu, sd := robust(b[k], floors[k])
				z := math.Min(10, math.Abs(v-mu)/sd)
				d += z * z
				n++
			}
		}
		if r.BodyWeightKG != nil {
			var nearest *model.Weight
			for i := range weights {
				w := &weights[i]
				if w.CatID == cat && !w.OccurredAt.After(r.OccurredAt) && w.OccurredAt.After(r.OccurredAt.AddDate(0, 0, -7)) && (nearest == nil || w.OccurredAt.After(nearest.OccurredAt)) {
					nearest = w
				}
			}
			if nearest != nil {
				z := math.Min(10, math.Abs(*r.BodyWeightKG-nearest.KG)/0.15)
				d += 2 * z * z
				n += 2
			}
		}
		if n >= 2 {
			s := math.Exp(-0.5 * d / float64(n))
			m.Candidates = append(m.Candidates, Candidate{cat, math.Round(s * 100), n})
		}
	}
	sort.Slice(m.Candidates, func(i, j int) bool { return m.Candidates[i].Score > m.Candidates[j].Score })
	// Require evidence for EVERY eligible cat; otherwise apparent separation is misleading.
	if len(m.Candidates) == len(r.EligibleCatIDs) && len(m.Candidates) > 0 {
		gap := 100.0
		if len(m.Candidates) > 1 {
			gap = m.Candidates[0].Score - m.Candidates[1].Score
		}
		if m.Candidates[0].Score >= 65 && gap >= 20 {
			m.Status = "review"
			m.Reason = "有区分度的匹配线索，请结合目击或设备身份核对；匹配分不是概率"
		} else {
			m.Reason = "猫之间特征重叠，无法可靠区分"
		}
	}
	return m
}
func Analyze(rs []model.Record, weights []model.Weight, now time.Time) Report {
	rep := Report{Matches: []Match{}, Alerts: []Alert{}, BaselineDays: 28}
	groups := map[string][]model.Record{}
	for _, r := range rs {
		if r.OccurredAt.After(now) {
			continue
		}
		if now.Sub(r.OccurredAt) <= 7*24*time.Hour {
			if r.Mode != "shared" {
				rep.Matches = append(rep.Matches, match(r, rs, weights))
			}
			if r.CatID == nil {
				rep.Unknown++
			} else {
				rep.Confirmed++
			}
			if r.Straining && r.NoOutput && r.Kind == "urine" {
				rep.Alerts = append(rep.Alerts, Alert{CatID: r.CatID, BoxID: r.BoxID, RecordID: r.ID, Level: "urgent", Title: "排尿用力但无产出", Detail: "请立即联系兽医。未知归属时，尽快观察该盆的所有使用猫。"})
			} else if r.Blood {
				rep.Alerts = append(rep.Alerts, Alert{CatID: r.CatID, BoxID: r.BoxID, RecordID: r.ID, Level: "urgent", Title: "记录中出现血迹", Detail: "请尽快联系兽医并保留观察记录。"})
			}
		}
		if r.CatID != nil && now.Sub(r.OccurredAt) <= 3*24*time.Hour {
			key := fmt.Sprintf("%d:%d:%s:%s", *r.CatID, r.BoxID, r.LitterKey, r.Kind)
			groups[key] = append(groups[key], r)
		}
	}
	for _, recent := range groups {
		sort.Slice(recent, func(i, j int) bool { return recent[i].OccurredAt.After(recent[j].OccurredAt) })
		last := recent[0]
		cut := now.Add(-3 * 24 * time.Hour)
		b := baseline(*last.CatID, last, rs, cut)
		vals := map[string][]float64{}
		for _, r := range recent {
			for k, v := range features(r) {
				vals[k] = append(vals[k], v)
			}
		}
		for k, xs := range vals {
			if len(b[k]) < 6 {
				continue
			}
			mu, sd := robust(b[k], floors[k])
			value := median(xs)
			z := (value - mu) / sd
			if math.Abs(z) >= 3.5 {
				direction := "升高"
				if z < 0 {
					direction = "降低"
				}
				rep.Alerts = append(rep.Alerts, Alert{CatID: last.CatID, BoxID: last.BoxID, RecordID: last.ID, Level: "watch", Title: names[k] + "较自身基线" + direction, Detail: fmt.Sprintf("近3天中位数 %.1f，历史中位数 %.1f；%d 条基线、%d 条近期记录。请复核称量方式和归属。", value, mu, len(b[k]), len(xs)), Metric: k, Z: math.Round(math.Abs(z)*10) / 10, Direction: z, Key: fmt.Sprintf("%d:%s:%s:%s", last.BoxID, last.LitterKey, last.Kind, k)})
			}
		}
		if last.Kind == "stool" && !last.NoOutput {
			history := map[string]int{}
			total := 0
			for _, old := range rs {
				if old.CatID != nil && *old.CatID == *last.CatID && old.Kind == "stool" && old.BoxID == last.BoxID && old.LitterKey == last.LitterKey && old.OccurredAt.Before(cut) && !old.OccurredAt.Before(cut.AddDate(0, 0, -28)) && old.Count == 1 && old.Shape != "" && !old.NoOutput {
					history[old.Shape]++
					total++
				}
			}
			mode := ""
			most := 0
			for shape, n := range history {
				if n > most {
					mode = shape
					most = n
				}
			}
			if total >= 6 && float64(most)/float64(total) >= 0.7 && last.Shape != mode && last.Shape != "" {
				rep.Alerts = append(rep.Alerts, Alert{CatID: last.CatID, BoxID: last.BoxID, RecordID: last.ID, Level: "watch", Title: "大便形态与历史常见形态不同", Detail: fmt.Sprintf("同配置历史 %d 条记录中，%d 条为 %s；最近一条为 %s。请结合干湿、大小及持续时间观察。", total, most, shapeName(mode), shapeName(last.Shape)), Metric: "shape", Direction: 1, Key: fmt.Sprintf("%d:%s:stool:shape:%s", last.BoxID, last.LitterKey, last.Shape)})
			}
		}
	}
	for i := range rep.Alerts {
		a := &rep.Alerts[i]
		if a.Level != "watch" {
			continue
		}
		others := map[uint]bool{}
		for _, b := range rep.Alerts {
			if b.CatID != nil && b.Key == a.Key && a.Direction*b.Direction > 0 {
				others[*b.CatID] = true
			}
		}
		if len(others) >= 2 {
			a.Common = true
			a.Detail += " 同盆多只猫同向变化：可能存在猫砂、测量方式或共同健康变化，个体异常提醒仍保留。"
		}
	}
	byCat := map[uint][]model.Weight{}
	for _, w := range weights {
		if !w.OccurredAt.After(now) {
			byCat[w.CatID] = append(byCat[w.CatID], w)
		}
	}
	for cat, ws := range byCat {
		sort.Slice(ws, func(i, j int) bool { return ws[i].OccurredAt.After(ws[j].OccurredAt) })
		if len(ws) < 2 || now.Sub(ws[0].OccurredAt) > 7*24*time.Hour {
			continue
		}
		last := ws[0]
		var ref *model.Weight
		for i := 1; i < len(ws); i++ {
			days := last.OccurredAt.Sub(ws[i].OccurredAt).Hours() / 24
			if days >= 7 && days <= 30 {
				ref = &ws[i]
				break
			}
		}
		if ref != nil && math.Abs(last.KG-ref.KG)/ref.KG >= 0.05 {
			c := cat
			rep.Alerts = append(rep.Alerts, Alert{CatID: &c, Level: "watch", Title: "体重变化超过观察阈值", Detail: fmt.Sprintf("%.2f kg → %.2f kg。请复称并结合年龄、饮食与兽医建议判断。", ref.KG, last.KG), Metric: "body_weight"})
		}
	}
	sort.Slice(rep.Alerts, func(i, j int) bool {
		a, b := rep.Alerts[i], rep.Alerts[j]
		if a.Level != b.Level {
			return a.Level == "urgent"
		}
		if a.RecordID != b.RecordID {
			return a.RecordID > b.RecordID
		}
		return a.Metric < b.Metric
	})
	return rep
}

func shapeName(shape string) string {
	return map[string]string{"pellet": "颗粒状", "formed": "成形条状", "soft": "软条状", "paste": "糊状", "watery": "水样"}[shape]
}
