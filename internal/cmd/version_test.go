package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVersionCmd(t *testing.T) {
	t.Run("creates version command", func(t *testing.T) {
		t.Parallel()
		cmd := NewVersionCmd("1.2.3")
		assert.NotNil(t, cmd)
		assert.Equal(t, "version", cmd.Use)
		assert.Equal(t, "Show version information", cmd.Short)
	})

	t.Run("command has run function", func(t *testing.T) {
		t.Parallel()
		cmd := NewVersionCmd("1.2.3")
		assert.NotNil(t, cmd.Run)
	})

	t.Run("command executes without error", func(t *testing.T) {
		t.Parallel()
		cmd := NewVersionCmd("1.2.3")
		err := cmd.Execute()
		require.NoError(t, err)
	})
}

func TestVersionCmd_EmptyVersion(t *testing.T) {
	t.Parallel()
	cmd := NewVersionCmd("")
	assert.NotNil(t, cmd)
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestVersionCmd_DevelopmentVersion(t *testing.T) {
	t.Parallel()
	cmd := NewVersionCmd("dev")
	assert.NotNil(t, cmd)
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestVersionCmd_LongVersionString(t *testing.T) {
	t.Parallel()
	longVersion := "1.2.3-456-abc123def"
	cmd := NewVersionCmd(longVersion)
	assert.NotNil(t, cmd)
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestVersionCmd_SemanticVersion(t *testing.T) {
	versions := []string{
		"1.0.0",
		"2.1.3",
		"0.0.1",
		"10.20.30",
	}

	for _, version := range versions {
		t.Run("version_"+version, func(t *testing.T) {
			t.Parallel()
			cmd := NewVersionCmd(version)
			assert.NotNil(t, cmd)
			err := cmd.Execute()
			require.NoError(t, err)
		})
	}
}

func TestVersionCmd_WithPrerelease(t *testing.T) {
	versions := []string{
		"1.0.0-alpha",
		"1.0.0-beta.1",
		"1.0.0-rc.1",
		"2.0.0-preview",
	}

	for _, version := range versions {
		t.Run("version_"+version, func(t *testing.T) {
			t.Parallel()
			cmd := NewVersionCmd(version)
			assert.NotNil(t, cmd)
			err := cmd.Execute()
			require.NoError(t, err)
		})
	}
}

func TestVersionCmd_WithBuildMetadata(t *testing.T) {
	versions := []string{
		"1.0.0+20130313144700",
		"1.0.0-alpha+001",
		"1.0.0-beta+exp.sha.5114f85",
	}

	for _, version := range versions {
		t.Run("version_"+version, func(t *testing.T) {
			t.Parallel()
			cmd := NewVersionCmd(version)
			assert.NotNil(t, cmd)
			err := cmd.Execute()
			require.NoError(t, err)
		})
	}
}

func TestVersionCmd_VariedFormats(t *testing.T) {
	testCases := []struct {
		name    string
		version string
	}{
		{"empty", ""},
		{"single_digit", "1"},
		{"two_parts", "1.2"},
		{"three_parts", "1.2.3"},
		{"with_v_prefix", "v1.2.3"},
		{"git_describe", "1.2.3-0-gabcdef"},
		{"timestamp", "20241228.153000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := NewVersionCmd(tc.version)
			assert.NotNil(t, cmd)
			err := cmd.Execute()
			require.NoError(t, err)
		})
	}
}


