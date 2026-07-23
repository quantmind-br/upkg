package appimage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/paths"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDesktopBackend builds an AppImage backend whose home is an isolated temp
// dir, so createDesktopFile writes under home/.local/share/applications instead
// of leaking into the real user home (BUG-010/011).
func newDesktopBackend(t *testing.T, cfg *config.Config) (*AppImageBackend, string) {
	t.Helper()
	home := t.TempDir()
	logger := zerolog.New(io.Discard)
	b := NewWithDeps(cfg, &logger, afero.NewOsFs(), helpers.NewOSCommandRunner())
	b.Paths = paths.NewResolverWithHome(cfg, home)
	return b, home
}

func TestAppImageBackend_createDesktopFile_WithExistingDesktop(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{DataDir: tmpDir}}
	backend, home := newDesktopBackend(t, cfg)

	squashfsRoot := filepath.Join(tmpDir, "squashfs")
	applicationsDir := filepath.Join(squashfsRoot, "usr", "share", "applications")
	require.NoError(t, os.MkdirAll(applicationsDir, 0755))

	desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=testapp
Categories=Utility;`
	desktopPath := filepath.Join(applicationsDir, "testapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

	metadata := &appImageMetadata{desktopFile: desktopPath, icon: "testapp"}
	execPath := "/opt/testapp.TestImage"

	resultPath, err := backend.createDesktopFile(squashfsRoot, "TestApp", "testapp", execPath, metadata, core.InstallOptions{})
	require.NoError(t, err)

	// The file is written under the isolated home, not the real user home.
	assert.Equal(t, filepath.Join(home, ".local", "share", "applications", "testapp.desktop"), resultPath)
	content, readErr := os.ReadFile(resultPath)
	require.NoError(t, readErr)
	got := string(content)
	assert.Contains(t, got, "Name=TestApp")
	assert.Contains(t, got, execPath, "Exec must point at the installed AppImage")
	assert.Contains(t, got, "Icon=testapp")
}

func TestAppImageBackend_createDesktopFile_WithoutDesktop(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{DataDir: tmpDir}}
	backend, home := newDesktopBackend(t, cfg)

	metadata := &appImageMetadata{}
	execPath := "/opt/testapp.TestImage"

	resultPath, err := backend.createDesktopFile(tmpDir, "TestApp", "testapp", execPath, metadata, core.InstallOptions{})
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, ".local", "share", "applications", "testapp.desktop"), resultPath)
	content, readErr := os.ReadFile(resultPath)
	require.NoError(t, readErr)
	got := string(content)
	// A synthesized entry falls back to binName for the icon and defaults categories.
	assert.Contains(t, got, "Name=TestApp")
	assert.Contains(t, got, "Icon=testapp")
	assert.Contains(t, got, "Categories=Utility")
}

func TestAppImageBackend_createDesktopFile_WithElectron(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths:   config.PathsConfig{DataDir: tmpDir},
		Desktop: config.DesktopConfig{ElectronDisableSandbox: true},
	}
	backend, _ := newDesktopBackend(t, cfg)

	squashfsRoot := tmpDir
	resourcesDir := filepath.Join(squashfsRoot, "resources")
	require.NoError(t, os.MkdirAll(resourcesDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("fake"), 0644))

	metadata := &appImageMetadata{icon: "testapp"}

	resultPath, err := backend.createDesktopFile(squashfsRoot, "ElectronApp", "electronapp", "/opt/electronapp.AppImage", metadata, core.InstallOptions{})
	require.NoError(t, err)
	content, readErr := os.ReadFile(resultPath)
	require.NoError(t, readErr)
	// Electron app with sandbox disabling must inject --no-sandbox into Exec.
	assert.Contains(t, string(content), "--no-sandbox")
}

func TestAppImageBackend_createDesktopFile_WithWaylandEnv(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths:   config.PathsConfig{DataDir: tmpDir},
		Desktop: config.DesktopConfig{WaylandEnvVars: true},
	}
	backend, _ := newDesktopBackend(t, cfg)

	metadata := &appImageMetadata{icon: "testapp"}

	resultPath, err := backend.createDesktopFile(tmpDir, "TestApp", "testapp", "/opt/testapp.AppImage", metadata, core.InstallOptions{SkipWaylandEnv: false})
	require.NoError(t, err)
	content, readErr := os.ReadFile(resultPath)
	require.NoError(t, readErr)
	// Wayland injection prepends an env prefix with the known variables.
	assert.Contains(t, string(content), "GDK_BACKEND=wayland,x11")
	assert.Contains(t, string(content), "ELECTRON_OZONE_PLATFORM_HINT=auto")
}

func TestAppImageBackend_createDesktopFile_SkipWaylandEnv(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths:   config.PathsConfig{DataDir: tmpDir},
		Desktop: config.DesktopConfig{WaylandEnvVars: true},
	}
	backend, _ := newDesktopBackend(t, cfg)

	metadata := &appImageMetadata{icon: "testapp"}

	resultPath, err := backend.createDesktopFile(tmpDir, "TestApp", "testapp", "/opt/testapp.AppImage", metadata, core.InstallOptions{SkipWaylandEnv: true})
	require.NoError(t, err)
	content, readErr := os.ReadFile(resultPath)
	require.NoError(t, readErr)
	// With SkipWaylandEnv, no env prefix is injected.
	assert.NotContains(t, string(content), "GDK_BACKEND=wayland,x11")
}

func TestAppImageBackend_createDesktopFile_WithTauriApp(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths:   config.PathsConfig{DataDir: tmpDir},
		Desktop: config.DesktopConfig{WaylandEnvVars: true},
	}
	backend, _ := newDesktopBackend(t, cfg)

	squashfsRoot := tmpDir
	applicationsDir := filepath.Join(squashfsRoot, "usr", "share", "applications")
	require.NoError(t, os.MkdirAll(applicationsDir, 0755))
	desktopContent := `[Desktop Entry]
Type=Application
Name=TauriApp
StartupWMClass=tauri-app
Exec=tauriapp`
	desktopPath := filepath.Join(applicationsDir, "tauriapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

	metadata := &appImageMetadata{desktopFile: desktopPath, icon: "tauriapp"}

	resultPath, err := backend.createDesktopFile(squashfsRoot, "TauriApp", "tauriapp", "/opt/tauriapp.AppImage", metadata, core.InstallOptions{})
	require.NoError(t, err)
	content, readErr := os.ReadFile(resultPath)
	require.NoError(t, readErr)
	// Tauri apps must NOT get Wayland env injection despite WaylandEnvVars=true.
	assert.NotContains(t, string(content), "GDK_BACKEND=wayland,x11")
}

func TestAppImageBackend_createDesktopFile_WithCustomEnvVars(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{DataDir: tmpDir},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
			CustomEnvVars:  []string{"CUSTOM_VAR=value"},
		},
	}
	backend, _ := newDesktopBackend(t, cfg)

	metadata := &appImageMetadata{icon: "testapp"}

	resultPath, err := backend.createDesktopFile(tmpDir, "TestApp", "testapp", "/opt/testapp.AppImage", metadata, core.InstallOptions{})
	require.NoError(t, err)
	content, readErr := os.ReadFile(resultPath)
	require.NoError(t, readErr)
	// Custom env var must appear alongside the standard Wayland ones.
	assert.Contains(t, string(content), "CUSTOM_VAR=value")
	assert.Contains(t, string(content), "GDK_BACKEND=wayland,x11")
}

// TestAppImageBackend_extractAppImage_SourceUnchangedOnSuccess verifies BUG-005:
// the user's AppImage file is never chmod'd or written to. Extraction runs from
// a temporary executable copy inside destDir; the primary --appimage-extract is
// mocked to succeed, and the original 0644 source must be byte- and
// mode-identical afterwards.
func TestAppImageBackend_extractAppImage_SourceUnchangedOnSuccess(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{DataDir: tmpDir}}
	logger := zerolog.New(io.Discard)

	srcContent := []byte("original appimage bytes")
	srcPath := filepath.Join(tmpDir, "app.AppImage")
	require.NoError(t, os.WriteFile(srcPath, srcContent, 0644))

	destDir := filepath.Join(tmpDir, "dest")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	// Mock runner: the primary --appimage-extract "succeeds" by creating the
	// expected squashfs-root directory inside destDir. It also asserts the
	// invocation runs the temporary executable copy (never the source) from
	// destDir, and that the copy is executable (0755) at execution time.
	expectedCopy := filepath.Join(destDir, ".upkg-extract.AppImage")
	var ran bool
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(_ string) bool { return true },
		RunCommandInDirFunc: func(_ context.Context, dir, name string, args ...string) (string, error) {
			ran = true
			assert.Equal(t, destDir, dir, "extraction must run from destDir")
			assert.Equal(t, expectedCopy, name, "must execute the temp copy, not the source")
			assert.NotEqual(t, srcPath, name, "source must never be executed directly")
			assert.Equal(t, []string{"--appimage-extract"}, args)
			ci, statErr := os.Stat(name)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0755), ci.Mode().Perm(), "temp copy must be executable")
			return "", os.MkdirAll(filepath.Join(dir, "squashfs-root"), 0755)
		},
	}
	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

	require.NoError(t, backend.extractAppImage(context.Background(), srcPath, destDir))
	assert.True(t, ran, "primary --appimage-extract should have run")

	// Source content and permissions must be untouched.
	info, err := os.Stat(srcPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), "source must not be chmod'd")
	got, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, srcContent, got, "source content must be unchanged")

	// The temporary executable copy must be cleaned up, not left in destDir.
	_, statErr := os.Stat(filepath.Join(destDir, ".upkg-extract.AppImage"))
	assert.True(t, os.IsNotExist(statErr), "temp extraction copy should be removed")
}

// TestAppImageBackend_extractAppImage_UnsquashfsFallback verifies BUG-006: when
// the primary --appimage-extract fails, the fallback runs unsquashfs from
// destDir with an explicit "-d squashfs-root" and the ABSOLUTE source path, so
// extraction never lands in the process working directory. The source file is
// read (never executed or modified).
func TestAppImageBackend_extractAppImage_UnsquashfsFallback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{DataDir: tmpDir}}
	logger := zerolog.New(io.Discard)

	srcContent := []byte("original appimage bytes")
	srcPath := filepath.Join(tmpDir, "app.AppImage")
	require.NoError(t, os.WriteFile(srcPath, srcContent, 0644))
	absSrc, err := filepath.Abs(srcPath)
	require.NoError(t, err)

	destDir := filepath.Join(tmpDir, "dest")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	var sawUnsquashfs, sawPrimary bool
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(cmd string) bool { return cmd == "unsquashfs" },
		RunCommandInDirFunc: func(_ context.Context, dir, name string, args ...string) (string, error) {
			if name != "unsquashfs" {
				// Primary attempt: it must run the temp copy from destDir (never
				// the source), then fail so the fallback path is taken.
				sawPrimary = true
				assert.Equal(t, destDir, dir, "primary must run from destDir")
				assert.Equal(t, filepath.Join(destDir, ".upkg-extract.AppImage"), name,
					"primary must execute the temp copy, not the source")
				assert.NotEqual(t, srcPath, name, "source must never be executed directly")
				assert.Equal(t, []string{"--appimage-extract"}, args)
				return "", assert.AnError
			}
			sawUnsquashfs = true
			assert.Equal(t, destDir, dir, "unsquashfs must run from destDir")
			assert.Equal(t, []string{"-d", "squashfs-root", absSrc}, args,
				"unsquashfs must extract to squashfs-root from the absolute source")
			return "", os.MkdirAll(filepath.Join(dir, "squashfs-root"), 0755)
		},
	}
	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

	require.NoError(t, backend.extractAppImage(context.Background(), srcPath, destDir))
	assert.True(t, sawPrimary, "primary --appimage-extract should have been attempted first")
	assert.True(t, sawUnsquashfs, "fallback unsquashfs should have run")

	// Source must be untouched by the fallback path too.
	info, statErr := os.Stat(srcPath)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	got, readErr := os.ReadFile(srcPath)
	require.NoError(t, readErr)
	assert.Equal(t, srcContent, got)

	// The temporary executable copy must be cleaned up even when the primary
	// attempt failed and the fallback took over.
	_, copyErr := os.Stat(filepath.Join(destDir, ".upkg-extract.AppImage"))
	assert.True(t, os.IsNotExist(copyErr), "temp extraction copy should be removed")
}

func TestAppImageBackend_installIcons(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{DataDir: tmpDir}}
	backend, home := newDesktopBackend(t, cfg)

	// Create a discoverable icon in the squashfs root.
	squashfsRoot := filepath.Join(tmpDir, "squashfs")
	iconDir := filepath.Join(squashfsRoot, "usr", "share", "icons", "hicolor", "256x256", "apps")
	require.NoError(t, os.MkdirAll(iconDir, 0755))
	iconPath := filepath.Join(iconDir, "testapp.png")
	require.NoError(t, os.WriteFile(iconPath, []byte("fake icon"), 0644))

	installed, err := backend.installIcons(squashfsRoot, "testapp", &appImageMetadata{})
	require.NoError(t, err)
	require.NotEmpty(t, installed, "a discoverable icon must be installed")

	// Every returned path must live under the isolated home and exist on disk.
	iconsRoot := filepath.Join(home, ".local", "share", "icons")
	for _, p := range installed {
		assert.True(t, strings.HasPrefix(p, iconsRoot), "icon must be installed under the isolated home: %s", p)
		assert.FileExists(t, p)
	}
}

func TestAppImageBackend_Detect(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	ctx := context.Background()

	t.Run("non-existent file", func(t *testing.T) {
		isAppImage, err := backend.Detect(ctx, "/nonexistent/file.AppImage")
		assert.NoError(t, err)
		assert.False(t, isAppImage)
	})

	t.Run("file without AppImage extension", func(t *testing.T) {
		otherFile := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(otherFile, []byte("not an appimage"), 0644))

		isAppImage, err := backend.Detect(ctx, otherFile)
		assert.NoError(t, err)
		assert.False(t, isAppImage)
	})

	t.Run("file with AppImage extension but invalid content", func(t *testing.T) {
		appImageFile := filepath.Join(tmpDir, "test.AppImage")
		require.NoError(t, os.WriteFile(appImageFile, []byte("not a real appimage"), 0755))

		isAppImage, err := backend.Detect(ctx, appImageFile)
		assert.NoError(t, err)
		assert.False(t, isAppImage)
	})
}

func TestAppImageBackend_Uninstall(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{Paths: config.PathsConfig{DataDir: tmpDir}}
	backend, home := newDesktopBackend(t, cfg)

	// Materialize a full installation: binary, desktop file, and an icon.
	binPath := filepath.Join(home, ".local", "bin", "testapp.appimage")
	desktopPath := filepath.Join(home, ".local", "share", "applications", "testapp.desktop")
	iconPath := filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps", "testapp.png")
	for _, p := range []string{binPath, desktopPath, iconPath} {
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0644))
	}

	record := &core.InstallRecord{
		InstallID:   "test-123",
		Name:        "testapp",
		PackageType: "appimage",
		InstallPath: binPath,
		Metadata:    core.Metadata{IconFiles: []string{iconPath}},
		DesktopFile: desktopPath,
	}

	require.NoError(t, backend.Uninstall(context.Background(), record))

	// All installed artifacts must be gone.
	assert.NoFileExists(t, binPath)
	assert.NoFileExists(t, desktopPath)
	assert.NoFileExists(t, iconPath)
}

func TestAppImageBackend_parseAppImageMetadata(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// parseAppImageMetadata globs the squashfs ROOT for *.desktop (the AppImage
	// convention), so the entry lives at the top level.
	squashfsRoot := tmpDir
	desktopContent := `[Desktop Entry]
Type=Application
Name=Test App Display
Exec=testapp
Icon=testapp-icon
Comment=A test app
Categories=Utility;`
	desktopPath := filepath.Join(squashfsRoot, "testapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

	metadata, err := backend.parseAppImageMetadata(squashfsRoot)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	// appName derives from the FILENAME (application ID), not the Name field.
	assert.Equal(t, "testapp", metadata.appName)
	assert.Equal(t, "testapp-icon", metadata.icon)
	assert.Equal(t, "A test app", metadata.comment)
	assert.Equal(t, desktopPath, metadata.desktopFile)
}
