package aischedule

import (
	"strings"
	"time"

	"github.com/teambition/rrule-go"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	DefaultTimezone = "UTC"
	MaxPreviewCount = 20
)

// Rule is the backend-owned recurrence calculator used by CRUD preview and the
// runtime scheduler. Keeping this behind a small wrapper prevents the protobuf
// contract and frontend from depending on a particular RRULE implementation.
type Rule struct {
	rule     *rrule.RRule
	location *time.Location
}

func Parse(rruleText, timezone string, startAt time.Time) (*Rule, error) {
	rruleText = strings.TrimSpace(rruleText)
	if rruleText == "" {
		return nil, utils.Error("rrule is required")
	}
	if startAt.IsZero() {
		return nil, utils.Error("start_at is required")
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = DefaultTimezone
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, utils.Errorf("invalid timezone %q: %v", timezone, err)
	}
	option, err := rrule.StrToROptionInLocation(rruleText, location)
	if err != nil {
		return nil, utils.Errorf("invalid rrule: %v", err)
	}
	if option.Freq == rrule.SECONDLY {
		return nil, utils.Error("second-level schedules are not supported")
	}
	option.Dtstart = startAt.In(location).Truncate(time.Second)
	r, err := rrule.NewRRule(*option)
	if err != nil {
		return nil, utils.Errorf("invalid rrule: %v", err)
	}
	return &Rule{rule: r, location: location}, nil
}

// Next returns the first occurrence strictly after the supplied instant.
func (r *Rule) Next(after time.Time) (time.Time, bool) {
	if r == nil || r.rule == nil {
		return time.Time{}, false
	}
	next := r.rule.After(after.In(r.location), false)
	if next.IsZero() {
		return time.Time{}, false
	}
	return next.UTC(), true
}

func (r *Rule) Preview(after time.Time, count int) []time.Time {
	if count <= 0 {
		count = 5
	}
	if count > MaxPreviewCount {
		count = MaxPreviewCount
	}
	result := make([]time.Time, 0, count)
	cursor := after
	for len(result) < count {
		next, ok := r.Next(cursor)
		if !ok {
			break
		}
		result = append(result, next)
		cursor = next
	}
	return result
}

func NormalizeRRULE(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("RRULE:") && strings.EqualFold(value[:len("RRULE:")], "RRULE:") {
		value = value[len("RRULE:"):]
	}
	return "RRULE:" + value
}
