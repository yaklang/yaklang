package yaktest

import (
	"context"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func getWastTime(f func()) time.Duration {
	start := time.Now()
	f()
	return time.Now().Sub(start)
}
func TestMITM(t *testing.T) {
	var client, err = coreplugin.NewLocalClient()
	if err != nil {
		t.Fatalf("start mitm local client failed: %s", err)
	}
	stream, err := client.MITM(context.Background())
	if err != nil {
		t.Fatalf("start mitm stream failed: %s", err)
	}
	err = stream.Send(&ypb.MITMRequest{
		Host:       "127.0.0.1",
		Port:       8089,
		DnsServers: []string{"8.8.8.8"},
	})
	if err != nil {
		t.Fatalf("send mitm request failed: %s", err)
	}
	err = utils.WaitConnect("127.0.0.1:8089", 5)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{
		Transport: &http.Transport{
			Proxy: func(request *http.Request) (*url.URL, error) {
				return url.Parse("http://127.0.0.1:8089")
			},
		},
	}
	var t1 time.Duration
	for range make([]int, 10) {
		t1 += getWastTime(func() {
			_, err := c.Get("https://www.baidu.com")
			if err != nil {
				t.Fatalf("get failed: %s", err)
			}
		})
	}
	c.Transport = nil
	var t2 time.Duration
	for range make([]int, 10) {
		t2 += getWastTime(func() {
			_, err := c.Get("https://www.baidu.com")
			if err != nil {
				t.Fatalf("get failed: %s", err)
			}
		})
	}
	t.Logf("t1: %s, t2: %s", t1, t2)
}
