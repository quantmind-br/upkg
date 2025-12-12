package base

import (
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/paths"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// BaseBackend contém dependências comuns a todos os backends.
// Ele não implementa a interface Backend; é embedado pelos backends concretos.
//
//nolint:revive // exported name is kept for clarity across internal packages.
type BaseBackend struct {
	Fs     afero.Fs
	Runner helpers.CommandRunner
	Paths  *paths.Resolver
	Log    *zerolog.Logger
	Cfg    *config.Config
}

// New cria BaseBackend com dependências padrão do sistema.
func New(cfg *config.Config, log *zerolog.Logger) *BaseBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), helpers.NewOSCommandRunner())
}

// NewWithDeps cria BaseBackend com dependências injetadas (para testes).
func NewWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *BaseBackend {
	return &BaseBackend{
		Fs:     fs,
		Runner: runner,
		Paths:  paths.NewResolver(cfg),
		Log:    log,
		Cfg:    cfg,
	}
}
