package usagestats

import (
	"math"
	"time"

	"github.com/keithah/stint/internal/pricing"
)

// PeriodCents is one bucket's list-price cost (in integer cents) split into the
// daily / weekly / monthly / total windows the AI dashboard's Cost Tracker shows.
type PeriodCents struct {
	Daily   int
	Weekly  int
	Monthly int
	Total   int
}

// AICostCents is engine-priced AI cost (integer cents) sliced the way the AI
// dashboard needs it: a grand total, per-bucket, per-day, and per-bucket period
// windows. The bucket key is whatever the caller's keyOf returns (a usage tool
// like "codex", or a mapped heartbeat agent like "gpt"). All figures use the
// equivalent-API list price (pricing.Result.USD), never the subscription
// marginal — the dashboard meter shows metered-API cost regardless of billing.
type AICostCents struct {
	TotalCents  int
	ByKeyCents  map[string]int
	ByDayCents  map[string]int
	PeriodByKey map[string]PeriodCents
}

// GroupCost is one usage-event group with its list-price cost in USD. Pricing is
// done once (PriceGroups); the same priced slice is bucketed under multiple
// taxonomies (BucketCents) without re-pricing.
type GroupCost struct {
	Group Group
	USD   float64
}

// PriceGroups prices each group exactly once with the engine, using list prices
// (nil billing override + Result.USD, so subscription reclassification cannot
// discount the meter). Returns nil when no engine is available.
func PriceGroups(groups []Group, engine *pricing.Engine, mode pricing.Mode) []GroupCost {
	if engine == nil {
		return nil
	}
	out := make([]GroupCost, len(groups))
	for i, g := range groups {
		out[i] = GroupCost{Group: g, USD: engine.Price(g.syntheticEvent(nil), mode).USD}
	}
	return out
}

// BucketCents aggregates pre-priced groups into total / by-key / by-day / period
// buckets, keying each group by keyOf. Daily/weekly/monthly membership is decided
// by the group's Day relative to windowEnd (parsed in windowEnd's location so the
// boundaries match the user's timezone). Cost accumulates in dollars and rounds
// to cents once per bucket so rounding never compounds across groups.
func BucketCents(costs []GroupCost, keyOf func(Group) string, windowEnd time.Time) AICostCents {
	out := AICostCents{ByKeyCents: map[string]int{}, ByDayCents: map[string]int{}, PeriodByKey: map[string]PeriodCents{}}
	dailyStart := windowEnd.AddDate(0, 0, -1)
	weeklyStart := windowEnd.AddDate(0, 0, -7)
	monthlyStart := windowEnd.AddDate(0, 0, -30)

	var totalUSD float64
	keyUSD := map[string]float64{}
	dayUSD := map[string]float64{}
	type periodUSD struct{ daily, weekly, monthly, total float64 }
	periodByKey := map[string]*periodUSD{}

	for _, c := range costs {
		key := keyOf(c.Group)
		totalUSD += c.USD
		keyUSD[key] += c.USD
		dayUSD[c.Group.Day] += c.USD

		p := periodByKey[key]
		if p == nil {
			p = &periodUSD{}
			periodByKey[key] = p
		}
		p.total += c.USD
		if d, err := time.ParseInLocation("2006-01-02", c.Group.Day, windowEnd.Location()); err == nil {
			if !d.Before(dailyStart) {
				p.daily += c.USD
			}
			if !d.Before(weeklyStart) {
				p.weekly += c.USD
			}
			if !d.Before(monthlyStart) {
				p.monthly += c.USD
			}
		}
	}

	out.TotalCents = toCents(totalUSD)
	for key, v := range keyUSD {
		out.ByKeyCents[key] = toCents(v)
	}
	for day, v := range dayUSD {
		out.ByDayCents[day] = toCents(v)
	}
	for key, p := range periodByKey {
		out.PeriodByKey[key] = PeriodCents{Daily: toCents(p.daily), Weekly: toCents(p.weekly), Monthly: toCents(p.monthly), Total: toCents(p.total)}
	}
	return out
}

func toCents(usd float64) int { return int(math.Round(usd * 100)) }
