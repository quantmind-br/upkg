package version

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompare_Semver(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b string
		want Result
	}{
		{"patch bump", "1.0.0", "1.0.1", Less},
		{"minor bump", "1.0.0", "1.1.0", Less},
		{"major bump", "1.0.0", "2.0.0", Less},
		{"equal", "1.2.3", "1.2.3", Equal},
		{"classic string trap", "1.9.0", "1.64.0", Less},
		{"prerelease before release", "1.0.0-rc1", "1.0.0", Less},
		{"prerelease order", "1.0.0-alpha", "1.0.0-beta", Less},
		{"prerelease numeric", "1.0.0-rc.1", "1.0.0-rc.2", Less},
		{"v prefix", "v1.0.0", "v1.0.1", Less},
		{"mixed v", "v1.0.0", "1.0.1", Less},
		{"desktop monorepo newer", "desktop-v0.5.4", "desktop-v0.5.5", Less},
		{"schema change still semver", "v0.5.2", "desktop-v0.5.3", Less},
		{"two-segment", "1.9", "1.10", Less},
		{"release- prefix", "release-1.0.0", "release-1.0.1", Less},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, mode := Compare(tc.a, tc.b, time.Time{}, time.Time{})
			assert.Equal(t, ModeSemver, mode, "a=%q b=%q", tc.a, tc.b)
			assert.Equal(t, tc.want, got, "a=%q b=%q", tc.a, tc.b)

			// Symmetry
			rev, revMode := Compare(tc.b, tc.a, time.Time{}, time.Time{})
			assert.Equal(t, ModeSemver, revMode)
			assert.Equal(t, Result(-int(tc.want)), rev)
		})
	}
}

func TestCompare_Date(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b string
		want Result
	}{
		{"dot", "2024.01.15", "2024.02.01", Less},
		{"dash", "2024-01-15", "2024-02-01", Less},
		{"compact", "20240115", "20240201", Less},
		{"equal", "2024.01.15", "2024.01.15", Equal},
		{"cross format", "2024.01.15", "2024-01-16", Less},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, mode := Compare(tc.a, tc.b, time.Time{}, time.Time{})
			assert.Equal(t, ModeDate, mode)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCompare_Natural(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b string
		want Result
	}{
		{"build numbers", "build-9", "build-10", Less},
		{"equal", "rev42", "rev42", Equal},
		{"hex-ish opaque", "r9a", "r10b", Less},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, mode := Compare(tc.a, tc.b, time.Time{}, time.Time{})
			assert.Equal(t, ModeNatural, mode, "a=%q b=%q", tc.a, tc.b)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCompare_EmbeddedSemverViaNaturalPath(t *testing.T) {
	t.Parallel()
	// app-1.9-x normalizes to 1.9-x which parses as semver prerelease.
	got, mode := Compare("app-1.9-x", "app-1.10-x", time.Time{}, time.Time{})
	assert.Equal(t, ModeSemver, mode)
	assert.Equal(t, Less, got)
}

func TestCompare_PublishedFallback(t *testing.T) {
	t.Parallel()

	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	got, mode := Compare("abc", "xyz", older, newer)
	assert.Equal(t, ModePublished, mode)
	assert.Equal(t, Less, got)

	got, mode = Compare("deadbeef", "cafebabe", newer, older)
	assert.Equal(t, ModePublished, mode)
	assert.Equal(t, Greater, got)

	got, mode = Compare("aaa", "bbb", time.Time{}, time.Time{})
	assert.Equal(t, ModePublished, mode)
	assert.Equal(t, Equal, got)
}

func TestCompare_EqualTags(t *testing.T) {
	t.Parallel()

	got, mode := Compare("desktop-v0.5.5", "desktop-v0.5.5", time.Time{}, time.Time{})
	assert.Equal(t, Equal, got)
	assert.Equal(t, ModeSemver, mode)
}

func TestIsNewer(t *testing.T) {
	t.Parallel()

	ok, mode := IsNewer("0.5.4", "0.5.5", time.Time{}, time.Time{})
	assert.True(t, ok)
	assert.Equal(t, ModeSemver, mode)

	ok, mode = IsNewer("0.5.5", "0.5.5", time.Time{}, time.Time{})
	assert.False(t, ok)
	assert.Equal(t, ModeSemver, mode)

	ok, _ = IsNewer("0.5.5", "0.5.4", time.Time{}, time.Time{})
	assert.False(t, ok)
}

func TestNormalizeForDisplay(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0.5.5", NormalizeForDisplay("desktop-v0.5.5"))
	assert.Equal(t, "0.5.2", NormalizeForDisplay("v0.5.2"))
	assert.Equal(t, "1.0.0", NormalizeForDisplay("release-1.0.0"))
	assert.Equal(t, "", NormalizeForDisplay("   "))
}

func TestComponentPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "desktop", ComponentPrefix("desktop-v0.5.5"))
	assert.Equal(t, "", ComponentPrefix("v0.5.2"))
	assert.Equal(t, "sprig", ComponentPrefix("sprig-latest"))
	assert.Equal(t, "", ComponentPrefix(""))
}

func TestResultString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "less", Less.String())
	assert.Equal(t, "equal", Equal.String())
	assert.Equal(t, "greater", Greater.String())
}

func TestNormalizeTag_ReleaseV(t *testing.T) {
	t.Parallel()
	require.Equal(t, "1.2.3", normalizeTag("release-v1.2.3"))
}
