package oracleprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
)

func TestResultClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want Status
	}{
		{nil, Success}, {&Error{Code: 1017}, AuthRejected}, {&Error{Code: 28000}, AccountLocked},
		{&Error{Code: 28001}, PasswordExpired}, {&Error{Code: 65162}, PasswordExpired},
		{&Error{Code: 12514}, ServiceUnknown}, {&Error{Code: 28040}, CredentialUnsupported},
		{&Error{Code: 28041}, ServerError}, {&Error{Code: 12650}, Unsupported},
		{&Error{Code: 12516}, Unavailable}, {&Error{Code: 3135}, Unavailable},
		{&Error{Code: 1031}, ServerError}, {ErrUnsupportedCredentialEncoding, CredentialUnsupported},
		{ErrUnsupportedVerifier, CredentialUnsupported}, {ErrEncryptionPolicy, Unsupported},
		{&tls.CertificateVerificationError{Err: errors.New("bad certificate")}, TLSRejected},
		{context.Canceled, Cancelled}, {context.DeadlineExceeded, Unavailable}, {io.EOF, Unavailable},
		{&net.OpError{Op: "read", Err: io.ErrUnexpectedEOF}, Unavailable},
		{&Failure{Stage: "options", Err: errors.New("invalid service")}, InvalidOptions},
		{&Failure{Stage: "options", Err: ErrInvalidCredentials}, CredentialUnsupported},
		{errors.New("invalid packet size"), ProtocolError},
		{&Failure{Stage: "auth-challenge", Err: errors.New("invalid padding")}, AuthIncomplete},
	} {
		if got := Classify(tc.err); got != tc.want {
			t.Errorf("%v: got %s want %s", tc.err, got, tc.want)
		}
	}
}

func TestProbeDetailedFailureStage(t *testing.T) {
	details, err := ProbeDetailed(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return nil, io.EOF }), Options{Address: "127.0.0.1:1521", Service: "s", Username: "u"})
	if details.Stage != "connect" || Classify(err) != Unavailable || !errors.Is(err, io.EOF) {
		t.Fatalf("details=%+v err=%v", details, err)
	}
}
