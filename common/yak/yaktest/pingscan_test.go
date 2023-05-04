package yaktest

import (
	"fmt"
	"testing"
)

func TestMisc_PingScan(t *testing.T) {

	cases := []YakTestCase{
		{
			Name: "测试 ping scan",
			Src:  fmt.Sprintf(`loglevel("info");for result = range ping.Scan("47.52.100.0", ping.concurrent(20)) {if(result.Ok){println(result.Reason);};}`),
		},
	}

	Run("pingscan 可用性测试", t, cases...)
}

func TestMisc_SynPingScan(t *testing.T) {

	cases := []YakTestCase{
		{
			Name: "测试 ping scan",
			Src:  fmt.Sprintf(`loglevel("info");for result = range ping.Scan("47.52.100.0", ping.concurrent(20)) {if(result.Ok){println(result.Reason);};}`),
		},
	}

	Run("pingscan 可用性测试", t, cases...)
}
