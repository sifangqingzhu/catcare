package analysis

import (
	"catcare/internal/model"
	"time"
)

// Only direct recorded symptoms/counters are returned before the statistical job
// finishes. This O(records) path never fits or guesses identities.
func Immediate(rs []model.Record, now time.Time, timezone string) Report {
	r := Report{Alerts: []Alert{}, Matches: []Match{}, Pools: []Pool{}, JointAnalyses: []JointComparison{}, CalibrationDays: 7, CalibrationTimezone: timezone, BaselineDays: 28}
	for _, x := range rs {
		if x.OccurredAt.After(now) || now.Sub(x.OccurredAt) > 7*24*time.Hour {
			continue
		}
		if x.CatID == nil {
			r.Unknown++
		} else {
			r.Confirmed++
		}
		if x.Mode == "shared" {
			r.SharedCount += x.Count
		}
		if x.Straining && x.NoOutput && x.Kind == "urine" {
			r.Alerts = append(r.Alerts, Alert{CatID: x.CatID, BoxID: x.BoxID, RecordID: x.ID, Level: "urgent", Title: "排尿用力但无产出", Detail: "请立即联系兽医。未知归属时尽快观察该盆的使用猫。"})
		} else if x.Blood {
			r.Alerts = append(r.Alerts, Alert{CatID: x.CatID, BoxID: x.BoxID, RecordID: x.ID, Level: "urgent", Title: "记录中出现血迹", Detail: "请尽快联系兽医并保留观察记录。"})
		}
	}
	return r
}
