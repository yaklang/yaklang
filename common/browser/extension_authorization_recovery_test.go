package browser

import (
	"errors"
	"testing"
)

func TestExtensionAuthorizationRecoveryForError(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		code  string
		scope string
	}{
		{
			name:  "paired browser went offline",
			err:   errors.New("device is offline"),
			code:  "reconnect-device",
			scope: "workspace",
		},
		{
			name:  "document changed",
			err:   errors.New("authorization target document changed"),
			code:  "reselect-document",
			scope: "workspace",
		},
		{
			name:  "transform profile changed",
			err:   errors.New("transform profile fingerprint changed"),
			code:  "rebind-transform",
			scope: "transform",
		},
		{
			name:  "baseline changed",
			err:   errors.New("authorization baseline is no longer available"),
			code:  "recapture-baselines",
			scope: "baseline",
		},
		{
			name:  "isolation context changed",
			err:   errors.New("authorization identity context changed"),
			code:  "rebuild-identity-proof",
			scope: "identity",
		},
		{
			name:  "unknown failure",
			err:   errors.New("unexpected revalidation failure"),
			code:  "rebuild-workspace",
			scope: "workspace",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recovery := extensionAuthorizationRecoveryForError(test.err)
			if recovery.Code != test.code {
				t.Fatalf("expected code %q, got %q", test.code, recovery.Code)
			}
			if recovery.Scope != test.scope {
				t.Fatalf("expected scope %q, got %q", test.scope, recovery.Scope)
			}
			if recovery.Automatic {
				t.Fatal("authorization recovery must require an explicit user action")
			}
			if recovery.Message == "" || recovery.Message == test.err.Error() {
				t.Fatalf("expected a fixed redacted message, got %q", recovery.Message)
			}
		})
	}
}
