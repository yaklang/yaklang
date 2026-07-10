package notify

import "testing"

func TestParseURL(t *testing.T) {
	req, err := ParseURL("notify://feishu/messages:send")
	if err != nil {
		t.Fatalf("ParseURL failed: %v", err)
	}
	if req.Platform != PlatformFeishu {
		t.Fatalf("platform = %q, want %q", req.Platform, PlatformFeishu)
	}
	if req.Action != ActionMessagesSend {
		t.Fatalf("action = %q, want %q", req.Action, ActionMessagesSend)
	}
	if req.URL != "notify://feishu/messages:send" {
		t.Fatalf("url = %q", req.URL)
	}
}

func TestParseURLRejectsInvalidScheme(t *testing.T) {
	if _, err := ParseURL("https://feishu/messages:send"); err == nil {
		t.Fatal("expected invalid scheme error")
	}
}

func TestParseURLRejectsMissingVerb(t *testing.T) {
	if _, err := ParseURL("notify://feishu/messages"); err == nil {
		t.Fatal("expected missing action verb error")
	}
}
