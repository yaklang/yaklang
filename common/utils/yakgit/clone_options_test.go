package yakgit

import (
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/require"
)

func TestBuildCloneOptionsUsesSingleBranchShallowDefaults(t *testing.T) {
	t.Parallel()

	c := &config{
		Depth:        1,
		SingleBranch: true,
		NoFetchTags:  true,
		Branch:       "release/1.0",
		VerifyTLS:    true,
	}
	opts := buildCloneOptions("https://example.invalid/repo.git", c)
	require.Equal(t, 1, opts.Depth)
	require.True(t, opts.SingleBranch)
	require.Equal(t, git.NoTags, opts.Tags)
	require.Equal(t, plumbing.NewBranchReferenceName("release/1.0"), opts.ReferenceName)
}

func TestCloneReferenceNameNormalizesShortBranch(t *testing.T) {
	t.Parallel()

	require.Equal(t, plumbing.ReferenceName(""), cloneReferenceName(""))
	require.Equal(t, plumbing.NewBranchReferenceName("main"), cloneReferenceName("main"))
	require.Equal(t, plumbing.ReferenceName("refs/heads/main"), cloneReferenceName("refs/heads/main"))
	require.Equal(t, plumbing.ReferenceName("refs/tags/v1.0.0"), cloneReferenceName("refs/tags/v1.0.0"))
}

func TestWithSingleBranchOption(t *testing.T) {
	t.Parallel()

	c := &config{}
	require.NoError(t, WithSingleBranch(true)(c))
	require.True(t, c.SingleBranch)
}
