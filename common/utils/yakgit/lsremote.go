package yakgit

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// RemoteRefs is the result of listing advertised refs from a remote URL.
type RemoteRefs struct {
	// HeadSHA is the commit SHA for the requested branch, or the remote HEAD
	// when no branch was requested.
	HeadSHA string
	// HEADTarget is the symbolic target of refs/heads/... pointed by HEAD when known.
	HEADTarget string
	// Branches maps short branch name → commit SHA.
	Branches map[string]string
	// Tags maps short tag name → commit SHA. Annotated tags are peeled to the
	// commit SHA (peeled ^{} wins over the tag object).
	Tags map[string]string
}

// ListRemote lists advertised refs from a remote repository using go-git.
// It never shells out to a host `git` binary.
func ListRemote(remoteURL string, opts ...Option) (*RemoteRefs, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return nil, fmt.Errorf("remote url is empty")
	}

	c := NewConfig()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}

	rem := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})

	listOpts := &git.ListOptions{
		Auth:            c.Auth,
		InsecureSkipTLS: !c.VerifyTLS,
		PeelingOption:   git.AppendPeeled,
	}
	refs, err := rem.ListContext(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("list remote refs: %w", err)
	}

	out := &RemoteRefs{
		Branches: make(map[string]string),
		Tags:     make(map[string]string),
	}
	var headTarget string
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		name := ref.Name()
		switch {
		case name == plumbing.HEAD:
			if ref.Type() == plumbing.SymbolicReference {
				headTarget = ref.Target().String()
				out.HEADTarget = headTarget
			} else if ref.Type() == plumbing.HashReference {
				out.HeadSHA = ref.Hash().String()
			}
		case name.IsBranch():
			out.Branches[name.Short()] = ref.Hash().String()
		case name.IsTag():
			short := name.Short()
			if strings.HasSuffix(short, "^{}") {
				peeled := strings.TrimSuffix(short, "^{}")
				out.Tags[peeled] = ref.Hash().String()
				continue
			}
			if _, exists := out.Tags[short]; !exists {
				out.Tags[short] = ref.Hash().String()
			}
		}
	}

	wanted := strings.TrimSpace(c.Branch)
	switch {
	case wanted != "":
		sha, ok := out.Branches[wanted]
		if !ok {
			return nil, fmt.Errorf("branch %q not found on remote", wanted)
		}
		out.HeadSHA = sha
	case out.HeadSHA == "" && headTarget != "":
		if strings.HasPrefix(headTarget, "refs/heads/") {
			branch := strings.TrimPrefix(headTarget, "refs/heads/")
			if sha, ok := out.Branches[branch]; ok {
				out.HeadSHA = sha
			}
		}
	}

	return out, nil
}
