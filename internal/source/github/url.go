package github

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// RepoRef is a parsed GitHub owner/repo reference.
type RepoRef struct {
	Owner string
	Repo  string
	// Host is the web host (always "github.com" for public GitHub in v1).
	Host string
}

// shortRepoRE matches owner/repo (no scheme, no extra path). RE2 has no lookahead.
var shortRepoRE = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9-]{0,38})/([A-Za-z0-9._-]+)$`)

// ownerRE / repoRE enforce GitHub naming rules loosely (no leading/trailing hyphen check in regex).
var ownerRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
var repoRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// IsGitHubURL reports whether s is a github.com URL or a short owner/repo form.
// Scheme-less "owner/repo" is ambiguous with a relative local path (e.g.
// "dist/app.AppImage"); an existing local file always wins so local installs
// are never routed to the network resolver.
//
// Callers that need to distinguish "not GitHub" from "malformed GitHub
// reference" should call ParseRepoURL and inspect the error.
func IsGitHubURL(s string) bool {
	if _, err := ParseRepoURL(s); err != nil {
		return false
	}
	if !strings.Contains(s, "://") {
		if _, statErr := os.Stat(strings.TrimSpace(s)); statErr == nil {
			return false
		}
	}
	return true
}

// ParseRepoURL accepts:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo/
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo/releases/...
//   - owner/repo
//
// HTTP (non-TLS) is rejected. Non-github.com hosts are rejected.
//
//nolint:gocyclo // URL shape variants
func ParseRepoURL(s string) (RepoRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return RepoRef{}, fmt.Errorf("empty repository reference")
	}

	// Short form owner/repo (no scheme).
	if !strings.Contains(s, "://") && !strings.HasPrefix(s, "git@") {
		// Disambiguate from absolute paths.
		if strings.HasPrefix(s, "/") || strings.HasPrefix(s, ".") {
			return RepoRef{}, fmt.Errorf("not a GitHub repository reference")
		}
		m := shortRepoRE.FindStringSubmatch(s)
		if m == nil {
			return RepoRef{}, fmt.Errorf("invalid owner/repo: %q", s)
		}
		owner, repo := m[1], strings.TrimSuffix(m[2], ".git")
		if err := validateOwnerRepo(owner, repo); err != nil {
			return RepoRef{}, err
		}
		return RepoRef{Owner: owner, Repo: repo, Host: "github.com"}, nil
	}

	// git@github.com:owner/repo.git
	if strings.HasPrefix(s, "git@") {
		return RepoRef{}, fmt.Errorf("SSH git URLs are not supported; use https://github.com/owner/repo or owner/repo")
	}

	u, err := url.Parse(s)
	if err != nil {
		return RepoRef{}, fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return RepoRef{}, fmt.Errorf("only https URLs are allowed, got %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return RepoRef{}, fmt.Errorf("unsupported host %q (only github.com)", host)
	}

	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return RepoRef{}, fmt.Errorf("URL missing owner/repo path: %s", s)
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if err := validateOwnerRepo(owner, repo); err != nil {
		return RepoRef{}, err
	}
	return RepoRef{Owner: owner, Repo: repo, Host: "github.com"}, nil
}

func validateOwnerRepo(owner, repo string) error {
	if !ownerRE.MatchString(owner) || strings.HasPrefix(owner, "-") || strings.HasSuffix(owner, "-") {
		return fmt.Errorf("invalid GitHub owner: %q", owner)
	}
	if !repoRE.MatchString(repo) || repo == "." || repo == ".." {
		return fmt.Errorf("invalid GitHub repo: %q", repo)
	}
	if strings.HasPrefix(repo, "-") {
		return fmt.Errorf("invalid GitHub repo: %q must not start with '-'", repo)
	}
	return nil
}

// String returns owner/repo.
func (r RepoRef) String() string {
	return r.Owner + "/" + r.Repo
}
