package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepoURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		owner   string
		repo    string
		wantErr bool
	}{
		{"block/buzz", "block", "buzz", false},
		{"https://github.com/block/buzz", "block", "buzz", false},
		{"https://github.com/block/buzz/", "block", "buzz", false},
		{"https://github.com/block/buzz.git", "block", "buzz", false},
		{"https://github.com/block/buzz/releases/tag/v1", "block", "buzz", false},
		{"https://www.github.com/cli/cli", "cli", "cli", false},
		{"http://github.com/block/buzz", "", "", true},
		{"https://gitlab.com/foo/bar", "", "", true},
		{"git@github.com:block/buzz.git", "", "", true},
		{"/abs/path", "", "", true},
		{"", "", "", true},
		{"onlyone", "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			ref, err := ParseRepoURL(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.owner, ref.Owner)
			assert.Equal(t, tc.repo, ref.Repo)
			assert.True(t, IsGitHubURL(tc.in))
		})
	}
}
