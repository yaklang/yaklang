package tools

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func Test_scanFingerprint(t *testing.T) {

	target := "123.204.5.34"

	port := "3389"

	protoList := []interface{}{"tcp"}

	pp := func(proto ...interface{}) fp.ConfigOption {
		return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
	}

	ch, err := scanFingerprint(target, port, pp(protoList...),
		fp.WithForceEnableAllFingerprint(true),
		fp.WithActiveMode(true),
		//fp.WithProbeTimeoutHumanRead(5),
		//fp.WithProbesMax(100),
	)
	//ch, err := scanFingerprint(target, "162", pp(protoList...), fp.WithProbeTimeoutHumanRead(5))

	if err != nil {
		t.Error(err)
	}

	for v := range ch {
		//fmt.Println(v.String())

		spew.Dump(v)
	}
}

func Test_scanFingerprint1(t *testing.T) {
	target := "192.168.3.104"

	tcpPorts := "3306,9090"
	synPorts := "6379,9090"

	tcpScan := func(addr string) {
		ch, err := scanFingerprint(
			addr, tcpPorts,
		)

		if err != nil {
			t.FailNow()
		}

		for v := range ch {
			fmt.Println("TCPGOT " + v.String())
		}
	}

	Scan := func(target string, port string, opts ...scanOpt) (chan *synscan.SynScanResult, error) {
		config := &_yakPortScanConfig{
			waiting:           5 * time.Second,
			rateLimitDelayMs:  1,
			rateLimitDelayGap: 5,
		}
		for _, opt := range opts {
			opt(config)
		}
		return _synScanDo(hostsToChan(target), port, config)
	}

	synScan := func(addr string) {
		res, err := Scan(target, synPorts, _scanOptExcludePorts(tcpPorts))
		//res, err := Scan(target, synPorts, _scanOptOpenPortInitPortFilter("6379"))
		//res, err := Scan(target, synPorts)
		if err != nil {
			t.FailNow()
		}
		res2, err := _scanFromTargetStream(res)
		if err != nil {
			t.FailNow()
		}
		for result := range res2 {
			fmt.Println("SYNGOT " + result.String())
		}
	}
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		synScan(target)
	}()

	go func() {
		defer wg.Done()
		tcpScan(target)
	}()

	wg.Wait()
}

func TestMUSTPASS_Fp_GMTls(t *testing.T) {
	mockGMHost, mockGMPort := utils.DebugMockOnlyGMHTTP(context.Background(), nil)
	t.Logf("mockGMHost: %v, mockGMPort: %v", mockGMHost, mockGMPort)
	type args struct {
		target string
		port   string
		opts   []fp.ConfigOption
	}
	tests := []struct {
		name    string
		args    args
		want    fp.PortState
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "GM Tls 站点启用 all() 时，应当返回 OPEN",
			args: args{
				target: mockGMHost,
				port:   fmt.Sprint(mockGMPort),
				opts: []fp.ConfigOption{
					fp.WithActiveMode(true),
					fp.WithForceEnableAllFingerprint(true),
					fp.WithOnlyEnableWebFingerprint(true),
					fp.WithTransportProtos(fp.TCP),
				},
			},
			want:    fp.OPEN,
			wantErr: assert.NoError,
		},
		{
			name: "GM Tls 站点启用 only web() 时，应当返回 CLOSE",
			args: args{
				target: mockGMHost,
				port:   fmt.Sprint(mockGMPort),
				opts: []fp.ConfigOption{
					fp.WithActiveMode(true),
					//fp.WithForceEnableAllFingerprint(true),
					fp.WithOnlyEnableWebFingerprint(true),
					fp.WithTransportProtos(fp.TCP),
				},
			},
			want:    fp.CLOSED,
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanFingerprint(tt.args.target, tt.args.port, tt.args.opts...)
			if !tt.wantErr(t, err, fmt.Sprintf("scanFingerprint(%v, %v)", tt.args.target, tt.args.port)) {
				return
			}
			for v := range got {
				assert.Equalf(t, tt.want, v.State, "scanFingerprint(%v, %v)", tt.args.target, tt.args.port)
			}
		})
	}
}

func mockRedirectServer(resp []byte) (server *httptest.Server) {
	// 创建一个新的ServeMux（路由器）
	mux := http.NewServeMux()

	// 处理函数，返回模拟的响应
	handler := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/second", http.StatusFound)
	}

	handler2 := func(w http.ResponseWriter, r *http.Request) {
		w.Write(resp)
	}

	// 注册处理函数到路由器
	mux.HandleFunc("/", handler)
	mux.HandleFunc("/second", handler2)

	// 创建一个httptest.Server
	server = httptest.NewServer(mux)

	return server
}
func TestMUSTPASS_Fp_ScanHttpFlow(t *testing.T) {
	resp := utils.RandNumberStringBytes(20)

	server := mockRedirectServer([]byte(resp))

	defer server.Close()

	host, port, err := utils.ParseStringToHostPort(server.URL)

	if err != nil {
		t.Error(err)
	}

	ch, err := scanFingerprint(host, fmt.Sprintf("%d", port), fp.WithActiveMode(true))

	for v := range ch {
		if len(v.Fingerprint.HttpFlows) != 2 {
			t.FailNow()
		}
		if string(v.Fingerprint.HttpFlows[0].ResponseBody) != resp {
			t.FailNow()
		}

		if !strings.Contains(string(v.Fingerprint.HttpFlows[1].RequestHeader), "/second") {
			t.FailNow()
		}
	}
}

func mockTimeoutServer() *httptest.Server {
	mux := http.NewServeMux()

	// 模拟超时的处理函数
	timeoutHandler := func(w http.ResponseWriter, r *http.Request) {
		// 通过sleep模拟长时间运行的处理，这里的时间应该长于测试中设置的HTTP请求超时时间
		time.Sleep(20 * time.Second) // 假设客户端的超时设置小于2分钟
	}

	// 注册处理函数到路由器，对favicon.ico请求模拟超时
	mux.HandleFunc("/favicon.ico", timeoutHandler)

	// 创建并返回一个httptest.Server
	server := httptest.NewServer(mux)

	return server
}
func TestMUSTPASS_Fp_favicon(t *testing.T) {

	server := mockTimeoutServer()

	defer server.Close()

	host, port, err := utils.ParseStringToHostPort(server.URL)

	if err != nil {
		t.Error(err)
	}

	done := make(chan bool)

	go func() {
		ch, err := scanFingerprint(host, fmt.Sprintf("%d", port), fp.WithActiveMode(true))
		if err != nil {
			t.Error(err)
		}

		for v := range ch {
			log.Info(v.String())
		}
		done <- true
	}()

	select {
	case <-time.After(25 * time.Second):
		t.Fatal("Test favicon.ico failed due to timeout")
	case <-done:
	}
}
