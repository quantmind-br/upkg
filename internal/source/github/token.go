package github

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/quantmind-br/upkg/internal/helpers"
)

// ResolveToken returns a GitHub token from the environment or gh CLI.
// Order: UPKG_GITHUB_TOKEN → GH_TOKEN → GITHUB_TOKEN → `gh auth token`.
// Empty string means unauthenticated (60 req/h). The token must never be logged.
func ResolveToken(ctx context.Context, runner helpers.CommandRunner) string {
	for _, key := range []string{"UPKG_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	if runner == nil {
		return ""
	}
	if !runner.CommandExists("gh") {
		return ""
	}
	// Best-effort fallback: bound gh so a hung keyring cannot stall callers
	// that pass an unbounded context (e.g. doctor).
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := runner.RunCommand(cctx, "gh", "auth", "token", "--hostname", "github.com")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
