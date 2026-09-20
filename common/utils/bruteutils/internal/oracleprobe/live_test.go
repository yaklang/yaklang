package oracleprobe

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

// A separate fixture lets callers check locked/expired accounts without
// treating those accounts as successful authentication.
func TestProbeLiveRejection(t *testing.T) {
	code, err := strconv.Atoi(os.Getenv("YAK_ORACLE_TEST_ERROR_CODE"))
	if err != nil {
		t.Skip("set YAK_ORACLE_TEST_ERROR_CODE and fixture credentials")
	}
	err = Probe(context.Background(), nil, Options{Address: os.Getenv("YAK_ORACLE_TEST_ADDRESS"), Service: os.Getenv("YAK_ORACLE_TEST_SERVICE"), Username: os.Getenv("YAK_ORACLE_TEST_USER"), Password: os.Getenv("YAK_ORACLE_TEST_PASSWORD")})
	var ora *Error
	if !errors.As(err, &ora) || ora.Code != code {
		t.Fatalf("want ORA-%05d, got %v", code, err)
	}
}

// Explicit opt-in: this test contacts only a caller-provided test database.
func TestProbeLive(t *testing.T) {
	addr := os.Getenv("YAK_ORACLE_TEST_ADDRESS")
	if addr == "" {
		t.Skip("set YAK_ORACLE_TEST_ADDRESS, SERVICE, USER and PASSWORD for an isolated Oracle fixture")
	}
	o := Options{Address: addr, Service: os.Getenv("YAK_ORACLE_TEST_SERVICE"), Username: os.Getenv("YAK_ORACLE_TEST_USER"), Password: os.Getenv("YAK_ORACLE_TEST_PASSWORD"), Timeout: 15 * time.Second, SID: os.Getenv("YAK_ORACLE_TEST_SID") == "1", SysDBA: os.Getenv("YAK_ORACLE_TEST_SYSDBA") == "1"}
	// All scenarios share one budget, so a stalled fixture cannot multiply the timeout.
	ctx, cancel := context.WithTimeout(context.Background(), MaxTimeout)
	defer cancel()
	t.Run("correct_password", func(t *testing.T) {
		if e := Probe(ctx, &net.Dialer{}, o); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("wrong_password", func(t *testing.T) {
		bad := o
		bad.Password += "_incorrect"
		e := Probe(ctx, &net.Dialer{}, bad)
		var ora *Error
		if !errors.As(e, &ora) || !ora.CredentialsRejected() {
			t.Fatalf("want ORA-01017, got %v", e)
		}
	})
	t.Run("unknown_service", func(t *testing.T) {
		bad := o
		bad.Service = "YAK_PROBE_SERVICE_DOES_NOT_EXIST"
		e := Probe(ctx, &net.Dialer{}, bad)
		var ora *Error
		if !errors.As(e, &ora) || !ora.ServiceUnknown() {
			t.Fatalf("want unknown service, got %v", e)
		}
	})
	for _, name := range []string{"unknown_user", "empty_password"} {
		t.Run(name, func(t *testing.T) {
			bad := o
			if name == "unknown_user" {
				bad.Username = "YAK_PROBE_USER_DOES_NOT_EXIST"
			} else {
				bad.Password = ""
			}
			e := Probe(ctx, &net.Dialer{}, bad)
			var ora *Error
			if !errors.As(e, &ora) || !ora.CredentialsRejected() {
				t.Fatalf("want ORA-01017, got %v", e)
			}
		})
	}
}
