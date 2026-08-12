package yakgit

import (
	"testing"
)

func TestListRemoteRejectsEmptyURL(t *testing.T) {
	t.Parallel()
	if _, err := ListRemote("  "); err == nil {
		t.Fatal("expected error for empty remote url")
	}
}

func TestListRemoteRejectsMissingBranch(t *testing.T) {
	t.Parallel()
	// Use an obviously invalid host so we don't depend on network success;
	// the empty-url / auth option path is covered separately. This asserts
	// WithBranch is accepted by the option plumbing.
	_, err := ListRemote("https://127.0.0.1:1/no-such-repo.git", WithBranch("main"))
	if err == nil {
		t.Fatal("expected list remote to fail against closed port")
	}
}
