package synscanx

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/time/rate"
)

func TestChooseRouteSamplePrefersNonLoopback(t *testing.T) {
	if got := chooseRouteSample(""); got != "" {
		t.Fatalf("empty target sample = %q", got)
	}
	if got := chooseRouteSample("127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("only loopback sample = %q", got)
	}
	if got := chooseRouteSample("127.0.0.1,10.1.2.3,8.8.8.8"); got != "10.1.2.3" {
		t.Fatalf("sample = %q, want first non-loopback", got)
	}
}

func TestToUint16AcceptsNumericTypes(t *testing.T) {
	cases := []struct {
		in   interface{}
		want uint16
	}{
		{uint16(7), 7},
		{int(8), 8},
		{int64(9), 9},
		{float64(10), 10},
		{"nope", 0},
		{nil, 0},
	}
	for _, tc := range cases {
		if got := toUint16(tc.in); got != tc.want {
			t.Fatalf("toUint16(%#v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSynScanResultString(t *testing.T) {
	var none *synscan.SynScanResult
	if none.String() != "" {
		t.Fatal("nil result string")
	}
	none.Show()
	got := (&synscan.SynScanResult{Host: "10.0.0.8", Port: 80}).String()
	if got != "OPEN: 10.0.0.8:80          from synscan" {
		t.Fatalf("result string = %q", got)
	}
}

func TestCallCallbackRecoversPanic(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.callback = func(result *synscan.SynScanResult) {
		panic("callback boom")
	}
	cfg.callCallback(&synscan.SynScanResult{Host: "10.0.0.1", Port: 1})
}

func TestGetNonExcludedPortsSkipsExcludedAndKeepsOrder(t *testing.T) {
	s := newPlanScanner(t)
	s.config.excludePorts = utils.NewPortsFilter("81,83")
	ports := s.GetNonExcludedPorts("80,81,82,83")
	want := []int{80, 82}
	if len(ports) != len(want) {
		t.Fatalf("ports = %v", ports)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("ports = %v, want %v", ports, want)
		}
	}
}

func TestGetNonExcludedHostsSkipsExcluded(t *testing.T) {
	s := newPlanScanner(t)
	WithExcludeHosts("10.0.0.2")(s.config)
	hosts := s.GetNonExcludedHosts("10.0.0.1,10.0.0.2,10.0.0.3")
	want := []string{"10.0.0.1", "10.0.0.3"}
	if len(hosts) != len(want) {
		t.Fatalf("hosts = %v", hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Fatalf("hosts = %v, want %v", hosts, want)
		}
	}
}

func TestSubmitTargetEmitsPlannedTCPTargets(t *testing.T) {
	s := newPlanScanner(t)
	ch, err := s.SubmitTarget("10.0.0.1,10.0.0.2", "80,443")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for target := range ch {
		if target.Mode != TCP {
			t.Fatalf("mode = %v", target.Mode)
		}
		got = append(got, target.Host+":"+itoa(target.Port))
	}
	want := []string{"10.0.0.1:80", "10.0.0.1:443", "10.0.0.2:80", "10.0.0.2:443"}
	if len(got) != len(want) {
		t.Fatalf("targets = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("targets = %v, want %v", got, want)
		}
	}
}

func TestSubmitTargetRejectsEmptyInput(t *testing.T) {
	s := newPlanScanner(t)
	if _, err := s.SubmitTarget("", "80"); err == nil {
		t.Fatal("expected error for empty targets")
	}
	s = newPlanScanner(t)
	if _, err := s.SubmitTarget("10.0.0.1", ""); err == nil {
		t.Fatal("expected error for empty ports")
	}
}

func TestSubmitTargetStopsWhenOpenPortCapAlreadyHit(t *testing.T) {
	s := newPlanScanner(t)
	s.config.maxOpenPorts = 1
	s.ipOpenPortMap.Store("10.0.0.1", uint16(1))
	ch, err := s.SubmitTarget("10.0.0.1,10.0.0.2", "80,81")
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
		t.Fatal("submit continued after the open-port cap was already reached")
	}
}

func TestGenerateHostPortStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := generateHostPort(ctx, []string{"10.0.0.1", "10.0.0.2"}, []int{1, 2, 3})
	first := <-out
	if first.Host != "10.0.0.1" || first.Port != 1 {
		t.Fatalf("first = %+v", first)
	}
	cancel()
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("generateHostPort did not stop after cancel")
		}
	}
}

func TestScanRejectsEmptyTargetBeforeCapture(t *testing.T) {
	if _, err := Scan(context.Background(), "", "80"); err == nil {
		t.Fatal("expected empty target error")
	}
	if _, err := ScanWithConfig("   ", "80", nil); err == nil {
		t.Fatal("expected blank target error")
	}
}

func newPlanScanner(t *testing.T) *Scannerx {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Scannerx{
		ctx:           ctx,
		cancel:        cancel,
		config:        &SynxConfig{shuffle: false, Ctx: ctx},
		ports:         utils.NewPortsFilter(),
		ipOpenPortMap: &sync.Map{},
		limiter:       rate.NewLimiter(rate.Limit(1e6), 1<<20),
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
