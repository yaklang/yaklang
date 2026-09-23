package yaktest

import "testing"

func TestMisc_PingScan(t *testing.T) {
	Run("pingscan local regression tests", t, []YakTestCase{
		{
			Name: "loopback scan",
			Src: `count = 0
for result = range ping.Scan("127.0.0.1,127.0.0.2", ping.concurrent(2)) {
 assert(result.Ok)
 count++
}
assert(count == 2)`,
		},
		{
			Name: "skip scan with zero concurrency",
			Src: `count = 0
for result = range ping.Scan("192.0.2.1,192.0.2.2", ping.skip(true), ping.concurrent(0)) {
 assert(result.Ok)
 assert(result.Reason == "skipped")
 count++
}
assert(count == 2)
assert(ping.Ping("192.0.2.1", ping.skip(true)).Ok)`,
		},
	}...)
}
