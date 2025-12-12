package paths

import (
	"os"
	"path/filepath"

	"github.com/quantmind-br/upkg/internal/config"
)

// Resolver centraliza caminhos padrão do upkg.
// Ele calcula diretórios base a partir de HOME e da configuração.
type Resolver struct {
	homeDir string
	cfg     *config.Config
}

// NewResolver cria um Resolver usando o HOME do usuário atual.
func NewResolver(cfg *config.Config) *Resolver {
	homeDir, _ := os.UserHomeDir()
	return &Resolver{
		homeDir: homeDir,
		cfg:     cfg,
	}
}

// NewResolverWithHome cria um Resolver com homeDir explícito (útil para testes).
func NewResolverWithHome(cfg *config.Config, homeDir string) *Resolver {
	return &Resolver{
		homeDir: homeDir,
		cfg:     cfg,
	}
}

// HomeDir retorna o diretório HOME resolvido.
func (r *Resolver) HomeDir() string {
	return r.homeDir
}

// GetBinDir retorna ~/.local/bin.
func (r *Resolver) GetBinDir() string {
	return filepath.Join(r.homeDir, ".local", "bin")
}

// GetAppsDir retorna ~/.local/share/applications.
func (r *Resolver) GetAppsDir() string {
	return filepath.Join(r.homeDir, ".local", "share", "applications")
}

// GetIconsDir retorna ~/.local/share/icons/hicolor.
func (r *Resolver) GetIconsDir() string {
	return filepath.Join(r.homeDir, ".local", "share", "icons", "hicolor")
}

// GetUpkgAppsDir retorna o diretório de apps gerenciados pelo upkg.
// Por padrão: ~/.local/share/upkg/apps, respeitando cfg.Paths.DataDir se definido.
func (r *Resolver) GetUpkgAppsDir() string {
	base := ""
	if r.cfg != nil {
		base = r.cfg.Paths.DataDir
	}
	if base == "" {
		base = filepath.Join(r.homeDir, ".local", "share", "upkg")
	}
	return filepath.Join(base, "apps")
}

// GetIconSizeDir retorna ~/.local/share/icons/hicolor/{size}/apps.
func (r *Resolver) GetIconSizeDir(size string) string {
	return filepath.Join(r.GetIconsDir(), size, "apps")
}
