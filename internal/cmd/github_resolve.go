package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/quantmind-br/upkg/internal/assetmatch"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/source/github"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/quantmind-br/upkg/internal/version"
	"github.com/rs/zerolog"
)

// githubResolveOpts controls release resolution for install/update.
type githubResolveOpts struct {
	Tag             string
	Prerelease      bool
	Yes             bool
	Format          string
	AssetRegex      string
	PreferAssetName string
	Existing        *core.SourceRef
	DataDir         string
	AppName         string
	// MetadataOnly skips asset download (update --check / version compare).
	MetadataOnly bool
}

// githubReleaseInfo is release metadata before asset download.
type githubReleaseInfo struct {
	Ref     github.RepoRef
	RepoID  int64
	Release *github.Release
	ETag    string
	Client  *github.Client
}

// githubResolved is a downloaded, verified local artifact plus source metadata.
type githubResolved struct {
	LocalPath string
	Source    *core.SourceRef
	Format    string
	Cleanup   func()
}

// errNotModified is returned when conditional latest-release yields 304.
var errNotModified = fmt.Errorf("release not modified")

func newGitHubClient(ctx context.Context) *github.Client {
	return github.NewClient(ctx, helpers.NewOSCommandRunner())
}

// fetchGitHubRelease loads repo pin + release metadata (no asset download).
//
//nolint:gocyclo // pin checks + latest/tag/prerelease branches
func fetchGitHubRelease(ctx context.Context, input string, opts githubResolveOpts) (*githubReleaseInfo, error) {
	ref, err := github.ParseRepoURL(input)
	if err != nil {
		return nil, fmt.Errorf("parse github ref: %w", err)
	}
	client := newGitHubClient(ctx)

	repoMeta, err := client.GetRepo(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch repository: %w", err)
	}

	if opts.Existing != nil && opts.Existing.RepoID != 0 && repoMeta.ID != opts.Existing.RepoID {
		return nil, fmt.Errorf("repository id changed for %s (was %d, now %d) — possible transfer/takeover; reinstall manually to accept",
			ref.String(), opts.Existing.RepoID, repoMeta.ID)
	}
	if opts.Existing != nil {
		if opts.Existing.Owner != "" && !strings.EqualFold(opts.Existing.Owner, ref.Owner) {
			return nil, fmt.Errorf("owner changed (was %s, now %s); reinstall to accept", opts.Existing.Owner, ref.Owner)
		}
		if opts.Existing.Repo != "" && !strings.EqualFold(opts.Existing.Repo, ref.Repo) {
			return nil, fmt.Errorf("repo changed (was %s, now %s); reinstall to accept", opts.Existing.Repo, ref.Repo)
		}
	}

	info := &githubReleaseInfo{Ref: ref, RepoID: repoMeta.ID, Client: client}

	if opts.Tag != "" {
		rel, tagErr := client.ReleaseByTag(ctx, ref, opts.Tag)
		if tagErr != nil {
			return nil, fmt.Errorf("fetch release %q: %w", opts.Tag, tagErr)
		}
		info.Release = rel
		return info, nil
	}

	etagIn := ""
	if opts.Existing != nil {
		etagIn = opts.Existing.ETag
	}
	res, err := client.LatestRelease(ctx, ref, etagIn, opts.Prerelease)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	if res.NotModified {
		return nil, errNotModified
	}
	info.Release = res.Release
	info.ETag = res.ETag

	if opts.Existing != nil && opts.Existing.ComponentPrefix != "" && info.Release != nil {
		got := version.ComponentPrefix(info.Release.TagName)
		if got != "" && got != opts.Existing.ComponentPrefix {
			return nil, fmt.Errorf("release tag prefix %q does not match pinned component %q", got, opts.Existing.ComponentPrefix)
		}
	}
	return info, nil
}

// pickAndDownload selects an asset and downloads it, building SourceRef.
//
//nolint:gocyclo // menu/auto-pick, digest pin, source assembly
func pickAndDownload(ctx context.Context, info *githubReleaseInfo, opts githubResolveOpts, log *zerolog.Logger) (*githubResolved, error) {
	rel := info.Release
	if rel == nil {
		return nil, fmt.Errorf("no release")
	}

	assets := make([]assetmatch.Asset, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		assets = append(assets, assetmatch.Asset{
			Name:        a.Name,
			Size:        a.Size,
			Digest:      a.Digest,
			DownloadURL: a.BrowserDownloadURL,
			ContentType: a.ContentType,
		})
	}

	assetRE := opts.AssetRegex
	if assetRE == "" && opts.PreferAssetName != "" {
		assetRE = "^" + regexpQuoteMeta(opts.PreferAssetName) + "$"
	}

	target := hostAssetTarget()
	pickOpts := assetmatch.PickOptions{
		Yes:        opts.Yes,
		Format:     opts.Format,
		AssetRegex: assetRE,
		AppName:    firstNonEmpty(opts.AppName, info.Ref.Repo),
	}

	pick, err := assetmatch.Pick(assets, target, pickOpts)
	if err != nil {
		return nil, err
	}
	if len(pick.Compatible) == 0 && opts.PreferAssetName != "" && opts.AssetRegex == "" {
		if log != nil {
			log.Debug().Str("prefer", opts.PreferAssetName).Msg("no asset matched previous name; relaxing asset pin")
		}
		color.Yellow("! No asset matching %q in this release; selecting by score instead", opts.PreferAssetName)
		// Prefer the previously installed format so an update cannot silently
		// escalate from a user-space artifact to a privileged system package.
		if opts.Format == "" && opts.Existing != nil && opts.Existing.AssetName != "" {
			prevFmt := assetmatch.DetectFormat(opts.Existing.AssetName)
			if prevFmt != "" && prevFmt != "unknown" {
				pickOpts.Format = prevFmt
			}
		}
		pickOpts.AssetRegex = ""
		pick, err = assetmatch.Pick(assets, target, pickOpts)
		if err != nil {
			return nil, err
		}
		// If format-constrained re-pick found nothing, fall back to any format.
		if len(pick.Compatible) == 0 && pickOpts.Format != "" {
			pickOpts.Format = ""
			pick, err = assetmatch.Pick(assets, target, pickOpts)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(pick.Compatible) == 0 {
		return nil, fmt.Errorf("%s", assetmatch.ExplainExcluded(pick.Excluded))
	}

	var chosen assetmatch.Candidate
	switch {
	case pick.Winner != nil:
		chosen = *pick.Winner
	case pick.NeedsMenu && !opts.Yes:
		selected, selErr := promptAssetMenu(pick.Compatible, target)
		if selErr != nil {
			return nil, selErr
		}
		chosen = selected
	case opts.Yes && pick.NeedsMenu:
		return nil, fmt.Errorf("ambiguous asset selection under --yes (top score %d, lead unclear); pass --asset or --format to disambiguate",
			pick.Compatible[0].Score)
	default:
		chosen = pick.Compatible[0]
	}

	if log != nil {
		log.Debug().
			Str("asset", chosen.Asset.Name).
			Int("score", chosen.Score).
			Str("format", chosen.Format).
			Msg("selected github asset")
	}

	color.Cyan("→ Downloading %s (%s)…", chosen.Asset.Name, humanBytes(chosen.Asset.Size))

	cacheDir := filepath.Join(opts.DataDir, "cache", "github", info.Ref.Owner, info.Ref.Repo)
	dl, err := info.Client.DownloadAsset(ctx, chosen.Asset.DownloadURL, chosen.Asset.Name, cacheDir, chosen.Asset.Digest)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	cleanup := func() { _ = os.Remove(dl.Path) }

	if err := github.VerifyLocalFileType(dl.Path, chosen.Format); err != nil {
		cleanup()
		return nil, err
	}

	if opts.Existing != nil && opts.Existing.Tag == rel.TagName && opts.Existing.AssetDigest != "" {
		if !digestsEqual(opts.Existing.AssetDigest, dl.Digest) {
			cleanup()
			return nil, fmt.Errorf("integrity failure: tag %s digest changed (was %s, now %s)",
				rel.TagName, opts.Existing.AssetDigest, dl.Digest)
		}
	}

	_, compareMode := version.Compare(rel.TagName, rel.TagName, rel.PublishedAt, rel.PublishedAt)
	if compareMode == "" {
		compareMode = version.ModeSemver
	}

	src := &core.SourceRef{
		Kind:            "github",
		Owner:           info.Ref.Owner,
		Repo:            info.Ref.Repo,
		RepoID:          info.RepoID,
		Tag:             rel.TagName,
		Version:         version.NormalizeForDisplay(rel.TagName),
		AssetName:       chosen.Asset.Name,
		AssetDigest:     dl.Digest,
		DownloadURL:     chosen.Asset.DownloadURL,
		PublishedAt:     rel.PublishedAt,
		Prerelease:      rel.Prerelease,
		ETag:            info.ETag,
		CompareMode:     compareMode,
		AssetPattern:    opts.AssetRegex,
		ComponentPrefix: version.ComponentPrefix(rel.TagName),
	}
	if opts.Existing != nil {
		src.Hold = opts.Existing.Hold
		if src.ETag == "" {
			// --tag path does not yield a latest-release ETag; keep the old one
			// so conditional (304) polling keeps working on the next update.
			src.ETag = opts.Existing.ETag
		}
		if src.AssetPattern == "" {
			src.AssetPattern = opts.Existing.AssetPattern
		}
		if src.ComponentPrefix == "" {
			src.ComponentPrefix = opts.Existing.ComponentPrefix
		}
	}

	digestShort := dl.Digest
	if len(digestShort) > 19 {
		digestShort = digestShort[:19]
	}
	color.Green("✓ Downloaded %s (%s)", chosen.Asset.Name, digestShort)

	return &githubResolved{
		LocalPath: dl.Path,
		Source:    src,
		Format:    chosen.Format,
		Cleanup:   cleanup,
	}, nil
}

// resolveGitHubPackage turns a GitHub URL / owner/repo into a local file path.
func resolveGitHubPackage(ctx context.Context, input string, opts githubResolveOpts, log *zerolog.Logger) (*githubResolved, error) {
	if !opts.MetadataOnly {
		color.Cyan("→ Fetching %s from GitHub…", input)
	}
	info, err := fetchGitHubRelease(ctx, input, opts)
	if err != nil {
		return nil, err
	}
	if opts.MetadataOnly {
		// Build a SourceRef without downloading (version check path).
		rel := info.Release
		_, mode := version.Compare(rel.TagName, rel.TagName, rel.PublishedAt, rel.PublishedAt)
		src := &core.SourceRef{
			Kind:            "github",
			Owner:           info.Ref.Owner,
			Repo:            info.Ref.Repo,
			RepoID:          info.RepoID,
			Tag:             rel.TagName,
			Version:         version.NormalizeForDisplay(rel.TagName),
			PublishedAt:     rel.PublishedAt,
			Prerelease:      rel.Prerelease,
			ETag:            info.ETag,
			CompareMode:     mode,
			ComponentPrefix: version.ComponentPrefix(rel.TagName),
		}
		if opts.Existing != nil {
			src.AssetName = opts.Existing.AssetName
			src.AssetDigest = opts.Existing.AssetDigest
			src.AssetPattern = opts.Existing.AssetPattern
			src.Hold = opts.Existing.Hold
			if src.ETag == "" {
				src.ETag = opts.Existing.ETag
			}
		}
		return &githubResolved{Source: src, Cleanup: func() {}}, nil
	}
	return pickAndDownload(ctx, info, opts, log)
}

func promptAssetMenu(candidates []assetmatch.Candidate, target assetmatch.Target) (assetmatch.Candidate, error) {
	opts := make([]ui.SelectOption, 0, len(candidates))
	for _, c := range candidates {
		detail := fmt.Sprintf("%s | score %d | %s", c.Format, c.Score, humanBytes(c.Asset.Size))
		if c.Format == "deb" || c.Format == "rpm" {
			detail += " | requires sudo"
		} else {
			detail += " | user-space"
		}
		opts = append(opts, ui.SelectOption{
			Label:  c.Asset.Name,
			Detail: detail,
			Value:  c.Asset.Name,
		})
	}
	label := fmt.Sprintf("Select package to install for %s/%s (%d compatible)", target.OS, target.Arch, len(candidates))
	_, sel, err := ui.SelectPromptDetailed(label, opts)
	if err != nil {
		return assetmatch.Candidate{}, err
	}
	for _, c := range candidates {
		if c.Asset.Name == sel.Value {
			return c, nil
		}
	}
	return assetmatch.Candidate{}, fmt.Errorf("selected asset not found")
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "?"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func regexpQuoteMeta(s string) string {
	special := `\.+*?()|[]{}^$`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func digestsEqual(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(a, "sha256:")))
	b = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(b, "sha256:")))
	return a == b && a != ""
}

// pathShadowNames are common PATH tools we refuse to silently shadow in ~/.local/bin.
var pathShadowNames = map[string]struct{}{
	"sudo": {}, "git": {}, "ls": {}, "cp": {}, "mv": {}, "rm": {}, "bash": {},
	"sh": {}, "zsh": {}, "curl": {}, "wget": {}, "ssh": {}, "systemctl": {},
	"docker": {}, "podman": {}, "python": {}, "python3": {}, "node": {}, "npm": {},
	"pacman": {}, "apt": {}, "dpkg": {}, "rpm": {}, "tar": {}, "chmod": {}, "chown": {},
}

func confirmPathShadow(name string, yes bool) error {
	base := strings.ToLower(strings.TrimSpace(name))
	if _, ok := pathShadowNames[base]; !ok {
		return nil
	}
	if yes {
		return fmt.Errorf("refusing to install asset that would shadow %q in PATH (omit --yes to confirm interactively, or pass --name)", name)
	}
	ok, err := ui.ConfirmDangerousAction(fmt.Sprintf("install a package named %q (shadows a common PATH tool)", name), name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("aborted: would shadow %q", name)
	}
	return nil
}

// confirmRemoteSudo warns when a remote deb/rpm will invoke sudo.
func confirmRemoteSudo(format string, yes bool) error {
	f := strings.ToLower(format)
	if f != "deb" && f != "rpm" {
		return nil
	}
	if yes {
		color.Yellow("Warning: installing remote %s with sudo (non-interactive --yes)", f)
		return nil
	}
	ok, err := ui.ConfirmPrompt(fmt.Sprintf("This remote %s package requires sudo/pacman. Continue?", f))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("aborted: remote %s install requires confirmation", f)
	}
	return nil
}
