package yaktest

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net"
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
	port := utils.GetRandomAvailableTCPPort()
	host := fmt.Sprintf("%s.com", utils.RandStringBytes(10))
	err = stream.Send(&ypb.MITMRequest{
		Host:       "127.0.0.1",
		Port:       8089,
		DnsServers: []string{"8.8.8.8"},
		Hosts: []*ypb.KVPair{
			{
				Key:   host,
				Value: "127.0.0.1",
			},
		},
	})
	if err != nil {
		t.Fatalf("send mitm request failed: %s", err)
	}
	err = utils.WaitConnect("127.0.0.1:8089", 5)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{
		Proxy: func(request *http.Request) (*url.URL, error) {
			return url.Parse("http://127.0.0.1:8089")
		}}
	c := &http.Client{
		Transport: transport,
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
	t.Logf("get baidu html with mitm proxy wast time: %s, get baidu html no proxy wast time: %s", t1, t2)
	go func() {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		err = http.Serve(l, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte("hello"))
		}))
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = utils.WaitConnect(fmt.Sprintf("127.0.0.1:%d", port), 5)
	if err != nil {
		t.Fatal(err)
	}
	c.Transport = transport
	_, err = c.Get(fmt.Sprintf("http://%s:%d", host, port))
	if err != nil {
		t.Fatalf("get %s:%d failed: %v", host, port, err)
	}
}
