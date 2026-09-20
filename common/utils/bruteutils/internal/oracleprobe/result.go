package oracleprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strings"
)

// Details contains only protocol metadata; never credentials, session keys or
// untrusted server messages. An algorithm is reported only after negotiation.
type Details struct {
	Stage      string `json:"stage"`
	Transport  string `json:"transport"`
	Encryption string `json:"encryption,omitempty"`
	Integrity  string `json:"integrity,omitempty"`
	Verifier   int    `json:"verifier,omitempty"`
}

type Failure struct {
	Stage string
	Err   error
}

func (e *Failure) Error() string { return "oracle " + e.Stage + ": " + e.Err.Error() }
func (e *Failure) Unwrap() error { return e.Err }

var (
	ErrUnsupportedVerifier       = errors.New("oracle: unsupported verifier type")
	ErrUnsupportedAuthentication = errors.New("oracle: server requires non-password authentication")
	ErrEncryptionPolicy          = errors.New("oracle: server did not honor native encryption policy")
	ErrTLSRequired               = errors.New("oracle: redirect requires TLS")
	ErrInvalidCredentials        = errors.New("oracle: credentials exceed probe limits or username is empty")
)

type Status string

const (
	Success               Status = "success"
	AuthRejected          Status = "auth-rejected"
	AccountLocked         Status = "account-locked"
	PasswordExpired       Status = "password-expired"
	ServiceUnknown        Status = "service-unknown"
	Unavailable           Status = "temporarily-unavailable"
	Unsupported           Status = "unsupported"
	InvalidOptions        Status = "invalid-options"
	Cancelled             Status = "cancelled"
	ProtocolError         Status = "protocol-error"
	ServerError           Status = "server-error"
	TLSRejected           Status = "tls-rejected"
	CredentialUnsupported Status = "unsupported-credentials"
	AuthIncomplete        Status = "authentication-incomplete"
)

// Classify does not infer authentication failure from an arbitrary ORA code.
func Classify(err error) Status {
	if err == nil {
		return Success
	}
	if errors.Is(err, context.Canceled) {
		return Cancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Unavailable
	}
	if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrUnsupportedCredentialEncoding) || errors.Is(err, ErrUnsupportedVerifier) {
		return CredentialUnsupported
	}
	var failure *Failure
	if errors.As(err, &failure) && failure.Stage == "options" {
		return InvalidOptions
	}
	var ora *Error
	if errors.As(err, &ora) {
		switch ora.Code {
		case 1017:
			return AuthRejected
		case 28000:
			return AccountLocked
		case 28001, 65162:
			return PasswordExpired
		case 12505, 12514:
			return ServiceUnknown
		case 28040:
			// Oracle also raises this when one account has no suitable verifier;
			// it does not prove other accounts on this target are unsupported.
			// https://docs.oracle.com/en/error-help/db/ora-28040/
			return CredentialUnsupported
		case 12650, 12660:
			return Unsupported
		// Adapted from go-ora v3/network/oracle_error.go OracleError.Bad
		// at 360b4b7ac9e96cee3e443180f2d6412bcacee62a; see LICENSE.
		// Login adds listener capacity and connection timeout errors, and
		// deliberately excludes SQL cursor errors unrelated to authentication.
		case 28, 1012, 1033, 1034, 1089, 3113, 3114, 3135, 12528, 12537,
			12170, 12516, 12518, 12519, 12520, 12541, 12543, 12545, 12564:
			return Unavailable
		default:
			return ServerError
		}
	}
	if errors.Is(err, ErrUnsupportedAuthentication) || errors.Is(err, ErrEncryptionPolicy) || errors.Is(err, ErrTLSRequired) {
		return Unsupported
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return TLSRejected
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return Unavailable
	}
	// A legacy challenge can fail decryption with an incorrect candidate;
	// malformed/missing server proof also never proves the target unusable.
	if failure != nil && strings.HasPrefix(failure.Stage, "auth-") {
		return AuthIncomplete
	}
	return ProtocolError
}
