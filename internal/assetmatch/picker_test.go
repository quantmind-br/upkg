package assetmatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real release assets from block/buzz desktop-v0.5.5 (collected 2026-08-06).
var buzzAssets = []Asset{
	{Name: "Buzz_0.5.5_amd64.deb", Size: 108 * 1024 * 1024},
	{Name: "Buzz_0.5.5_amd64.AppImage", Size: 166 * 1024 * 1024},
	{Name: "Buzz_0.5.5_x64.exe", Size: 150 * 1024 * 1024},
	{Name: "Buzz_0.5.5_x64.dmg", Size: 140 * 1024 * 1024},
	{Name: "Buzz_0.5.5_x64.app.tar.gz", Size: 130 * 1024 * 1024},
	{Name: "Buzz_0.5.5_aarch64.app.tar.gz", Size: 125 * 1024 * 1024},
	{Name: "Buzz_0.5.5_amd64.deb.sig", Size: 500},
	{Name: "Buzz_0.5.5_amd64.AppImage.sig", Size: 500},
	{Name: "Buzz_0.5.5_x64.exe.sig", Size: 500},
	{Name: "Buzz_0.5.5_x64.dmg.sig", Size: 500},
	{Name: "latest-linux.yml", Size: 200},
}

func TestPick_Buzz_Amd64Glibc(t *testing.T) {
	t.Parallel()

	target := Target{OS: "linux", Arch: "amd64", Libc: "glibc", DistroID: "ubuntu"}
	res, err := Pick(buzzAssets, target, PickOptions{Yes: true, AppName: "buzz"})
	require.NoError(t, err)
	require.NotNil(t, res.Winner)
	assert.Equal(t, "Buzz_0.5.5_amd64.deb", res.Winner.Asset.Name)
	assert.Equal(t, "deb", res.Winner.Format)
	assert.GreaterOrEqual(t, res.Winner.Score, 60)

	// AppImage is second and still compatible
	require.GreaterOrEqual(t, len(res.Compatible), 2)
	assert.Equal(t, "Buzz_0.5.5_amd64.AppImage", res.Compatible[1].Asset.Name)

	// macOS app tarball must be excluded (darwin), not ranked as linux tarball
	for _, c := range res.Compatible {
		assert.NotContains(t, c.Asset.Name, ".app.tar")
	}
}

func TestPick_Buzz_WithoutYes_NeedsMenu(t *testing.T) {
	t.Parallel()

	target := Target{OS: "linux", Arch: "amd64", Libc: "glibc", DistroID: "ubuntu"}
	res, err := Pick(buzzAssets, target, PickOptions{Yes: false, AppName: "buzz"})
	require.NoError(t, err)
	assert.True(t, res.NeedsMenu)
	// Winner not auto-applied without --yes when multiple candidates
	if len(res.Compatible) > 1 {
		assert.Nil(t, res.Winner)
	}
}

func TestPick_Buzz_FormatAppImage(t *testing.T) {
	t.Parallel()

	target := Target{OS: "linux", Arch: "amd64", Libc: "glibc"}
	res, err := Pick(buzzAssets, target, PickOptions{Yes: true, Format: "appimage"})
	require.NoError(t, err)
	require.NotNil(t, res.Winner)
	assert.Equal(t, "Buzz_0.5.5_amd64.AppImage", res.Winner.Asset.Name)
}

func TestPick_Ripgrep_GnuVsMusl(t *testing.T) {
	t.Parallel()

	assets := []Asset{
		{Name: "ripgrep-15.2.0-x86_64-unknown-linux-gnu.tar.gz", Size: 2 * 1024 * 1024},
		{Name: "ripgrep-15.2.0-x86_64-unknown-linux-musl.tar.gz", Size: 2 * 1024 * 1024},
		{Name: "ripgrep-15.2.0-aarch64-unknown-linux-gnu.tar.gz", Size: 2 * 1024 * 1024},
		{Name: "ripgrep-15.2.0-x86_64-apple-darwin.tar.gz", Size: 2 * 1024 * 1024},
		{Name: "ripgrep-15.2.0-x86_64-pc-windows-msvc.zip", Size: 2 * 1024 * 1024},
		{Name: "ripgrep-15.2.0-armv7-unknown-linux-gnueabihf.tar.gz", Size: 1 * 1024 * 1024},
		{Name: "ripgrep_15.2.0-1_amd64.deb", Size: 2 * 1024 * 1024},
		{Name: "SHA256SUMS", Size: 1024},
	}

	t.Run("glibc prefers gnu", func(t *testing.T) {
		t.Parallel()
		target := Target{OS: "linux", Arch: "amd64", Libc: "glibc"}
		res, err := Pick(assets, target, PickOptions{Yes: true, AppName: "ripgrep", Format: "tarball"})
		require.NoError(t, err)
		require.NotNil(t, res.Winner)
		assert.Contains(t, res.Winner.Asset.Name, "linux-gnu")
		assert.NotContains(t, res.Winner.Asset.Name, "musl")
	})

	t.Run("musl prefers musl", func(t *testing.T) {
		t.Parallel()
		target := Target{OS: "linux", Arch: "amd64", Libc: "musl"}
		res, err := Pick(assets, target, PickOptions{Yes: true, AppName: "ripgrep", Format: "tarball"})
		require.NoError(t, err)
		require.NotNil(t, res.Winner)
		assert.Contains(t, res.Winner.Asset.Name, "musl")
	})
}

func TestPick_Lapce_MultiDeb_NeedsMenu(t *testing.T) {
	t.Parallel()

	assets := []Asset{
		{Name: "lapce.debian.bookworm.amd64.deb", Size: 20 * 1024 * 1024},
		{Name: "lapce.debian.bullseye.amd64.deb", Size: 20 * 1024 * 1024},
		{Name: "lapce.ubuntu.noble.amd64.deb", Size: 20 * 1024 * 1024},
		{Name: "lapce.ubuntu.jammy.amd64.deb", Size: 20 * 1024 * 1024},
		{Name: "lapce.fedora.amd64.rpm", Size: 20 * 1024 * 1024},
		{Name: "lapce.ubuntu.noble.arm64.deb", Size: 20 * 1024 * 1024},
	}

	target := Target{OS: "linux", Arch: "amd64", Libc: "glibc", DistroID: "ubuntu"}
	res, err := Pick(assets, target, PickOptions{Yes: true, Format: "deb"})
	require.NoError(t, err)
	// Near-tied multi-deb scores are not a clear winner; --yes still needs --asset.
	assert.True(t, res.NeedsMenu)
	assert.Nil(t, res.Winner)
	assert.GreaterOrEqual(t, len(res.Compatible), 3)

	// Without --yes, menu is required.
	resMenu, err := Pick(assets, target, PickOptions{Yes: false, Format: "deb"})
	require.NoError(t, err)
	assert.True(t, resMenu.NeedsMenu)
	assert.Nil(t, resMenu.Winner)

	// --asset resolves to an exact distro package (single match → auto-winner).
	res2, err := Pick(assets, target, PickOptions{Yes: true, Format: "deb", AssetRegex: `noble\.amd64`})
	require.NoError(t, err)
	require.NotNil(t, res2.Winner)
	assert.Equal(t, "lapce.ubuntu.noble.amd64.deb", res2.Winner.Asset.Name)
}

func TestPick_ExcludesZsyncAndChecksums(t *testing.T) {
	t.Parallel()

	assets := []Asset{
		{Name: "nvim-linux-x86_64.appimage", Size: 10 * 1024 * 1024},
		{Name: "nvim-linux-x86_64.appimage.zsync", Size: 100 * 1024},
		{Name: "nvim-linux-x86_64.appimage.sha256", Size: 100},
		{Name: "SHA256SUMS", Size: 500},
	}
	target := Target{OS: "linux", Arch: "amd64"}
	res, err := Pick(assets, target, PickOptions{Yes: true})
	require.NoError(t, err)
	require.NotNil(t, res.Winner)
	assert.Equal(t, "nvim-linux-x86_64.appimage", res.Winner.Asset.Name)
	assert.Len(t, res.Compatible, 1)
}

func TestPick_NothingMatches_Explains(t *testing.T) {
	t.Parallel()

	assets := []Asset{
		{Name: "thing-windows-amd64.exe", Size: 5 * 1024 * 1024},
		{Name: "thing.sig", Size: 100},
	}
	target := Target{OS: "linux", Arch: "amd64"}
	res, err := Pick(assets, target, PickOptions{})
	require.NoError(t, err)
	assert.Empty(t, res.Compatible)
	assert.NotEmpty(t, res.Excluded)
	msg := ExplainExcluded(res.Excluded)
	assert.Contains(t, msg, "no compatible")
	assert.Contains(t, msg, "--asset")
}

func TestDetectOS_AppBundle(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "darwin", detectOSFromName("Buzz_0.5.5_x64.app.tar.gz"))
	assert.Equal(t, "darwin", detectOSFromName("Buzz_0.5.5_x64.dmg"))
	assert.Equal(t, "windows", detectOSFromName("Buzz_0.5.5_x64.exe"))
	assert.Equal(t, "linux", detectOSFromName("foo-linux-amd64.tar.gz"))
}

func TestDetectArch(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "amd64", detectArchFromName("foo-x86_64.tar.gz"))
	assert.Equal(t, "amd64", detectArchFromName("foo_amd64.deb"))
	assert.Equal(t, "arm64", detectArchFromName("foo-aarch64.AppImage"))
	// Version numbers must not look like arch
	assert.Equal(t, "", detectArchFromName("project-1.64.0.tar.gz"))
}

func TestInvalidAssetRegex(t *testing.T) {
	t.Parallel()
	_, err := Pick(nil, Target{}, PickOptions{AssetRegex: "(unclosed"})
	require.Error(t, err)
}

func TestHostTarget(t *testing.T) {
	t.Parallel()
	tg := HostTarget("arch", "glibc")
	assert.NotEmpty(t, tg.OS)
	assert.NotEmpty(t, tg.Arch)
	assert.Equal(t, "glibc", tg.Libc)
	assert.Equal(t, "arch", tg.DistroID)
}

func TestScore_DebBeatsAppImageOnUbuntu(t *testing.T) {
	t.Parallel()
	// Design validation: Buzz deb 865-ish > AppImage 845-ish on ubuntu amd64 glibc
	target := Target{OS: "linux", Arch: "amd64", Libc: "glibc", DistroID: "ubuntu"}
	deb := scoreAsset(Asset{Name: "Buzz_0.5.5_amd64.deb", Size: 108 * 1024 * 1024}, "deb", target, "buzz")
	ai := scoreAsset(Asset{Name: "Buzz_0.5.5_amd64.AppImage", Size: 166 * 1024 * 1024}, "appimage", target, "buzz")
	assert.Greater(t, deb, ai, "deb=%d appimage=%d", deb, ai)
}
