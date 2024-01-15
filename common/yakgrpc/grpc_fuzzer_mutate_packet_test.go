package yakgrpc

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestGRPCMUSTPASS_MUTATE_MUTATE_NEG(T *testing.T) {
	test := assert.New(T)
	_, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 1

a`))
	var req, err = mutate.ExecPool(`GET /a HTTP/1.1
Host: 127.0.0.1:` + fmt.Sprint(port) + `
`)
	if err != nil {
		test.Fail("mutate.ExecPool failed", err)
	}
	count := 0
	for reqIns := range req {
		count++
		T.Log(reqIns.Url)
	}
	test.Equal(1, count)
}

func TestGRPCMUSTPASS_MUTATE_MUTATE_POS(T *testing.T) {
	test := assert.New(T)
	_, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 1

a`))
	var req, err = mutate.ExecPool(`GET /a HTTP/1.1
Host: 127.0.0.1:`+fmt.Sprint(port)+`
`, mutate.WithPoolOpt_MutateHook(func(bytes []byte) [][]byte {
		freq, err := mutate.NewFuzzHTTPRequest(bytes)
		if err != nil {
			test.Fail("mutate.NewFuzzHTTPRequest failed", err)
			return nil
		}
		reqs, err := freq.FuzzHTTPHeader("X-Token", "{{int(1-10)}}").Results()
		if err != nil {
			return nil
		}
		var results [][]byte
		for _, reqIns := range reqs {
			raw, err := utils.DumpHTTPRequest(reqIns, true)
			if err != nil {
				log.Warnf("utils.DumpHTTPRequest failed: %s", err)
				continue
			}
			results = append(results, raw)
		}
		return results
	}))
	if err != nil {
		test.Fail("mutate.ExecPool failed", err)
	}
	count := 0
	for reqIns := range req {
		count++
		T.Log(reqIns.Url)
		fmt.Println(string(reqIns.RequestRaw))
	}
	test.Equal(10, count)
}
