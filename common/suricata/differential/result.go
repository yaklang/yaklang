package differential

import "sort"

type Engine string

const (
	EngineYaklang  Engine = "yaklang"
	EngineSuricata Engine = "suricata"
	EngineSnort    Engine = "snort"
)

type Alert struct {
	GID   int    `json:"gid"`
	SID   int    `json:"sid"`
	Rev   int    `json:"rev"`
	Count int    `json:"count"`
	Msg   string `json:"msg,omitempty"`
}

type RuleStats struct {
	Loaded  int `json:"loaded"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type Result struct {
	Engine    Engine     `json:"engine"`
	Version   string     `json:"version,omitempty"`
	Alerts    []Alert    `json:"alerts,omitempty"`
	RuleStats *RuleStats `json:"rule_stats,omitempty"`
	ExitCode  int        `json:"exit_code"`
	Error     string     `json:"error,omitempty"`
	Stdout    string     `json:"stdout,omitempty"`
	Stderr    string     `json:"stderr,omitempty"`
}

func normalizeAlerts(alerts []Alert) []Alert {
	type key struct {
		gid int
		sid int
		rev int
		msg string
	}

	counts := make(map[key]int)
	for _, alert := range alerts {
		count := alert.Count
		if count == 0 {
			count = 1
		}
		gid := alert.GID
		if gid == 0 {
			gid = 1
		}
		counts[key{gid: gid, sid: alert.SID, rev: alert.Rev, msg: alert.Msg}] += count
	}

	result := make([]Alert, 0, len(counts))
	for alert, count := range counts {
		result = append(result, Alert{
			GID:   alert.gid,
			SID:   alert.sid,
			Rev:   alert.rev,
			Count: count,
			Msg:   alert.msg,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].GID != result[j].GID {
			return result[i].GID < result[j].GID
		}
		if result[i].SID != result[j].SID {
			return result[i].SID < result[j].SID
		}
		if result[i].Rev != result[j].Rev {
			return result[i].Rev < result[j].Rev
		}
		return result[i].Msg < result[j].Msg
	})
	return result
}
