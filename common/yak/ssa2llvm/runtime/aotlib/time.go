package aotlib

import "time"

func TimeNow() time.Time        { return time.Now() }
func TimeSleep(d time.Duration) { time.Sleep(d) }
func TimeSleepSeconds(seconds float64) {
	time.Sleep(time.Duration(seconds * float64(time.Second)))
}
func TimeParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// TimeExports mirrors the time module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.TimeExports signatures.
var TimeExports = map[string]any{
	"Now":           TimeNow,
	"Sleep":         TimeSleep,
	"sleep":         TimeSleepSeconds,
	"ParseDuration": TimeParseDuration,
}
