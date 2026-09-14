package aischedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRulePreviewWeeklyInTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	start := time.Date(2026, 8, 17, 9, 0, 0, 0, loc)
	rule, err := Parse("RRULE:FREQ=WEEKLY;BYDAY=MO,WE;BYHOUR=9;BYMINUTE=0", "Asia/Shanghai", start)
	require.NoError(t, err)

	got := rule.Preview(start.Add(-time.Second), 3)
	require.Len(t, got, 3)
	require.Equal(t, start.UTC(), got[0])
	require.Equal(t, time.Date(2026, 8, 19, 9, 0, 0, 0, loc).UTC(), got[1])
	require.Equal(t, time.Date(2026, 8, 24, 9, 0, 0, 0, loc).UTC(), got[2])
}

func TestRuleRejectsSecondly(t *testing.T) {
	_, err := Parse("RRULE:FREQ=SECONDLY", "UTC", time.Now())
	require.ErrorContains(t, err, "second-level")
}
