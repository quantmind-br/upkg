package github

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/quantmind-br/upkg/internal/helpers"
)

const (
	// DefaultUserAgent is required by the GitHub API.
	DefaultUserAgent = "upkg-github-client/1.0 (+https://github.com/quantmind-br/upkg)"
	// APIVersion is the GitHub REST API version header value.
	APIVersion = "2022-11-28"
	// DefaultMaxBytes is the default download / response body cap (512 MiB).
	DefaultMaxBytes int64 = 512 << 20
	// maxRedirects is the hard cap on redirect hops.
	maxRedirects = 5
)

// allowedExactHosts for API and asset downloads.
var allowedExactHosts = map[string]struct{}{
	"github.com":                           {},
	"api.github.com":                       {},
	"objects.githubusercontent.com":        {},
	"release-assets.githubusercontent.com": {},
}

func hostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if _, ok := allowedExactHosts[host]; ok {
		return true
	}
	if strings.HasSuffix(host, ".githubusercontent.com") {
		return true
	}
	if strings.HasSuffix(host, ".github.com") {
		return true
	}
	return false
}

// Client is a hardened GitHub HTTP client. It does not export *http.Client.
type Client struct {
	httpClient *http.Client
	token      string
	userAgent  string
	maxBytes   int64
	// apiBase overrides https://api.github.com (tests).
	apiBase string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithToken sets the bearer token (never log it).
func WithToken(token string) ClientOption {
	return func(c *Client) { c.token = strings.TrimSpace(token) }
}

// WithUserAgent overrides the User-Agent.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithMaxBytes sets the response body cap.
func WithMaxBytes(n int64) ClientOption {
	return func(c *Client) {
		if n > 0 {
			c.maxBytes = n
		}
	}
}

// WithAPIBase sets the API origin for tests (e.g. httptest.Server URL).
func WithAPIBase(base string) ClientOption {
	return func(c *Client) { c.apiBase = strings.TrimRight(base, "/") }
}

// WithHTTPClient replaces the hardened client (tests only).
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// NewClient builds a Client with TLS ≥ 1.2, allowlisted redirects, and anti-SSRF dialing.
// Token is resolved via ResolveToken when not provided through WithToken.
func NewClient(ctx context.Context, runner helpers.CommandRunner, opts ...ClientOption) *Client {
	c := &Client{
		userAgent: DefaultUserAgent,
		maxBytes:  DefaultMaxBytes,
		apiBase:   "https://api.github.com",
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.token == "" {
		c.token = ResolveToken(ctx, runner)
	}
	if c.httpClient == nil {
		c.httpClient = newHardenedHTTPClient()
	}
	return c
}

func proxyConfigured() bool {
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func newHardenedHTTPClient() *http.Client {
	useProxy := proxyConfigured()
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			// Skip anti-SSRF when an explicit proxy is configured: the dial
			// target is the proxy, which is legitimately private/loopback.
			if useProxy {
				return nil
			}
			return rejectBadAddress(address)
		},
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   0, // rely on context deadlines
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing non-https redirect to %s", req.URL.Scheme)
			}
			if !hostAllowed(req.URL.Host) {
				return fmt.Errorf("refusing redirect to non-allowlisted host %q", req.URL.Host)
			}
			if len(via) > 0 {
				prev := via[len(via)-1]
				if !strings.EqualFold(prev.URL.Host, req.URL.Host) {
					req.Header.Del("Authorization")
				}
			}
			return nil
		},
	}
}

// rejectBadAddress rejects private, loopback, link-local, CGNAT, ULA, multicast IPs.
func rejectBadAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("refusing dial to non-IP address %q", host)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("refusing dial to non-public IP %s", ip)
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return fmt.Errorf("refusing dial to CGNAT IP %s", ip)
		}
	}
	return nil
}

// APIError is a non-2xx GitHub API response with a typed status code.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	if e == nil {
		return "github api error"
	}
	msg := fmt.Sprintf("github api %s", e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

func (c *Client) resolveAPIURL(pathOrURL string) (string, error) {
	if strings.HasPrefix(pathOrURL, "/") {
		return c.apiBase + pathOrURL, nil
	}
	// Absolute URLs (pagination, *_url fields) must pass the same allowlist
	// as downloads before we attach the bearer token.
	u, perr := url.Parse(pathOrURL)
	if perr != nil || u.Scheme != "https" || !hostAllowed(u.Host) {
		// Allow test apiBase hosts (httptest) that are not github.com.
		if c.apiBase == "" || !strings.HasPrefix(pathOrURL, c.apiBase) {
			return "", fmt.Errorf("refusing api request to non-allowlisted URL")
		}
	}
	return pathOrURL, nil
}

func isRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden &&
		(resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "")
}

// doAPI issues a request to an API path (absolute path starting with /) or full URL.
func (c *Client) doAPI(ctx context.Context, pathOrURL string, etag string) ([]byte, http.Header, int, error) {
	full, err := c.resolveAPIURL(pathOrURL)
	if err != nil {
		return nil, nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("build request: %w", err)
	}
	c.applyAPIHeaders(req)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("github api request failed: %w", redactedErr(err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return nil, resp.Header.Clone(), http.StatusNotModified, nil
	}

	body, err := readCapped(resp.Body, c.maxBytes)
	if err != nil {
		return nil, resp.Header.Clone(), resp.StatusCode, err
	}

	if isRateLimited(resp) {
		return body, resp.Header.Clone(), resp.StatusCode, rateLimitError(resp, body)
	}
	if resp.StatusCode >= 400 {
		return body, resp.Header.Clone(), resp.StatusCode, &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       truncate(string(body), 200),
		}
	}
	return body, resp.Header.Clone(), resp.StatusCode, nil
}

func (c *Client) applyAPIHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", APIVersion)
	req.Header.Set("User-Agent", c.userAgent)
}

// DownloadGET downloads an HTTPS URL with allowlist + byte cap.
func (c *Client) DownloadGET(ctx context.Context, rawURL string, w io.Writer) (n int64, etag string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, "", fmt.Errorf("build download request: %w", err)
	}
	if req.URL.Scheme != "https" {
		return 0, "", fmt.Errorf("only https downloads allowed")
	}
	if !hostAllowed(req.URL.Host) {
		return 0, "", fmt.Errorf("download host not allowlisted: %s", req.URL.Host)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/octet-stream")
	if c.token != "" && strings.EqualFold(req.URL.Hostname(), "api.github.com") {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("download failed: %w", redactedErr(err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := readCapped(resp.Body, 4096) //nolint:errcheck // best-effort error body
		return 0, "", fmt.Errorf("download %s: %s", resp.Status, truncate(string(body), 200))
	}
	if cl := resp.ContentLength; cl > 0 && c.maxBytes > 0 && cl > c.maxBytes {
		return 0, "", fmt.Errorf("remote Content-Length %d exceeds cap %d", cl, c.maxBytes)
	}

	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	written, err := io.Copy(w, limited)
	if err != nil {
		return written, resp.Header.Get("ETag"), fmt.Errorf("download stream: %w", err)
	}
	if written > c.maxBytes {
		return written, resp.Header.Get("ETag"), fmt.Errorf("download exceeded size cap of %d bytes", c.maxBytes)
	}
	return written, resp.Header.Get("ETag"), nil
}

func readCapped(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response body exceeded size cap of %d bytes", maxBytes)
	}
	return data, nil
}

func rateLimitError(resp *http.Response, body []byte) error {
	ra := resp.Header.Get("Retry-After")
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	reset := resp.Header.Get("X-RateLimit-Reset")
	msg := fmt.Sprintf("github api rate limited (status %s", resp.Status)
	if remaining != "" {
		msg += ", remaining=" + remaining
	}
	if reset != "" {
		msg += ", reset=" + reset
	}
	if ra != "" {
		msg += ", retry-after=" + ra
	}
	msg += "): " + truncate(string(body), 120)
	return fmt.Errorf("%s", msg)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func redactedErr(err error) error {
	if err == nil {
		return nil
	}
	var uerr *url.Error
	if errors.As(err, &uerr) {
		if u, perr := url.Parse(uerr.URL); perr == nil {
			u.RawQuery = ""
			u.User = nil
			return fmt.Errorf("%s %s: %w", uerr.Op, u.Redacted(), uerr.Err)
		}
		return fmt.Errorf("%s: %w", uerr.Op, uerr.Err)
	}
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "bearer ") {
		return fmt.Errorf("request failed (details redacted)")
	}
	return err
}
