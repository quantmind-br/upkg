package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/quantmind-br/upkg/internal/backends"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/quantmind-br/upkg/internal/version"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewUpdateCmd creates the update command for GitHub-sourced packages.
func NewUpdateCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	return newUpdateCmd(cfg, log, resolveGitHubPackage)
}

type githubPackageResolver func(context.Context, string, githubResolveOpts, *zerolog.Logger) (*githubResolved, error)

//nolint:gocyclo // batch update flow branches on hold/check/dry-run/errors per package.
func newUpdateCmd(cfg *config.Config, log *zerolog.Logger, resolve githubPackageResolver) *cobra.Command {
	var (
		dryRun         bool
		checkOnly      bool
		yes            bool
		allowDowngrade bool
		holdNames      []string
		unholdNames    []string
		timeoutSecs    int
		parallel       int
	)

	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Update packages installed from GitHub",
		Long: `Check GitHub releases for newer versions of packages installed from a
GitHub URL or owner/repo reference, and reinstall them in place.
Release checks run in parallel (default: 4); downloads and installs run sequentially.

  upkg update              # all GitHub-sourced packages
  upkg update buzz         # one package by name
  upkg update --check      # exit 1 if any update is available
  upkg update --dry-run    # show plan without installing
  upkg update --yes        # non-interactive (includes remote deb/rpm via sudo)
  upkg update --hold NAME  # freeze a package
  upkg update --unhold NAME`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeoutSecs <= 0 {
				return fmt.Errorf("timeout must be greater than zero seconds")
			}
			if parallel <= 0 {
				return fmt.Errorf("parallel must be greater than zero")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(timeoutSecs)*time.Second)
			defer cancel()

			database, err := db.New(ctx, cfg.Paths.DBFile)
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer func() { _ = database.Close() }()

			if len(holdNames) > 0 || len(unholdNames) > 0 {
				return applyHoldFlags(ctx, database, holdNames, unholdNames)
			}

			installs, err := database.List(ctx)
			if err != nil {
				return fmt.Errorf("list installs: %w", err)
			}

			filterName := ""
			if len(args) == 1 {
				filterName = strings.ToLower(args[0])
			}

			var targets []*core.InstallRecord
			for i := range installs {
				rec := db.ToInstallRecord(&installs[i])
				if rec.Metadata.Source == nil || rec.Metadata.Source.Kind != "github" {
					continue
				}
				if filterName != "" &&
					strings.ToLower(rec.Name) != filterName &&
					strings.ToLower(rec.InstallID) != filterName {
					continue
				}
				targets = append(targets, rec)
			}

			if len(targets) == 0 {
				if filterName != "" {
					color.Yellow("No GitHub-sourced package matching %q", filterName)
				} else {
					color.Yellow("No packages installed from GitHub")
				}
				return nil
			}

			color.Cyan("Checking %d package(s) for updates…", len(targets))

			type updateJob struct {
				rec    *core.InstallRecord
				newTag string
				newVer string
				// non-empty skip reason means do not install
				skip string
			}

			var jobs []updateJob
			available := 0

			checks, err := checkUpdateReleases(ctx, targets, parallel, cfg.Paths.DataDir, log, resolve)
			if err != nil {
				return fmt.Errorf("check updates: %w", err)
			}

			for i, rec := range targets {
				src := rec.Metadata.Source
				job := updateJob{rec: rec}

				if src.Hold {
					job.skip = "held"
					jobs = append(jobs, job)
					color.White("  %-16s  held", rec.Name)
					continue
				}

				meta, metaErr := checks[i].meta, checks[i].err

				if errors.Is(metaErr, errNotModified) {
					job.skip = "up to date"
					jobs = append(jobs, job)
					color.White("  %-16s  %s  up to date", rec.Name, displayVer(src))
					continue
				}
				if metaErr != nil {
					job.skip = metaErr.Error()
					jobs = append(jobs, job)
					color.Red("  %-16s  error: %v", rec.Name, metaErr)
					continue
				}

				remote := meta.Source
				job.newTag = remote.Tag
				job.newVer = remote.Version

				if src.Tag == remote.Tag {
					job.skip = "up to date"
					jobs = append(jobs, job)
					color.White("  %-16s  %s  up to date", rec.Name, displayVer(src))
					continue
				}

				newer, _ := version.IsNewer(src.Tag, remote.Tag, src.PublishedAt, remote.PublishedAt)
				if !newer {
					newer, _ = version.IsNewer(src.Version, remote.Version, src.PublishedAt, remote.PublishedAt)
				}

				if !newer && !allowDowngrade {
					job.skip = "downgrade refused"
					jobs = append(jobs, job)
					color.Yellow("  %-16s  %s → %s  (downgrade skipped)", rec.Name, displayVer(src), remote.Version)
					continue
				}

				available++
				jobs = append(jobs, job)
				color.Green("  %-16s  %s → %s  %s", rec.Name, displayVer(src), remote.Version, rec.PackageType)
			}

			if checkOnly {
				if available > 0 {
					return fmt.Errorf("%d update(s) available", available)
				}
				color.Green("All packages up to date")
				return nil
			}
			if available == 0 {
				color.Green("Nothing to update")
				return nil
			}
			if dryRun {
				color.Cyan("Dry-run: %d package(s) would be updated", available)
				return nil
			}

			if !yes {
				ok, confErr := ui.ConfirmWithDefault(fmt.Sprintf("Update %d package(s)?", available), true)
				if confErr != nil {
					return confErr
				}
				if !ok {
					color.Yellow("Aborted")
					return nil
				}
			}

			registry := backends.NewRegistry(cfg, log)
			var errs []error
			updated := 0

			for _, job := range jobs {
				if job.skip != "" {
					continue
				}
				rec := job.rec
				src := rec.Metadata.Source
				input := src.Owner + "/" + src.Repo

				color.Cyan("→ Updating %s…", rec.Name)
				resolved, resErr := resolve(ctx, input, githubResolveOpts{
					// Pin to the tag we already decided is newer when possible.
					Tag:             job.newTag,
					Prerelease:      src.Prerelease || strings.Contains(job.newTag, "-"),
					Yes:             true,
					AssetRegex:      src.AssetPattern,
					PreferAssetName: src.AssetName,
					Existing:        src,
					DataDir:         cfg.Paths.DataDir,
					AppName:         rec.Name,
				}, log)
				if resErr != nil {
					color.Red("  %s: %v", rec.Name, resErr)
					errs = append(errs, resErr)
					continue
				}
				cleanup := resolved.Cleanup
				if cleanup == nil {
					cleanup = func() {}
				}

				if confErr := confirmRemoteSudo(resolved.Format, yes); confErr != nil {
					cleanup()
					color.Red("  %s: %v", rec.Name, confErr)
					errs = append(errs, confErr)
					continue
				}

				backend, detErr := registry.DetectBackend(ctx, resolved.LocalPath)
				if detErr != nil {
					cleanup()
					color.Red("  %s: %v", rec.Name, detErr)
					errs = append(errs, detErr)
					continue
				}

				tx := transaction.NewManager(log)
				tx.Add("remove_update_download", func() error {
					cleanup()
					return nil
				})

				newRec, instErr := backend.Install(ctx, resolved.LocalPath, core.InstallOptions{
					Force:      true,
					CustomName: rec.Name,
					Overwrite:  false,
				}, tx)
				if instErr != nil {
					if rbErr := tx.Rollback(); rbErr != nil {
						log.Warn().Err(rbErr).Str("name", rec.Name).Msg("rollback after install failure")
					}
					color.Red("  %s: install failed: %v", rec.Name, instErr)
					errs = append(errs, instErr)
					continue
				}

				newRec.Metadata.Source = resolved.Source
				if newRec.Version == "" && resolved.Source != nil {
					newRec.Version = resolved.Source.Version
				}

				dbRecord := &db.Install{
					InstallID:    newRec.InstallID,
					PackageType:  string(newRec.PackageType),
					Name:         newRec.Name,
					Version:      newRec.Version,
					InstallDate:  newRec.InstallDate,
					OriginalFile: newRec.OriginalFile,
					InstallPath:  newRec.InstallPath,
					DesktopFile:  newRec.DesktopFile,
					Metadata:     buildDBMetadata(newRec),
				}

				if isLocalPackageType(newRec.PackageType) {
					canonical, repErr := database.ReplaceByPath(ctx, dbRecord)
					if repErr != nil {
						if rbErr := tx.Rollback(); rbErr != nil {
							log.Warn().Err(rbErr).Str("name", rec.Name).Msg("rollback after db replace failure")
						}
						color.Red("  %s: db replace failed: %v", rec.Name, repErr)
						errs = append(errs, repErr)
						continue
					}
					_ = canonical
				} else {
					dbRecord.InstallID = rec.InstallID
					dbRecord.InstallDate = rec.InstallDate
					if upErr := database.Update(ctx, dbRecord); upErr != nil {
						if crErr := database.Create(ctx, dbRecord); crErr != nil {
							if rbErr := tx.Rollback(); rbErr != nil {
								log.Warn().Err(rbErr).Str("name", rec.Name).Msg("rollback after db save failure")
							}
							errs = append(errs, crErr)
							color.Red("  %s: db save failed: %v", rec.Name, crErr)
							continue
						}
					}
				}

				if commitErr := tx.Commit(); commitErr != nil {
					log.Warn().Err(commitErr).Str("name", rec.Name).Msg("update commit cleanup failed")
				}
				cleanup()
				updated++
				color.Green("✓ %s updated to %s", rec.Name, job.newVer)
			}

			color.Cyan("Updated %d package(s)", updated)
			if len(errs) > 0 {
				return errors.Join(errs...)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show available updates without installing")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "check only; exit 1 if updates are available")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "non-interactive confirmation (includes remote deb/rpm)")
	cmd.Flags().BoolVar(&allowDowngrade, "allow-downgrade", false, "allow installing an older release")
	cmd.Flags().StringArrayVar(&holdNames, "hold", nil, "mark package(s) as held (skip updates)")
	cmd.Flags().StringArrayVar(&unholdNames, "unhold", nil, "clear hold on package(s)")
	cmd.Flags().IntVar(&parallel, "parallel", 4, "maximum simultaneous release checks (downloads and installs remain sequential)")
	cmd.Flags().IntVar(&timeoutSecs, "timeout", 600, "timeout in seconds")

	return cmd
}

type updateReleaseCheck struct {
	meta *githubResolved
	err  error
}

// checkUpdateReleases writes each result to its own slot; only the caller renders output.
func checkUpdateReleases(ctx context.Context, targets []*core.InstallRecord, parallel int, dataDir string, log *zerolog.Logger, resolve githubPackageResolver) ([]updateReleaseCheck, error) {
	if parallel <= 0 {
		return nil, fmt.Errorf("parallel must be greater than zero")
	}
	checks := make([]updateReleaseCheck, len(targets))
	eligible := 0
	for _, rec := range targets {
		if !rec.Metadata.Source.Hold {
			eligible++
		}
	}
	work := make(chan int)
	var workers sync.WaitGroup
	for range min(parallel, eligible) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range work {
				if ctx.Err() != nil {
					return
				}
				rec := targets[i]
				src := rec.Metadata.Source
				checks[i].meta, checks[i].err = resolve(ctx, src.Owner+"/"+src.Repo, githubResolveOpts{
					Prerelease:      src.Prerelease,
					Yes:             true,
					AssetRegex:      src.AssetPattern,
					PreferAssetName: src.AssetName,
					Existing:        src,
					DataDir:         dataDir,
					AppName:         rec.Name,
					MetadataOnly:    true,
				}, log)
			}
		}()
	}
dispatch:
	for i, rec := range targets {
		if ctx.Err() != nil {
			break
		}
		if rec.Metadata.Source.Hold {
			continue
		}
		select {
		case <-ctx.Done():
			break dispatch
		case work <- i:
		}
	}
	close(work)
	workers.Wait()
	return checks, ctx.Err()
}

func displayVer(src *core.SourceRef) string {
	if src == nil {
		return ""
	}
	if src.Version != "" {
		return src.Version
	}
	return src.Tag
}

func applyHoldFlags(ctx context.Context, database *db.DB, hold, unhold []string) error {
	installs, err := database.List(ctx)
	if err != nil {
		return err
	}
	byName := map[string]*db.Install{}
	for i := range installs {
		byName[strings.ToLower(installs[i].Name)] = &installs[i]
		byName[strings.ToLower(installs[i].InstallID)] = &installs[i]
	}

	setHold := func(name string, value bool) error {
		inst, ok := byName[strings.ToLower(name)]
		if !ok {
			return fmt.Errorf("package not found: %s", name)
		}
		rec := db.ToInstallRecord(inst)
		if rec.Metadata.Source == nil || rec.Metadata.Source.Kind != "github" {
			return fmt.Errorf("%s has no GitHub source metadata", name)
		}
		rec.Metadata.Source.Hold = value
		inst.Metadata = buildDBMetadata(rec)
		if err := database.Update(ctx, inst); err != nil {
			return err
		}
		state := "held"
		if !value {
			state = "unheld"
		}
		color.Green("✓ %s %s", rec.Name, state)
		return nil
	}

	var errs []error
	for _, n := range hold {
		if err := setHold(n, true); err != nil {
			errs = append(errs, err)
			color.Red("Error: %v", err)
		}
	}
	for _, n := range unhold {
		if err := setHold(n, false); err != nil {
			errs = append(errs, err)
			color.Red("Error: %v", err)
		}
	}
	return errors.Join(errs...)
}
