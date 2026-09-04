package scannode

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

type aiRuntimeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f aiRuntimeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadOrdinaryAISessionAttachmentKeepsInlineSafetyLimit(t *testing.T) {
	t.Parallel()

	const inlineLimit = 64 << 10
	tests := []struct {
		name           string
		size           int
		wantTruncated  bool
		wantMarkerText string
	}{
		{name: "keeps inline maximum", size: inlineLimit},
		{name: "truncates above inline maximum", size: inlineLimit + 1, wantTruncated: true, wantMarkerText: "truncated to 65536 bytes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := strings.Repeat("~", test.size)
			client := &http.Client{Transport: aiRuntimeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if got := request.Header.Get("Authorization"); got != "Bearer node-session-token" {
					t.Fatalf("authorization header = %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    request,
				}, nil
			})}

			content, err := downloadAISessionAttachment(context.Background(), aiSessionBinding{
				PlatformBearerToken: "node-session-token",
				PlatformAPIBaseURL:  "https://platform.example",
				NodeSessionID:       "node-session-expanded",
				HTTPClient:          client,
			}, aiSessionAttachmentRef{AttachmentID: "aiatt_expanded", Filename: "expanded.log"})
			if err != nil {
				t.Fatalf("download attachment: %v", err)
			}
			if got := strings.Count(content, "~"); got != min(test.size, inlineLimit) {
				t.Fatalf("rendered content bytes = %d, want %d", got, min(test.size, inlineLimit))
			}
			if got := strings.Contains(content, "attachment content truncated"); got != test.wantTruncated {
				t.Fatalf("truncated marker present = %v, want %v", got, test.wantTruncated)
			}
			if test.wantMarkerText != "" && !strings.Contains(content, test.wantMarkerText) {
				t.Fatalf("missing truncation marker %q", test.wantMarkerText)
			}
		})
	}
}

func TestDownloadAISessionAttachmentUsesNodeReachablePlatformOrigin(t *testing.T) {
	t.Parallel()

	const expectedURL = "http://127.0.0.1:8094/v1/ai/attachments/aiatt_123/download?node_session_id=node-session-ai"
	client := &http.Client{Transport: aiRuntimeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != expectedURL {
			t.Fatalf("attachment request URL = %q, want %q", request.URL.String(), expectedURL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("marker-after-64k")),
			Request:    request,
		}, nil
	})}

	content, err := downloadAISessionAttachment(context.Background(), aiSessionBinding{
		PlatformBearerToken: "node-session-token",
		PlatformAPIBaseURL:  "http://127.0.0.1:8094",
		NodeSessionID:       "node-session-ai",
		HTTPClient:          client,
	}, aiSessionAttachmentRef{
		AttachmentID: "aiatt_123",
		Filename:     "expanded.log",
	})
	if err != nil {
		t.Fatalf("download attachment: %v", err)
	}
	if !strings.Contains(content, "marker-after-64k") {
		t.Fatalf("downloaded content = %q", content)
	}
}

func TestManagedAISessionAttachmentDownloadURLIgnoresCommandProvidedURL(t *testing.T) {
	t.Parallel()

	binding := aiSessionBinding{
		PlatformAPIBaseURL: "http://127.0.0.1:8094",
		NodeSessionID:      "node-session-ai",
	}
	downloadURL, err := managedAISessionAttachmentDownloadURL(binding, aiSessionAttachmentRef{
		AttachmentID: "aiatt_123",
		DownloadURL:  "http://attacker.invalid/forged?node_session_id=other",
	})
	if err != nil {
		t.Fatalf("managed attachment URL: %v", err)
	}
	const expectedURL = "http://127.0.0.1:8094/v1/ai/attachments/aiatt_123/download?node_session_id=node-session-ai"
	if downloadURL != expectedURL {
		t.Fatalf("managed attachment URL = %q, want %q", downloadURL, expectedURL)
	}
}

func TestDownloadAISessionAttachmentDoesNotFollowRedirect(t *testing.T) {
	t.Parallel()

	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled = true
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	_, err := downloadAISessionAttachment(context.Background(), aiSessionBinding{
		PlatformBearerToken: "node-session-token",
		PlatformAPIBaseURL:  redirect.URL,
		NodeSessionID:       "node-session-ai",
		HTTPClient:          redirect.Client(),
	}, aiSessionAttachmentRef{AttachmentID: "aiatt_123"})
	if err == nil || !strings.Contains(err.Error(), "status=302") {
		t.Fatalf("redirect error = %v, want status=302", err)
	}
	if targetCalled {
		t.Fatal("managed attachment download followed redirect")
	}
}

func TestDownloadAISessionAttachmentTruncatesAtUTF8Boundary(t *testing.T) {
	t.Parallel()

	const inlineLimit = 64 << 10
	body := strings.Repeat("~", inlineLimit-1) + "中"
	client := &http.Client{Transport: aiRuntimeRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}

	content, err := downloadAISessionAttachment(context.Background(), aiSessionBinding{
		PlatformBearerToken: "node-session-token",
		PlatformAPIBaseURL:  "https://platform.example",
		NodeSessionID:       "node-session-utf8",
		HTTPClient:          client,
	}, aiSessionAttachmentRef{AttachmentID: "aiatt_utf8_boundary", Filename: "utf8.log"})
	if err != nil {
		t.Fatalf("download attachment: %v", err)
	}
	if !utf8.ValidString(content) {
		t.Fatal("rendered attachment is not valid UTF-8")
	}
	if got := strings.Count(content, "~"); got != inlineLimit-1 {
		t.Fatalf("rendered ASCII prefix bytes = %d, want %d", got, inlineLimit-1)
	}
	if strings.Contains(content, "中") {
		t.Fatal("partial boundary rune must be omitted")
	}
	if !strings.Contains(content, "truncated to 65536 bytes") {
		t.Fatal("missing inline truncation marker")
	}
}
