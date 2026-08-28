package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Asset is a GitHub release asset.
type Asset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"` // "sha256:hex" when present
}

// Release is a GitHub release.
type Release struct {
	ID          int64     `json:"id"`
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
	Assets      []Asset   `json:"assets"`
	HTMLURL     string    `json:"html_url"`
}

// RepoMeta is a minimal repository payload (for immutable repo_id pin).
type RepoMeta struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// ErrNotModified is returned when If-None-Match yields HTTP 304.
var ErrNotModified = fmt.Errorf("not modified")

// LatestReleaseResult holds a release plus response ETag for conditional polls.
type LatestReleaseResult struct {
	Release *Release
	ETag    string
	// NotModified is true when the server returned 304.
	NotModified bool
}

// GetRepo fetches repository metadata (includes immutable numeric id).
func (c *Client) GetRepo(ctx context.Context, ref RepoRef) (*RepoMeta, error) {
	path := fmt.Sprintf("/repos/%s/%s", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo))
	body, _, _, err := c.doAPI(ctx, path, "")
	if err != nil {
		return nil, err
	}
	var meta RepoMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("decode repo: %w", err)
	}
	return &meta, nil
}

// LatestRelease fetches /releases/latest. On 404 (only prereleases published),
// falls back to the first non-draft from ListReleases when includePrerelease is true,
// or still tries ListReleases for the newest non-draft non-prerelease.
// When includePrerelease is true, ListReleases is preferred because
// /releases/latest never returns prereleases.
// etag enables If-None-Match; 304 sets NotModified without counting as a full body fetch.
func (c *Client) LatestRelease(ctx context.Context, ref RepoRef, etag string, includePrerelease bool) (*LatestReleaseResult, error) {
	if includePrerelease {
		// /releases/latest never returns prereleases; list to find the newest non-draft.
		if rels, listErr := c.ListReleases(ctx, ref, 1, 30); listErr == nil {
			for i := range rels {
				if rels[i].Draft {
					continue
				}
				return &LatestReleaseResult{Release: &rels[i]}, nil
			}
		}
		// Fall through to /latest if the list failed or was empty.
	}

	path := fmt.Sprintf("/repos/%s/%s/releases/latest", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo))
	body, hdr, status, err := c.doAPI(ctx, path, etag)
	if status == http.StatusNotModified {
		return &LatestReleaseResult{NotModified: true, ETag: pickETag(hdr, etag)}, nil
	}
	if err == nil {
		var rel Release
		if decErr := json.Unmarshal(body, &rel); decErr != nil {
			return nil, fmt.Errorf("decode latest release: %w", decErr)
		}
		return &LatestReleaseResult{Release: &rel, ETag: pickETag(hdr, "")}, nil
	}

	// 404 from /latest is common when only prereleases exist.
	if status == http.StatusNotFound {
		rels, listErr := c.ListReleases(ctx, ref, 1, 30)
		if listErr != nil {
			return nil, fmt.Errorf("latest release not found and list failed: %w", listErr)
		}
		for i := range rels {
			r := &rels[i]
			if r.Draft {
				continue
			}
			if r.Prerelease && !includePrerelease {
				continue
			}
			return &LatestReleaseResult{Release: r}, nil
		}
		return nil, fmt.Errorf("no suitable release found for %s", ref)
	}
	return nil, err
}

// ReleaseByTag fetches a release by tag name.
func (c *Client) ReleaseByTag(ctx context.Context, ref RepoRef, tag string) (*Release, error) {
	path := fmt.Sprintf("/repos/%s/%s/releases/tags/%s",
		url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), url.PathEscape(tag))
	body, _, _, err := c.doAPI(ctx, path, "")
	if err != nil {
		return nil, err
	}
	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("decode release by tag: %w", err)
	}
	return &rel, nil
}

// ListReleases lists releases (paginated). page is 1-based; perPage max 100.
func (c *Client) ListReleases(ctx context.Context, ref RepoRef, page, perPage int) ([]Release, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 30
	}
	if perPage > 100 {
		perPage = 100
	}
	path := fmt.Sprintf("/repos/%s/%s/releases?page=%d&per_page=%d",
		url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), page, perPage)
	body, _, _, err := c.doAPI(ctx, path, "")
	if err != nil {
		return nil, err
	}
	var rels []Release
	if err := json.Unmarshal(body, &rels); err != nil {
		return nil, fmt.Errorf("decode releases list: %w", err)
	}
	return rels, nil
}

// RateLimit probes GET /rate_limit (does not consume primary REST limit meaningfully).
func (c *Client) RateLimit(ctx context.Context) (remaining, limit int, reset time.Time, err error) {
	body, _, _, err := c.doAPI(ctx, "/rate_limit", "")
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	var raw struct {
		Resources struct {
			Core struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, 0, time.Time{}, fmt.Errorf("decode rate_limit: %w", err)
	}
	return raw.Resources.Core.Remaining, raw.Resources.Core.Limit,
		time.Unix(raw.Resources.Core.Reset, 0).UTC(), nil
}

func pickETag(hdr http.Header, fallback string) string {
	if hdr == nil {
		return fallback
	}
	if v := hdr.Get("ETag"); v != "" {
		return v
	}
	return fallback
}
