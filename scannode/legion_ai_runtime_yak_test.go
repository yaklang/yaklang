package scannode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/aiengine"
)

type noopAISessionRuntimeEmitter struct{}

func (noopAISessionRuntimeEmitter) Emit(string, []byte) {}

func (noopAISessionRuntimeEmitter) Done([]byte) {}

func (noopAISessionRuntimeEmitter) Failed(string, string, []byte) {}

func TestBuildYakAIEngineOptionsIncludesAttachmentContentAndCredentialProjection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer node-session-token" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from attachment"))
	}))
	defer server.Close()

	options, err := buildYakAIEngineOptions(context.Background(), aiSessionBinding{
		Ref: aiSessionCommandRef{SessionID: "ai-session-1"},
		Attachments: []aiSessionAttachmentRef{
			{
				AttachmentID: "inputf_123",
				Filename:     "targets.txt",
				ContentType:  "text/plain",
				DownloadURL:  server.URL,
			},
		},
		CredentialRefs: []aiSessionCredentialRef{
			{
				CredentialID:   "sourcecred-1",
				CredentialType: "ssa_source",
				Scope:          "ssa.source",
			},
		},
		PlatformBearerToken: "node-session-token",
		HTTPClient:          server.Client(),
	}, noopAISessionRuntimeEmitter{})
	if err != nil {
		t.Fatalf("build yak ai engine options: %v", err)
	}

	config := aiengine.NewAIEngineConfig(options...)
	if len(config.AttachedResources) != 2 {
		t.Fatalf("unexpected attached resource count: %d", len(config.AttachedResources))
	}

	attachmentContent := config.AttachedResources[0].Value
	if !strings.Contains(attachmentContent, "Filename: targets.txt") {
		t.Fatalf("unexpected attachment resource: %s", attachmentContent)
	}
	if !strings.Contains(attachmentContent, "hello from attachment") {
		t.Fatalf("unexpected attachment content: %s", attachmentContent)
	}

	credentialProjection := config.AttachedResources[1].Value
	if !strings.Contains(credentialProjection, "credential_id: sourcecred-1") {
		t.Fatalf("unexpected credential projection: %s", credentialProjection)
	}
	if !strings.Contains(credentialProjection, "Secret material is not exposed") {
		t.Fatalf("unexpected credential projection: %s", credentialProjection)
	}
}
