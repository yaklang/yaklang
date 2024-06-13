package mutate

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"testing"
	"time"
)

func TestFuzzNucleiVar(t *testing.T) {
	server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK

aaa`))
	for _, v := range [][2]string{
		{
			"Host", "www.example.com",
		},
		{
			"Port", "80",
		},
		{
			"Hostname", "www.example.com",
		},
		{
			"RootURL", "http://www.example.com",
		},
		{
			"BaseURL", "http://www.example.com/aa",
		},
		{
			"Path", "/aa",
		},
		{
			"File", "aa",
		},
		{
			"Schema", "http",
		},
	} {
		resChan, err := _httpPool(fmt.Sprintf(`POST /aa HTTP/1.1
Content-Type: application/json
Host: www.example.com

{{%s}}`, v[0]), _httpPool_Host(utils.HostPort(server, port), false))
		if err != nil {
			t.Error(err)
			continue
		}
		for res := range resChan {
			body := lowhttp.GetHTTPPacketBody(res.RequestRaw)
			if string(body) != v[1] {
				t.Errorf("test var %s failed, expect %s, got %s", v[0], v[1], string(body))
			}
		}
	}
	server, port = utils.DebugMockHTTPS([]byte(`HTTP/1.1 200 OK

aaa`))
	for _, v := range [][2]string{
		{
			"Host", "www.example.com",
		},
		{
			"Port", "443",
		},
		{
			"Hostname", "www.example.com",
		},
		{
			"RootURL", "https://www.example.com",
		},
		{
			"BaseURL", "https://www.example.com/aa",
		},
		{
			"Path", "/aa",
		},
		{
			"File", "aa",
		},
		{
			"Schema", "https",
		},
	} {
		resChan, err := _httpPool(fmt.Sprintf(`POST /aa HTTP/1.1
Content-Type: application/json
Host: www.example.com

{{%s}}`, v[0]), _httpPool_Host(utils.HostPort(server, port), true), _httpPool_IsHttps(true))
		if err != nil {
			t.Error(err)
			continue
		}
		for res := range resChan {
			body := lowhttp.GetHTTPPacketBody(res.RequestRaw)
			if string(body) != v[1] {
				t.Errorf("test var %s failed, expect %s, got %s", v[0], v[1], string(body))
			}
		}
	}
}

func TestSendAndRenderAsynchronously(t *testing.T) {
	start := time.Now()
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		if time.Now().Sub(start).Seconds() > 5 {
			t.Fatal("test TestSendAndRenderAsynchronously failed")
		}
		return []byte(`HTTP/1.1 200 OK`)
	})
	fuzzOpt := Fuzz_WithExtraDynFuzzTagHandler("sleep", func(s string) []string {
		time.Sleep(5 * time.Second)
		return nil
	})
	addr := fmt.Sprintf("%s:%d", host, port)
	res, err := _httpPool(`POST / HTTP/1.1
Content-Type: application/json
Host: www.example.com

{{rs(10,10,100)}}`, WithPoolOpt_Addr(addr, false), WithPoolOpt_ForceFuzzDangerous(true), WithPoolOpt_ExtraFuzzOptions(fuzzOpt), WithPoolOpt_RetryTimes(0))
	if err != nil {
		t.Fatal(err)
	}
	for i := range res {
		_ = i
	}

}
