package cache

import (
	"context"
	"testing"

	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

const gtkUpdateIconCacheCmd = "gtk-update-icon-cache"

func TestNewCacheManager(t *testing.T) {
	cm := NewCacheManager()
	assert.NotNil(t, cm)
	assert.IsType(t, &helpers.OSCommandRunner{}, cm.runner)
}

func TestNewCacheManagerWithRunner(t *testing.T) {
	mockRunner := &helpers.MockCommandRunner{}
	cm := NewCacheManagerWithRunner(mockRunner)
	assert.NotNil(t, cm)
	assert.Equal(t, mockRunner, cm.runner)
}

func TestUpdateIconCache(t *testing.T) {
	mockRunner := &helpers.MockCommandRunner{}
	cm := NewCacheManagerWithRunner(mockRunner)
	log := zerolog.Nop()

	// Test when gtk-update-icon-cache is not found
	mockRunner.CommandExistsFunc = func(_ string) bool {
		return false
	}
	err := cm.UpdateIconCache("/tmp/icons", &log)
	assert.NoError(t, err)

	// Test when gtk-update-icon-cache is found and command succeeds
	mockRunner.CommandExistsFunc = func(name string) bool {
		return name == gtkUpdateIconCacheCmd
	}
	mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", nil
	}
	err = cm.UpdateIconCache("/tmp/icons", &log)
	assert.NoError(t, err)

	// Test when command fails
	mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", assert.AnError
	}
	err = cm.UpdateIconCache("/tmp/icons", &log)
	assert.NoError(t, err)
}

func TestUpdateDesktopDatabase(t *testing.T) {
	mockRunner := &helpers.MockCommandRunner{}
	cm := NewCacheManagerWithRunner(mockRunner)
	log := zerolog.Nop()

	// Test when update-desktop-database is not found
	mockRunner.CommandExistsFunc = func(_ string) bool {
		return false
	}
	err := cm.UpdateDesktopDatabase("/tmp/apps", &log)
	assert.NoError(t, err)

	// Test when update-desktop-database is found and command succeeds
	mockRunner.CommandExistsFunc = func(name string) bool {
		return name == "update-desktop-database"
	}
	mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", nil
	}
	err = cm.UpdateDesktopDatabase("/tmp/apps", &log)
	assert.NoError(t, err)

	// Test when command fails
	mockRunner.RunCommandFunc = func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", assert.AnError
	}
	err = cm.UpdateDesktopDatabase("/tmp/apps", &log)
	assert.NoError(t, err)
}

func TestDetectIconCacheCommand(t *testing.T) {
	mockRunner := &helpers.MockCommandRunner{}
	cm := NewCacheManagerWithRunner(mockRunner)

	// Test when gtk4-update-icon-cache is found
	mockRunner.CommandExistsFunc = func(name string) bool {
		return name == "gtk4-update-icon-cache"
	}
	cmd := cm.detectIconCacheCommand()
	assert.Equal(t, "gtk4-update-icon-cache", cmd)

	// Test when gtk-update-icon-cache is found
	mockRunner.CommandExistsFunc = func(name string) bool {
		return name == gtkUpdateIconCacheCmd
	}
	cmd = cm.detectIconCacheCommand()
	assert.Equal(t, gtkUpdateIconCacheCmd, cmd)

	// Test when neither is found
	mockRunner.CommandExistsFunc = func(_ string) bool {
		return false
	}
	cmd = cm.detectIconCacheCommand()
	assert.Equal(t, "", cmd)
}

func TestNeedsSudo(t *testing.T) {
	mockRunner := &helpers.MockCommandRunner{}
	cm := NewCacheManagerWithRunner(mockRunner)

	// Test when path is in system directories
	assert.True(t, cm.needsSudo("/usr/share/icons"))
	assert.True(t, cm.needsSudo("/opt/myapp"))
	assert.True(t, cm.needsSudo("/var/lib/apps"))
	assert.True(t, cm.needsSudo("/etc/config"))

	// Test when path is not in system directories
	assert.False(t, cm.needsSudo("/home/user/icons"))
	assert.False(t, cm.needsSudo("/tmp/icons"))
}
