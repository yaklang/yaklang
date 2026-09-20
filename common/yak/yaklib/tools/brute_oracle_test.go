package tools

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils"
)

func TestOracleOptionsReachBruteStart(t *testing.T) {
	encryption, err := yakBruteOpt_OracleEncryption("required")
	if err != nil {
		t.Fatal(err)
	}
	tlsOpt, err := yakBruteOpt_OracleTLS("db.example")
	if err != nil {
		t.Fatal(err)
	}
	timeout, err := yakBruteOpt_OracleTimeout(99)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(chan bool, 1)
	b, err := _yakitBruterNew("oracle", WithBruteCtx(context.Background()), yakBruteOpt_OracleSID("CUSTOM"), yakBruteOpt_OracleSysDBA(false), encryption, tlsOpt, timeout, yakBruteOpt_userlist("sys"), yakBruteOpt_passlist("p"), yakBruteOpt_minDelay(0), yakBruteOpt_maxDelay(0), yakBruteOpt_coreHandler(func(i *bruteutils.BruteItem) *bruteutils.BruteItemResult {
		c := i.OracleConfig
		seen <- c != nil && len(c.Services) == 1 && c.Services[0] == "CUSTOM" && c.SID && c.SysDBA != nil && !*c.SysDBA && c.TLS != nil && !c.TLS.InsecureSkipVerify && c.TLS.ServerName == "db.example" && c.Encryption == "required" && c.Timeout == 20*time.Second
		return i.Result()
	}))
	if err != nil {
		t.Fatal(err)
	}
	ch, err := b.Start("127.0.0.1:1521")
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	select {
	case ok := <-seen:
		if !ok {
			t.Fatal("Oracle options not forwarded to Start")
		}
	default:
		t.Fatal("handler not called")
	}
}

func TestOracleOptionsValidation(t *testing.T) {
	if _, err := _yakitBruterNew("ssh", yakBruteOpt_OracleService("s")); err == nil {
		t.Fatal("wrong protocol accepted Oracle options")
	}
	if _, err := _yakitBruterNew("oracle", yakBruteOpt_OracleSID("")); err == nil {
		t.Fatal("empty SID accepted")
	}
	if _, err := yakBruteOpt_OracleEncryption("requiredd"); err == nil {
		t.Fatal("invalid policy accepted")
	}
	if _, err := yakBruteOpt_OracleTLS("db.example", "not a certificate"); err == nil {
		t.Fatal("invalid CA accepted")
	}
	for _, n := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := yakBruteOpt_OracleTimeout(n); err == nil {
			t.Fatal("bad timeout accepted")
		}
	}
	b, err := _yakitBruterNew("oracle", yakBruteOpt_OracleSID("OLD"), yakBruteOpt_OracleService("A", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if b.oracleConfig.SID || len(b.oracleConfig.Services) != 2 {
		t.Fatal("service option did not replace SID")
	}
	for _, name := range []string{"oracleService", "oracleSID", "oracleSysDBA", "oracleEncryption", "oracleTLS", "oracleTimeout"} {
		if BruterExports[name] == nil {
			t.Fatalf("missing Yak export %s", name)
		}
	}
}
