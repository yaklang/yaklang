package tools

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"testing"
	"time"
)

func Test_scanFingerprint(t *testing.T) {

	target := "127.0.0.1"

	port := "55072"

	protoList := []interface{}{"tcp", "udp"}

	pp := func(proto ...interface{}) fp.ConfigOption {
		return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
	}

	ch, err := scanFingerprint(target, port, pp(protoList...),
		fp.WithProbeTimeoutHumanRead(5),
		fp.WithProbesMax(100),
	)
	//ch, err := scanFingerprint(target, "162", pp(protoList...), fp.WithProbeTimeoutHumanRead(5))

	if err != nil {
		t.Error(err)
	}

	for v := range ch {
		fmt.Println(v.String())
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

func Test_scanFingerprint2(t *testing.T) {
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
