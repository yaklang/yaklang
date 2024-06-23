package sarif

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_invocation_with_time_utc(t *testing.T) {
	success := true
	i := (&Invocation{ExecutionSuccessful: &success}).
		WithStartTimeUTC(mustParseTime(t, "2020-12-31T23:59:59+01:00")).
		WithEndTimeUTC(mustParseTime(t, "2021-01-01T00:00:00+01:00"))

	assert.Equal(t, `{"endTimeUtc":"2020-12-31T23:00:00Z","executionSuccessful":true,"startTimeUtc":"2020-12-31T22:59:59Z"}`, getJsonString(i))
}

func mustParseTime(t *testing.T, value string) time.Time {
	ts, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return ts
}
