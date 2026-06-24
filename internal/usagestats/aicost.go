package usagestats

import (
	"math"
	"time"

	"github.com/keithah/stint/internal/pricing"
)

// PeriodCents is one agent's list-price cost (in integer cents) bucketed into the
// daily / weekly / monthly / total windows the AI dashboard's Cost Tracker shows.
type PeriodCents struct {
	Daily   int
	Weekly  int
	Monthly int
	Total   int
}

// AICostCents is engine-priced AI cost (integer cents) sliced the way the AI
// dashboard needs it: a grand total, per-agent, per-day, and per-agent period
// buckets. All figures use the equivalent-API list price (pricing.Result.USD),
// never the subscription-discounted marginal — the dashboard meter reflects what
// the usage would cost on metered API regardless of how it was actually billed.
type AICostCents struct {
	TotalCents    int
	ByAgentCents  map[string]int
	ByDayCents    map[string]int
	PeriodByAgent map[string]PeriodCents
}

// AICostsFromGroups prices usage-event groups with the engine and buckets the
// list-price cost by agent, by day, and by daily/weekly/monthly period relative
// to windowEnd. It replaces the legacy heartbeat token-rate estimate, which had
// no concept of cache reads and so priced the (cache-dominated) token stream as
// if every token were fresh input. Costs accumulate in dollars and round to
// cents once per bucket so rounding never compounds across thousands of groups.
func AICostsFromGroups(groups []Group, engine *pricing.Engine, mode pricing.Mode, windowEnd time.Time) AICostCents {
	out := AICostCents{
		ByAgentCents:  map[string]int{},
		ByDayCents:    map[string]int{},
		PeriodByAgent: map[string]PeriodCents{},
	}
	if engine == nil {
		return out
	}
	dailyStart := windowEnd.AddDate(0, 0, -1)
	weeklyStart := windowEnd.AddDate(0, 0, -7)
	monthlyStart := windowEnd.AddDate(0, 0, -30)

	var totalUSD float64
	agentUSD := map[string]float64{}
	dayUSD := map[string]float64{}
	type periodUSD struct{ daily, weekly, monthly, total float64 }
	periodByAgent := map[string]*periodUSD{}

	for _, g := range groups {
		// List price only: nil billing override + Result.USD means subscription
		// reclassification cannot zero or discount the meter.
		usd := engine.Price(g.syntheticEvent(nil), mode).USD
		totalUSD += usd
		agentUSD[g.Agent] += usd
		dayUSD[g.Day] += usd

		p := periodByAgent[g.Agent]
		if p == nil {
			p = &periodUSD{}
			periodByAgent[g.Agent] = p
		}
		p.total += usd
		// Parse the day in windowEnd's location so daily/weekly/monthly boundaries
		// line up with the user's timezone (Day is already bucketed in that tz).
		if d, err := time.ParseInLocation("2006-01-02", g.Day, windowEnd.Location()); err == nil {
			if !d.Before(dailyStart) {
				p.daily += usd
			}
			if !d.Before(weeklyStart) {
				p.weekly += usd
			}
			if !d.Before(monthlyStart) {
				p.monthly += usd
			}
		}
	}

	out.TotalCents = toCents(totalUSD)
	for agent, v := range agentUSD {
		out.ByAgentCents[agent] = toCents(v)
	}
	for day, v := range dayUSD {
		out.ByDayCents[day] = toCents(v)
	}
	for agent, p := range periodByAgent {
		out.PeriodByAgent[agent] = PeriodCents{
			Daily:   toCents(p.daily),
			Weekly:  toCents(p.weekly),
			Monthly: toCents(p.monthly),
			Total:   toCents(p.total),
		}
	}
	return out
}

func toCents(usd float64) int { return int(math.Round(usd * 100)) }
