package aotlib

import "time"

func TimeNow() time.Time { return time.Now() }

// TimeSleep is yak's time.Sleep. The argument is a number of seconds, the
// same as the global sleep() builtin. It is not a time.Duration: yak
// `time.Sleep(1)` is one second, and the ABI passes that 1 as an integer
// word. Decoding the word into a Duration sleeps for one nanosecond, so the
// next statement runs before a goroutine started with go gets scheduled.
func TimeSleep(seconds float64) {
	time.Sleep(time.Duration(seconds * float64(time.Second)))
}

func TimeSleepSeconds(seconds float64) { TimeSleep(seconds) }
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
