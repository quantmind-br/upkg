package arch

import (
	"context"
	"errors"
	"testing"

	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/syspkg"
	"github.com/stretchr/testify/assert"
)

func TestPacmanProvider_Install(t *testing.T) {
	// Setup
	mockRunner := &helpers.MockCommandRunner{}
	provider := NewPacmanProviderWithRunner(mockRunner)

	// Test case: Successful installation
	t.Run("successful installation", func(t *testing.T) {
		mockRunner.RunCommandFunc = func(_ context.Context, name string, args ...string) (string, error) {
			assert.Equal(t, "sudo", name)
			assert.Equal(t, []string{"pacman", "-U", "--noconfirm", "test.pkg.tar.zst"}, args)
			return "", nil
		}

		err := provider.Install(context.Background(), "test.pkg.tar.zst", nil)
		assert.NoError(t, err)
	})

	// Test case: Successful installation with overwrite
	t.Run("successful installation with overwrite", func(t *testing.T) {
		mockRunner.RunCommandFunc = func(_ context.Context, name string, args ...string) (string, error) {
			assert.Equal(t, "sudo", name)
			assert.Equal(t, []string{"pacman", "-U", "--noconfirm", "--overwrite", "*", "test.pkg.tar.zst"}, args)
			return "", nil
		}

		err := provider.Install(context.Background(), "test.pkg.tar.zst", &syspkg.InstallOptions{Overwrite: true})
		assert.NoError(t, err)
	})

	// Test case: Failed installation
	t.Run("failed installation", func(t *testing.T) {
		expectedErr := errors.New("pacman installation failed")
		mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
			return "", expectedErr
		}

		err := provider.Install(context.Background(), "test.pkg.tar.zst", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pacman installation failed")
	})
}

func TestPacmanProvider_Remove(t *testing.T) {
	// Setup
	mockRunner := &helpers.MockCommandRunner{}
	provider := NewPacmanProviderWithRunner(mockRunner)

	// Test case: Successful removal
	t.Run("successful removal", func(t *testing.T) {
		mockRunner.RunCommandFunc = func(_ context.Context, name string, args ...string) (string, error) {
			assert.Equal(t, "sudo", name)
			assert.Equal(t, []string{"pacman", "-R", "--noconfirm", "test-package"}, args)
			return "", nil
		}

		err := provider.Remove(context.Background(), "test-package")
		assert.NoError(t, err)
	})

	// Test case: Failed removal
	t.Run("failed removal", func(t *testing.T) {
		expectedErr := errors.New("pacman removal failed")
		mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
			return "", expectedErr
		}

		err := provider.Remove(context.Background(), "test-package")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pacman removal failed")
	})
}

func TestPacmanProvider_IsInstalled(t *testing.T) {
	// Setup
	mockRunner := &helpers.MockCommandRunner{}
	provider := NewPacmanProviderWithRunner(mockRunner)

	// Test case: Package is installed
	t.Run("package is installed", func(t *testing.T) {
		mockRunner.RunCommandFunc = func(_ context.Context, name string, args ...string) (string, error) {
			assert.Equal(t, "pacman", name)
			assert.Equal(t, []string{"-Qi", "test-package"}, args)
			return "Name: test-package\nVersion: 1.0.0", nil
		}

		installed, err := provider.IsInstalled(context.Background(), "test-package")
		assert.NoError(t, err)
		assert.True(t, installed)
	})

	// Test case: Package is not installed
	t.Run("package is not installed", func(t *testing.T) {
		mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
			return "", errors.New("package not found")
		}

		installed, err := provider.IsInstalled(context.Background(), "test-package")
		assert.NoError(t, err)
		assert.False(t, installed)
	})
}

func TestPacmanProvider_GetInfo(t *testing.T) {
	// Setup
	mockRunner := &helpers.MockCommandRunner{}
	provider := NewPacmanProviderWithRunner(mockRunner)

	// Test case: Get package info
	t.Run("get package info", func(t *testing.T) {
		mockRunner.RunCommandFunc = func(_ context.Context, name string, args ...string) (string, error) {
			assert.Equal(t, "pacman", name)
			assert.Equal(t, []string{"-Qi", "test-package"}, args)
			return "Name: test-package\nVersion: 1.0.0\nDescription: Test package", nil
		}

		info, err := provider.GetInfo(context.Background(), "test-package")
		assert.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "test-package", info.Name)
		assert.Equal(t, "1.0.0", info.Version)
	})

	// Test case: Package not found
	t.Run("package not found", func(t *testing.T) {
		expectedErr := errors.New("package not found")
		mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
			return "", expectedErr
		}

		info, err := provider.GetInfo(context.Background(), "test-package")
		assert.Error(t, err)
		assert.Nil(t, info)
	})
}

func TestNewPacmanProvider(t *testing.T) {
	t.Run("creates provider with OS runner", func(t *testing.T) {
		provider := NewPacmanProvider()
		assert.NotNil(t, provider)
		assert.NotNil(t, provider.runner)
	})
}

func TestPacmanProvider_Name(t *testing.T) {
	t.Run("returns correct name", func(t *testing.T) {
		provider := NewPacmanProvider()
		assert.Equal(t, "pacman", provider.Name())
	})

	t.Run("returns correct name with custom runner", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{}
		provider := NewPacmanProviderWithRunner(mockRunner)
		assert.Equal(t, "pacman", provider.Name())
	})
}

func TestPacmanProvider_ListFiles(t *testing.T) {
	// Setup
	mockRunner := &helpers.MockCommandRunner{}
	provider := NewPacmanProviderWithRunner(mockRunner)

	// Test case: List package files
	t.Run("list package files", func(t *testing.T) {
		mockRunner.RunCommandFunc = func(_ context.Context, name string, args ...string) (string, error) {
			assert.Equal(t, "pacman", name)
			assert.Equal(t, []string{"-Ql", "test-package"}, args)
			return "test-package /usr/bin/test\ntest-package /usr/share/test/data", nil
		}

		files, err := provider.ListFiles(context.Background(), "test-package")
		assert.NoError(t, err)
		assert.Equal(t, []string{"/usr/bin/test", "/usr/share/test/data"}, files)
	})

	// Test case: Package not found
	t.Run("package not found", func(t *testing.T) {
		expectedErr := errors.New("package not found")
		mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
			return "", expectedErr
		}

		files, err := provider.ListFiles(context.Background(), "test-package")
		assert.Error(t, err)
		assert.Nil(t, files)
	})
}
