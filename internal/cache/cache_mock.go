package cache

import "github.com/rs/zerolog"

// MockCacheManager is a mock implementation of CacheManager for testing
type MockCacheManager struct {
	UpdateIconCacheFunc      func(iconDir string, log *zerolog.Logger) error
	UpdateDesktopDatabaseFunc func(appsDir string, log *zerolog.Logger) error
}

// UpdateIconCache implements CacheManager.UpdateIconCache
func (m *MockCacheManager) UpdateIconCache(iconDir string, log *zerolog.Logger) error {
	if m.UpdateIconCacheFunc != nil {
		return m.UpdateIconCacheFunc(iconDir, log)
	}
	return nil
}

// UpdateDesktopDatabase implements CacheManager.UpdateDesktopDatabase
func (m *MockCacheManager) UpdateDesktopDatabase(appsDir string, log *zerolog.Logger) error {
	if m.UpdateDesktopDatabaseFunc != nil {
		return m.UpdateDesktopDatabaseFunc(appsDir, log)
	}
	return nil
}
