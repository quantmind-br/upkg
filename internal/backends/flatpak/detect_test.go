package flatpak

import (
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		setup    func(fs afero.Fs)
		expected bool
		wantErr  bool
	}{
		{
			name:  "detects flatpak bundle with ZIP magic",
			input: "/tmp/app.flatpak",
			setup: func(fs afero.Fs) {
				content := []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00}
				err := afero.WriteFile(fs, "/tmp/app.flatpak", content, 0644)
				require.NoError(t, err)
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:  "detects flatpakref with INI header",
			input: "/tmp/firefox.flatpakref",
			setup: func(fs afero.Fs) {
				content := []byte("[Flatpak Ref]\nName=org.mozilla.firefox\nUrl=https://flathub.org/repo/\n")
				err := afero.WriteFile(fs, "/tmp/firefox.flatpakref", content, 0644)
				require.NoError(t, err)
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:     "detects App ID with three segments",
			input:    "org.mozilla.firefox",
			setup:    nil,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "detects App ID with four segments",
			input:    "com.spotify.Client",
			setup:    nil,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "detects App ID with many segments",
			input:    "org.gnome.Builder.Devel",
			setup:    nil,
			expected: true,
			wantErr:  false,
		},
		{
			name:  "rejects regular ELF file",
			input: "/usr/bin/firefox",
			setup: func(fs afero.Fs) {
				content := []byte{0x7F, 0x45, 0x4C, 0x46, 0x02, 0x01, 0x01, 0x00}
				err := afero.WriteFile(fs, "/usr/bin/firefox", content, 0644)
				require.NoError(t, err)
			},
			expected: false,
			wantErr:  false,
		},
		{
			name:  "rejects fake flatpak with wrong magic",
			input: "/tmp/fake.flatpak",
			setup: func(fs afero.Fs) {
				content := []byte{0x00, 0x00, 0x00, 0x00, 0x46, 0x41, 0x4B, 0x45}
				err := afero.WriteFile(fs, "/tmp/fake.flatpak", content, 0644)
				require.NoError(t, err)
			},
			expected: false,
			wantErr:  false,
		},
		{
			name:  "rejects fake flatpakref without INI header",
			input: "/tmp/fake.flatpakref",
			setup: func(fs afero.Fs) {
				content := []byte("This is not a flatpakref file\n")
				err := afero.WriteFile(fs, "/tmp/fake.flatpakref", content, 0644)
				require.NoError(t, err)
			},
			expected: false,
			wantErr:  false,
		},
		{
			name:     "rejects path string (not App ID)",
			input:    "/usr/bin/firefox",
			setup:    nil,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "rejects App ID with only two segments",
			input:    "org.mozilla",
			setup:    nil,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "rejects App ID with invalid characters",
			input:    "org.mozilla.fire-fox",
			setup:    nil,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "rejects App ID starting with number",
			input:    "1org.mozilla.firefox",
			setup:    nil,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "rejects non-existent file",
			input:    "/tmp/nonexistent.flatpak",
			setup:    nil,
			expected: false,
			wantErr:  false,
		},
		{
			name:  "rejects empty file with .flatpak extension",
			input: "/tmp/empty.flatpak",
			setup: func(fs afero.Fs) {
				err := afero.WriteFile(fs, "/tmp/empty.flatpak", []byte{}, 0644)
				require.NoError(t, err)
			},
			expected: false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if tt.setup != nil {
				tt.setup(fs)
			}

			got, err := Detect(context.Background(), fs, tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, got, "Detect() returned unexpected result")
		})
	}
}
