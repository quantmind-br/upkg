package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func updateTarget(name string) *core.InstallRecord {
	return &core.InstallRecord{
		InstallID: name, Name: name, PackageType: core.PackageType("binary"),
		Metadata: core.Metadata{Source: &core.SourceRef{
			Kind: "github", Owner: "owner", Repo: name, Tag: "v1.0.0", Version: "1.0.0",
		}},
	}
}

func TestCheckUpdateReleases_ConcurrencyAndOrder(t *testing.T) {
	t.Parallel()
	for _, parallel := range []int{1, 2, 10} {
		t.Run(fmt.Sprint(parallel), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			targets := []*core.InstallRecord{updateTarget("a"), updateTarget("b"), updateTarget("c"), updateTarget("held")}
			targets[3].Metadata.Source.Hold = true
			started := make(chan string, 3)
			gates := map[string]chan struct{}{"a": make(chan struct{}), "b": make(chan struct{}), "c": make(chan struct{})}
			var active, peak atomic.Int32
			resolve := func(ctx context.Context, _ string, opts githubResolveOpts, _ *zerolog.Logger) (*githubResolved, error) {
				assert.True(t, opts.MetadataOnly)
				assert.True(t, opts.Yes)
				assert.Equal(t, "data", opts.DataDir)
				n := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); n > old; old = peak.Load() {
					if peak.CompareAndSwap(old, n) {
						break
					}
				}
				started <- opts.AppName
				select {
				case <-gates[opts.AppName]:
					return &githubResolved{Source: &core.SourceRef{Tag: opts.AppName}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			var checks []updateReleaseCheck
			var err error
			done := make(chan struct{})
			go func() {
				checks, err = checkUpdateReleases(ctx, targets, parallel, "data", nil, resolve)
				close(done)
			}()
			limit := min(parallel, 3)
			initial := make([]string, limit)
			for i := range initial {
				select {
				case initial[i] = <-started:
				case <-ctx.Done():
					t.Fatal("workers did not start concurrently")
				}
			}
			assert.Equal(t, int32(limit), peak.Load())
			// Finish the last started query first, so completion order differs from input order.
			close(gates[initial[limit-1]])
			for remaining := 3 - limit; remaining > 0; remaining-- {
				select {
				case name := <-started:
					close(gates[name])
				case <-ctx.Done():
					t.Fatal("queued query did not start")
				}
			}
			for _, name := range initial[:limit-1] {
				close(gates[name])
			}
			<-done
			require.NoError(t, err)
			assert.LessOrEqual(t, peak.Load(), int32(limit))
			for i, name := range []string{"a", "b", "c"} {
				require.NotNil(t, checks[i].meta)
				assert.Equal(t, name, checks[i].meta.Source.Tag)
			}
			assert.Nil(t, checks[3].meta)
		})
	}
}

func TestCheckUpdateReleases_Cancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	var calls atomic.Int32
	done := make(chan error, 1)
	go func() {
		_, err := checkUpdateReleases(ctx, []*core.InstallRecord{updateTarget("a"), updateTarget("b")}, 1, "", nil,
			func(ctx context.Context, _ string, _ githubResolveOpts, _ *zerolog.Logger) (*githubResolved, error) {
				calls.Add(1)
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not stop")
	}
	assert.Equal(t, int32(1), calls.Load())
}

func TestCheckUpdateReleases_IndividualErrors(t *testing.T) {
	t.Parallel()
	failed := errors.New("rate limited")
	checks, err := checkUpdateReleases(context.Background(), []*core.InstallRecord{updateTarget("a"), updateTarget("b"), updateTarget("c")}, 2, "", nil,
		func(_ context.Context, _ string, opts githubResolveOpts, _ *zerolog.Logger) (*githubResolved, error) {
			switch opts.AppName {
			case "a":
				return nil, errNotModified
			case "b":
				return nil, failed
			default:
				return &githubResolved{Source: &core.SourceRef{Tag: "v2.0.0"}}, nil
			}
		})
	require.NoError(t, err)
	assert.ErrorIs(t, checks[0].err, errNotModified)
	assert.ErrorIs(t, checks[1].err, failed)
	assert.Equal(t, "v2.0.0", checks[2].meta.Source.Tag)
}

func TestUpdateCmd_Validation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--parallel", "0"}, "parallel must be greater than zero"},
		{[]string{"--parallel", "-1"}, "parallel must be greater than zero"},
		{[]string{"--timeout", "0", "--parallel", "0"}, "timeout must be greater than zero seconds"},
	} {
		t.Run(fmt.Sprint(tc.args), func(t *testing.T) {
			t.Parallel()
			log := zerolog.New(io.Discard)
			cmd := NewUpdateCmd(&config.Config{}, &log)
			cmd.SetArgs(tc.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			assert.EqualError(t, cmd.Execute(), tc.want)
		})
	}
}

func TestUpdateCmd_MetadataChecks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		tag     string
		metaErr error
		held    bool
		want    string
		calls   int32
	}{
		{name: "check newer", args: []string{"--check"}, tag: "v2.0.0", want: "1 update(s) available", calls: 1},
		{name: "dry run", args: []string{"--dry-run"}, tag: "v2.0.0", calls: 1},
		{name: "same tag", args: []string{"--check"}, tag: "v1.0.0", calls: 1},
		{name: "304", args: []string{"--check"}, metaErr: errNotModified, calls: 1},
		{name: "downgrade refused", args: []string{"--check"}, tag: "v0.9.0", calls: 1},
		{name: "downgrade allowed", args: []string{"--check", "--allow-downgrade"}, tag: "v0.9.0", want: "1 update(s) available", calls: 1},
		{name: "held", args: []string{"--check"}, held: true},
		{name: "filter match", args: []string{"APP", "--check"}, tag: "v2.0.0", want: "1 update(s) available", calls: 1},
		{name: "filter miss", args: []string{"other", "--check"}},
		{name: "individual error", args: []string{"--check"}, metaErr: errors.New("rate limited"), calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{Paths: config.PathsConfig{DBFile: filepath.Join(t.TempDir(), "test.db")}}
			database, err := db.New(context.Background(), cfg.Paths.DBFile)
			require.NoError(t, err)
			rec := updateTarget("app")
			rec.Metadata.Source.Hold = tc.held
			require.NoError(t, database.Create(context.Background(), &db.Install{
				InstallID: rec.InstallID, Name: rec.Name, PackageType: "binary", Version: "1.0.0",
				InstallDate: time.Now(), InstallPath: "/unused/app", OriginalFile: "/unused/download",
				Metadata: buildDBMetadata(rec),
			}))
			require.NoError(t, database.Close())
			var calls atomic.Int32
			log := zerolog.New(io.Discard)
			cmd := newUpdateCmd(cfg, &log, func(_ context.Context, _ string, opts githubResolveOpts, _ *zerolog.Logger) (*githubResolved, error) {
				calls.Add(1)
				assert.True(t, opts.MetadataOnly, "check/dry-run must not download")
				if tc.metaErr != nil {
					return nil, tc.metaErr
				}
				return &githubResolved{Source: &core.SourceRef{Tag: tc.tag, Version: tc.tag}}, nil
			})
			cmd.SetArgs(tc.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			assert.Equal(t, "4", cmd.Flags().Lookup("parallel").DefValue)
			err = cmd.Execute()
			if tc.want == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.want)
			}
			assert.Equal(t, tc.calls, calls.Load())
		})
	}
}

func TestCheckUpdateReleases_NoEligibleTargets(t *testing.T) {
	t.Parallel()
	held := updateTarget("held")
	held.Metadata.Source.Hold = true
	for _, targets := range [][]*core.InstallRecord{nil, {held}} {
		checks, err := checkUpdateReleases(context.Background(), targets, 4, "", nil,
			func(context.Context, string, githubResolveOpts, *zerolog.Logger) (*githubResolved, error) {
				t.Error("unexpected query")
				return nil, nil
			})
		require.NoError(t, err)
		assert.Len(t, checks, len(targets))
	}
}

func TestUpdateCmd_Context(t *testing.T) {
	t.Parallel()
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{Paths: config.PathsConfig{DBFile: filepath.Join(t.TempDir(), "test.db")}}
			database, err := db.New(context.Background(), cfg.Paths.DBFile)
			require.NoError(t, err)
			rec := updateTarget("app")
			require.NoError(t, database.Create(context.Background(), &db.Install{
				InstallID: rec.InstallID, Name: rec.Name, PackageType: "binary", Version: "1.0.0",
				InstallDate: time.Now(), InstallPath: "/unused/app", OriginalFile: "/unused/download",
				Metadata: buildDBMetadata(rec),
			}))
			require.NoError(t, database.Close())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			log := zerolog.New(io.Discard)
			var calls atomic.Int32
			cmd := newUpdateCmd(cfg, &log, func(ctx context.Context, _ string, opts githubResolveOpts, _ *zerolog.Logger) (*githubResolved, error) {
				calls.Add(1)
				assert.True(t, opts.MetadataOnly, "cancellation must stop before download")
				if !timeout {
					cancel()
				}
				<-ctx.Done()
				return nil, ctx.Err()
			})
			cmd.SetContext(ctx)
			cmd.SetArgs([]string{"--yes", "--timeout", "1"})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			err = cmd.Execute()
			if timeout {
				assert.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				assert.ErrorIs(t, err, context.Canceled)
			}
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}
