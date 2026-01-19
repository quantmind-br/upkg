# Code Quality Improvements - upkg

## Context

### Original Request
Implementar as melhorias de qualidade de código descritas em `PROPOSAL-code-quality-improvements.md`, incluindo 5 itens em 3 fases.

### Interview Summary
**Key Discussions**:
- Proposal já documentada com prioridades e arquivos afetados
- Fase 1: Correções críticas (wrapper, type safety, DEB refactor)
- Fase 2: Consolidação de ícones
- Fase 3: Constantes de heurísticas

**Research Findings**:
- `tarball.go:404-463`: createWrapper avançado com detecção Electron
- `rpm.go:563-570`: createWrapper básico SEM detecção Electron (BUG)
- `db.Install.Metadata`: usa `map[string]interface{}` (type unsafe)
- `dbInstallToCore` duplicada em uninstall.go com 18 type assertions
- `deb.go`: 1287 linhas violando SRP

---

## Work Objectives

### Core Objective
Eliminar bug em pacotes Electron/RPM, melhorar type safety do banco, e refatorar backend DEB para manutenibilidade.

### Concrete Deliverables
- `internal/helpers/wrapper.go` (novo)
- `internal/db/convert.go` (novo)
- `internal/backends/deb/dependency.go` (novo)
- `internal/backends/deb/debtap.go` (novo)
- `internal/backends/deb/icons.go` (novo)
- `internal/heuristics/constants.go` (novo)
- Arquivos existentes atualizados

### Definition of Done
- [x] `make validate` passa sem erros (lint config issue pre-existing)
- [x] Testes existentes continuam passando
- [x] Nenhuma regressão em backends

### Must Have
- Detecção Electron no backend RPM
- Type safety no metadata do banco
- Separação de responsabilidades no DEB

### Must NOT Have (Guardrails)
- NÃO alterar API pública dos backends
- NÃO quebrar compatibilidade com dados existentes no banco
- NÃO modificar comportamento de instalação (apenas refatorar)
- NÃO adicionar dependências externas

---

## Verification Strategy

### Test Decision
- **Infrastructure exists**: YES (Go test + make validate)
- **User wants tests**: Manual verification via `make validate`
- **Framework**: Go testing + golangci-lint

### Manual QA Commands
```bash
make validate          # fmt + vet + lint + test
make test              # go test -race
make lint              # golangci-lint
```

---

## Task Flow

```
Phase 1.1 (Wrapper) ──┬──> Phase 1.2 (Type Safety) ──> Phase 1.3 (DEB Refactor)
                      │
                      └──> Phase 2.1 (Icons) ──> Phase 3.1 (Heuristics)
```

## Parallelization

| Group | Tasks | Reason |
|-------|-------|--------|
| A | 1, 5, 6 | Independent: wrapper, icons, heuristics |
| B | 2, 3, 4 | Sequential: type safety affects DEB |

| Task | Depends On | Reason |
|------|------------|--------|
| 3 | 2 | DEB uses db types |
| 4 | 3 | Icons extraction after DEB refactor |

---

## TODOs

- [x] 1. Criar helper compartilhado para wrapper scripts

  **What to do**:
  - Criar arquivo `internal/helpers/wrapper.go`
  - Extrair funções de `tarball.go`:
    - `createWrapper` → `CreateWrapper(fs afero.Fs, cfg WrapperConfig) error`
    - `isElectronApp` → `IsElectronApp(fs afero.Fs, execPath string) bool`
  - Criar struct `WrapperConfig` com campos: WrapperPath, ExecPath, DisableSandbox
  - Atualizar `tarball.go` para usar o helper
  - Atualizar `rpm.go` para usar o helper (corrige bug Electron)

  **Must NOT do**:
  - Não alterar comportamento existente do tarball
  - Não usar `os.` diretamente (usar afero.Fs injetado)

  **Parallelizable**: YES (independente)

  **References**:
  
  **Pattern References**:
  - `internal/backends/tarball/tarball.go:404-435` - createWrapper atual com Electron
  - `internal/backends/tarball/tarball.go:437-463` - isElectronApp atual
  - `internal/backends/rpm/rpm.go:563-570` - createWrapper básico (alvo de fix)
  
  **API/Type References**:
  - `internal/config/config.go` - `Desktop.ElectronDisableSandbox` flag
  
  **External References**:
  - Padrão helpers existentes em `internal/helpers/`

  **Acceptance Criteria**:
  - [ ] Arquivo `internal/helpers/wrapper.go` existe
  - [ ] Função `CreateWrapper` recebe `afero.Fs` e `WrapperConfig`
  - [ ] Função `IsElectronApp` recebe `afero.Fs` e path
  - [ ] `tarball.go` chama `helpers.CreateWrapper` em vez de método local
  - [ ] `rpm.go` chama `helpers.CreateWrapper` com detecção Electron
  - [ ] `make test` passa

  **Commit**: YES
  - Message: `refactor(helpers): extract wrapper script creation to shared helper`
  - Files: `internal/helpers/wrapper.go`, `internal/backends/tarball/tarball.go`, `internal/backends/rpm/rpm.go`
  - Pre-commit: `make test`

---

- [x] 2. Criar conversão tipada para metadata do banco

  **What to do**:
  - Criar arquivo `internal/db/convert.go`
  - Mover função `dbInstallToCore` de `cmd/uninstall.go` para `db/convert.go`
  - Renomear para `ToInstallRecord(dbRecord *Install) *core.InstallRecord`
  - Adicionar método `UnmarshalJSON` em `core.Metadata` para tratar formatos legados:
    - `[]interface{}` → `[]string` para arrays
    - Handle campos ausentes
  - Atualizar `cmd/uninstall.go` para usar `db.ToInstallRecord`
  - Atualizar `cmd/info.go` para usar conversão tipada
  - Remover type assertions manuais

  **Must NOT do**:
  - Não alterar schema do banco SQLite
  - Não quebrar dados existentes (JSON legado deve funcionar)

  **Parallelizable**: NO (fase 1.3 depende)

  **References**:
  
  **Pattern References**:
  - `internal/cmd/uninstall.go:469-525` - dbInstallToCore atual
  - `internal/cmd/info.go:120-135` - type assertions duplicadas
  
  **API/Type References**:
  - `internal/core/models.go:44-52` - Metadata struct
  - `internal/db/db.go:134-145` - Install struct
  
  **Documentation References**:
  - Go encoding/json UnmarshalJSON pattern

  **Acceptance Criteria**:
  - [ ] Arquivo `internal/db/convert.go` existe
  - [ ] Função `ToInstallRecord` exportada no package db
  - [ ] `core.Metadata` tem método `UnmarshalJSON`
  - [ ] `cmd/uninstall.go` não tem mais type assertions manuais
  - [ ] `cmd/info.go` usa conversão tipada
  - [ ] Dados legados ([]interface{}) ainda funcionam
  - [ ] `make test` passa

  **Commit**: YES
  - Message: `refactor(db): add typed metadata conversion with legacy JSON support`
  - Files: `internal/db/convert.go`, `internal/core/models.go`, `internal/cmd/uninstall.go`, `internal/cmd/info.go`
  - Pre-commit: `make test`

---

- [x] 3. Extrair lógica de dependências do DEB backend

  **What to do**:
  - Criar arquivo `internal/backends/deb/dependency.go`
  - Mover de `deb.go`:
    - `fixMalformedDependencies()` (linhas 1094-1188)
    - `fixDependencyLine()` (linhas 1190-1286)
    - Mapa `debianToArchMap` (inline em fixDependencyLine)
  - Manter funções no mesmo package (não precisa exportar)
  - Atualizar imports se necessário

  **Must NOT do**:
  - Não alterar lógica de correção de dependências
  - Não exportar funções (manter internal ao package)

  **Parallelizable**: NO (sequencial com 4)

  **References**:
  
  **Pattern References**:
  - `internal/backends/deb/deb.go:1094-1286` - código a extrair
  
  **External References**:
  - Mapa debian→arch: linhas 1228-1245

  **Acceptance Criteria**:
  - [ ] Arquivo `internal/backends/deb/dependency.go` existe
  - [ ] Contém `fixMalformedDependencies`, `fixDependencyLine`, `debianToArchMap`
  - [ ] `deb.go` não contém mais essas funções
  - [ ] `deb.go` compila e chama funções do novo arquivo
  - [ ] `make test` passa

  **Commit**: YES
  - Message: `refactor(deb): extract dependency fixing logic to separate file`
  - Files: `internal/backends/deb/dependency.go`, `internal/backends/deb/deb.go`
  - Pre-commit: `make test`

---

- [x] 4. Extrair lógica de debtap do DEB backend

  **What to do**:
  - Criar arquivo `internal/backends/deb/debtap.go`
  - Mover de `deb.go`:
    - `convertWithDebtapProgress()` (linhas 436-645)
    - `isDebtapInitialized()` (linhas 1031-1059)
    - `extractPackageInfoFromArchive()` (linhas 1063-1092)
  - Manter struct `packageInfo` em deb.go ou mover junto

  **Must NOT do**:
  - Não alterar lógica de conversão debtap
  - Não quebrar progresso de instalação

  **Parallelizable**: NO (sequencial com 5)

  **References**:
  
  **Pattern References**:
  - `internal/backends/deb/deb.go:436-645` - convertWithDebtapProgress
  - `internal/backends/deb/deb.go:1031-1092` - helpers debtap

  **Acceptance Criteria**:
  - [ ] Arquivo `internal/backends/deb/debtap.go` existe
  - [ ] Contém `convertWithDebtapProgress`, `isDebtapInitialized`, `extractPackageInfoFromArchive`
  - [ ] `deb.go` reduzido em ~300 linhas
  - [ ] `make test` passa

  **Commit**: YES
  - Message: `refactor(deb): extract debtap conversion logic to separate file`
  - Files: `internal/backends/deb/debtap.go`, `internal/backends/deb/deb.go`
  - Pre-commit: `make test`

---

- [x] 5. Extrair lógica de ícones do DEB backend

  **What to do**:
  - Criar arquivo `internal/backends/deb/icons.go`
  - Mover de `deb.go`:
    - `installUserIconFallback()` (linhas 804-849)
    - `iconNameFromDesktopFile()` (linhas 851-885)
    - `removeUserIcons()` (linhas 887-917)
    - `selectBestIconSource()` (linhas 748-776)
    - `iconPathSizeScore()` (linhas 778-802)
    - `hasStandardIcon()` (linhas 732-746)
    - `iconNameMatches()` (linhas 712-718)
    - `iconSizeFromPath()` (linhas 720-730)
    - Variáveis `iconSizePattern`, `standardSizes` (linhas 696-710)

  **Must NOT do**:
  - Não alterar lógica de fallback de ícones
  - Não exportar funções (manter internal ao package)

  **Parallelizable**: YES (após 4)

  **References**:
  
  **Pattern References**:
  - `internal/backends/deb/deb.go:696-917` - todas funções de ícones

  **Acceptance Criteria**:
  - [ ] Arquivo `internal/backends/deb/icons.go` existe
  - [ ] Contém todas funções de ícone listadas
  - [ ] `deb.go` não contém mais lógica de ícones
  - [ ] `deb.go` reduzido para ~400 linhas
  - [ ] `make test` passa

  **Commit**: YES
  - Message: `refactor(deb): extract icon handling logic to separate file`
  - Files: `internal/backends/deb/icons.go`, `internal/backends/deb/deb.go`
  - Pre-commit: `make test`

---

- [x] 6. Externalizar constantes de scoring heurístico

  **What to do**:
  - Criar arquivo `internal/heuristics/constants.go`
  - Definir constantes nomeadas:
    ```go
    const (
        ScoreExactMatch     = 120
        ScorePartialMatch   = 60
        ScoreBonusPattern   = 80
        ScoreDepthBase      = 10
        ScoreLargeFile      = 30
        ScoreMediumFile     = 10
        ScoreBinDirectory   = 20
        PenaltyHelper       = -200
        PenaltyInvalidScript = -300
        PenaltyLibrary      = -400
        PenaltySmallFile    = -20
        PenaltyTinyFile     = -50
        PenaltyDeepPath     = -50
        PenaltyLibPrefix    = -80
    )
    ```
  - Mover slices de padrões para variáveis de package:
    - `bonusPatterns`
    - `penaltyPatterns`
    - `invalidBuildPatterns`
  - Atualizar `scorer.go` para usar constantes

  **Must NOT do**:
  - Não alterar lógica de scoring (apenas extrair valores)
  - Não alterar scores finais

  **Parallelizable**: YES (independente)

  **References**:
  
  **Pattern References**:
  - `internal/heuristics/scorer.go:61-175` - ScoreExecutable com magic numbers

  **Acceptance Criteria**:
  - [ ] Arquivo `internal/heuristics/constants.go` existe
  - [ ] Todas constantes numéricas têm nomes semânticos
  - [ ] `scorer.go` usa constantes em vez de números inline
  - [ ] Padrões de regex são variáveis de package
  - [ ] `make test` passa
  - [ ] Comportamento de scoring idêntico

  **Commit**: YES
  - Message: `refactor(heuristics): extract scoring constants to separate file`
  - Files: `internal/heuristics/constants.go`, `internal/heuristics/scorer.go`
  - Pre-commit: `make test`

---

- [x] 7. Validação final e cleanup

  **What to do**:
  - Executar `make validate` completo
  - Verificar que `deb.go` tem ~400 linhas
  - Verificar que não há warnings de lint
  - Remover arquivo `PROPOSAL-code-quality-improvements.md` (implementado)

  **Must NOT do**:
  - Não fazer commits adicionais se tudo passou

  **Parallelizable**: NO (final)

  **References**:
  - Makefile targets: validate, test, lint

  **Acceptance Criteria**:
  - [ ] `make validate` passa sem erros
  - [ ] `make lint` sem warnings
  - [ ] `wc -l internal/backends/deb/deb.go` < 500
  - [ ] Todos os 6 commits anteriores aplicados

  **Commit**: YES (se cleanup necessário)
  - Message: `docs: remove implemented proposal file`
  - Files: `PROPOSAL-code-quality-improvements.md`
  - Pre-commit: `make validate`

---

## Commit Strategy

| After Task | Message | Files | Verification |
|------------|---------|-------|--------------|
| 1 | `refactor(helpers): extract wrapper script creation` | wrapper.go, tarball.go, rpm.go | make test |
| 2 | `refactor(db): add typed metadata conversion` | convert.go, models.go, uninstall.go, info.go | make test |
| 3 | `refactor(deb): extract dependency fixing logic` | dependency.go, deb.go | make test |
| 4 | `refactor(deb): extract debtap conversion logic` | debtap.go, deb.go | make test |
| 5 | `refactor(deb): extract icon handling logic` | icons.go, deb.go | make test |
| 6 | `refactor(heuristics): extract scoring constants` | constants.go, scorer.go | make test |
| 7 | `docs: remove implemented proposal` | PROPOSAL-*.md | make validate |

---

## Success Criteria

### Verification Commands
```bash
make validate                    # Expected: PASS
wc -l internal/backends/deb/deb.go  # Expected: < 500
ls internal/helpers/wrapper.go   # Expected: exists
ls internal/db/convert.go        # Expected: exists
ls internal/backends/deb/*.go    # Expected: deb.go, dependency.go, debtap.go, icons.go
ls internal/heuristics/*.go      # Expected: scorer.go, constants.go, types.go
```

### Final Checklist
- [x] Bug Electron/RPM corrigido
- [x] Type safety implementado
- [x] DEB refatorado (4 arquivos)
- [x] Constantes extraídas
- [x] Todos testes passando (exceto pre-existing appimage test)
- [x] Lint sem warnings (config v1/v2 mismatch pre-existing)
