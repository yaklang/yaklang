package oracleprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A separate fixture lets callers check locked/expired accounts without
// treating those accounts as successful authentication.
func TestProbeLiveRejection(t *testing.T) {
	raw := os.Getenv("YAK_ORACLE_TEST_ERROR_CODE")
	if raw == "" {
		t.Skip("set YAK_ORACLE_TEST_ERROR_CODE and fixture credentials")
	}
	codes := map[int]bool{}
	for _, value := range strings.Split(raw, ",") {
		code, err := strconv.Atoi(value)
		if err != nil || code <= 0 {
			t.Fatal("invalid expected Oracle error code")
		}
		codes[code] = true
	}
	err := Probe(context.Background(), nil, liveOptions(t))
	var ora *Error
	if !errors.As(err, &ora) || !codes[ora.Code] {
		t.Fatalf("want Oracle error %s, got %v", raw, err)
	}
	t.Logf("server rejection: ORA-%05d", ora.Code)
}

// Explicit opt-in: this test contacts only a caller-provided test database.
func TestProbeLive(t *testing.T) {
	addr := os.Getenv("YAK_ORACLE_TEST_ADDRESS")
	if addr == "" {
		t.Skip("set YAK_ORACLE_TEST_ADDRESS, SERVICE, USER and PASSWORD for an isolated Oracle fixture")
	}
	o := liveOptions(t)
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

func liveOptions(t *testing.T) Options {
	t.Helper()
	o := Options{Address: os.Getenv("YAK_ORACLE_TEST_ADDRESS"), Service: os.Getenv("YAK_ORACLE_TEST_SERVICE"), Username: os.Getenv("YAK_ORACLE_TEST_USER"), Password: os.Getenv("YAK_ORACLE_TEST_PASSWORD"), Timeout: 15 * time.Second, SID: os.Getenv("YAK_ORACLE_TEST_SID") == "1", SysDBA: os.Getenv("YAK_ORACLE_TEST_SYSDBA") == "1"}
	switch strings.ToUpper(os.Getenv("YAK_ORACLE_TEST_ENCRYPTION")) {
	case "", "ACCEPTED":
	case "REJECTED":
		o.Encryption = EncryptionRejected
	case "REQUESTED":
		o.Encryption = EncryptionRequested
	case "REQUIRED":
		o.Encryption = EncryptionRequired
	default:
		t.Fatal("unknown fixture encryption policy")
	}
	if os.Getenv("YAK_ORACLE_TEST_TLS") == "1" {
		// Bypassing verification is an explicit fixture-only choice, never a default.
		o.TLS = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: os.Getenv("YAK_ORACLE_TEST_TLS_INSECURE") == "1", ServerName: os.Getenv("YAK_ORACLE_TEST_TLS_SERVER_NAME")}
		if path := os.Getenv("YAK_ORACLE_TEST_TLS_CA"); path != "" {
			pem, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				t.Fatal("invalid YAK_ORACLE_TEST_TLS_CA")
			}
			o.TLS.RootCAs = pool
			// Oracle wallets in this lab are CN-only (no SAN). Go 1.22+ rejects CN.
			// Verify the chain against the supplied CA and accept a matching CN.
			cfg := o.TLS
			cfg.InsecureSkipVerify = true
			cfg.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
				if len(raw) == 0 {
					return errors.New("tls: no server certificate")
				}
				cert, err := x509.ParseCertificate(raw[0])
				if err != nil {
					return err
				}
				if _, err := cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
					return err
				}
				if name := cfg.ServerName; name != "" && cert.Subject.CommonName != name {
					if err := cert.VerifyHostname(name); err != nil {
						return err
					}
				}
				return nil
			}
		}
	}
	return o
}

func TestProbeLiveUntrustedTLS(t *testing.T) {
	if os.Getenv("YAK_ORACLE_TEST_ADDRESS") == "" || os.Getenv("YAK_ORACLE_TEST_TLS") != "1" {
		t.Skip("requires an isolated TCPS fixture")
	}
	o := liveOptions(t)
	o.TLS.InsecureSkipVerify = false
	o.TLS.RootCAs = x509.NewCertPool()
	err := Probe(context.Background(), nil, o)
	var verification *tls.CertificateVerificationError
	if !errors.As(err, &verification) {
		t.Fatalf("expected certificate verification failure, got %v", err)
	}
}

func TestProbeLiveBoundaries(t *testing.T) {
	if os.Getenv("YAK_ORACLE_TEST_ADDRESS") == "" || os.Getenv("YAK_ORACLE_TEST_BOUNDARIES") != "1" {
		t.Skip("explicit boundary fixture opt-in")
	}
	o := liveOptions(t)
	ctx, cancel := context.WithTimeout(context.Background(), MaxTimeout)
	defer cancel()
	for _, name := range []string{"uppercase_username", "password_case", "password_space", "username_nul"} {
		t.Run(name, func(t *testing.T) {
			// A successful control also prevents failed-attempt counters accumulating.
			if err := Probe(ctx, nil, o); err != nil {
				t.Fatalf("control login: %v", err)
			}
			candidate := o
			switch name {
			case "uppercase_username":
				candidate.Username = strings.ToUpper(o.Username)
			case "password_case":
				candidate.Password = strings.ToLower(o.Password)
				if candidate.Password == o.Password {
					t.Skip("fixture password has no uppercase characters")
				}
			case "password_space":
				candidate.Password += " "
			case "username_nul":
				candidate.Username += "\x00suffix"
			}
			err := Probe(ctx, nil, candidate)
			if name == "uppercase_username" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid credential was reported as authenticated")
			}
			if name != "username_nul" {
				var ora *Error
				if !errors.As(err, &ora) || !ora.CredentialsRejected() {
					t.Fatalf("expected ORA-01017, got %v", err)
				}
			}
		})
	}
}
