package scannode

import (
	"testing"

	ssav1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ssa/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

func TestValidateSSAGitLsRemoteCommand(t *testing.T) {
	t.Parallel()
	cmd := &ssav1.GitLsRemoteCommand{
		Metadata:      &nodev1.CommandMetadata{CommandId: "c1"},
		TargetNodeId:  "node-a",
		SourceLocator: "https://example.com/repo.git",
		IncludeHead:   true,
	}
	if err := validateSSAGitLsRemoteCommand("node-a", cmd); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
	if err := validateSSAGitLsRemoteCommand("node-b", cmd); err == nil {
		t.Fatal("expected target mismatch")
	}
	bad := &ssav1.GitLsRemoteCommand{
		Metadata:      &nodev1.CommandMetadata{CommandId: "c1"},
		TargetNodeId:  "node-a",
		SourceLocator: "https://example.com/repo.git",
	}
	if err := validateSSAGitLsRemoteCommand("node-a", bad); err == nil {
		t.Fatal("expected include_head/tags required")
	}
}

func TestYakgitAuthOptions(t *testing.T) {
	t.Parallel()
	if opts, err := yakgitAuthOptions(nil); err != nil || opts != nil {
		t.Fatalf("nil auth: opts=%v err=%v", opts, err)
	}
	if _, err := yakgitAuthOptions(&ssav1.GitAuthSnapshot{Kind: "token"}); err == nil {
		t.Fatal("expected token required")
	}
	opts, err := yakgitAuthOptions(&ssav1.GitAuthSnapshot{Kind: "token", Token: "t"})
	if err != nil || len(opts) != 1 {
		t.Fatalf("token auth: opts=%d err=%v", len(opts), err)
	}
}
