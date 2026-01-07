# Plano de Cobertura de Testes - upkg

## Status Atual

### Cobertura Geral
- **Cobertura atual**: 72.3%
- **Meta**: 90%
- **Gap**: 17.7%

### Cobertura por Pacote

| Pacote | Cobertura Atual | Meta | Gap | Prioridade |
|--------|----------------|------|-----|------------|
| cmd/upkg | 0.0% | 90% | 90% | **CRÍTICA** |
| internal/ui | 62.9% | 90% | 27.1% | **ALTA** |
| internal/backends/tarball | 57.6% | 90% | 32.4% | **ALTA** |
| internal/backends/appimage | 58.0% | 90% | 32.0% | **ALTA** |
| internal/backends/rpm | 62.1% | 90% | 27.9% | **ALTA** |
| internal/backends/deb | 67.3% | 90% | 22.7% | **ALTA** |
| internal/cmd | 72.8% | 90% | 17.2% | MÉDIA |
| internal/cache | 80.0% | 90% | 10.0% | MÉDIA |
| internal/backends | 80.5% | 90% | 9.5% | MÉDIA |
| internal/backends/binary | 80.8% | 90% | 9.2% | MÉDIA |
| internal/icons | 80.9% | 90% | 9.1% | MÉDIA |
| internal/db | 80.4% | 90% | 9.6% | MÉDIA |
| internal/helpers | 80.4% | 90% | 9.6% | MÉDIA |
| internal/desktop | 87.9% | 90% | 2.1% | BAIXA |
| internal/heuristics | 88.9% | 90% | 1.1% | BAIXA |
| internal/logging | 88.6% | 90% | 1.4% | BAIXA |
| internal/security | 89.7% | 90% | 0.3% | BAIXA |
| internal/config | 90.2% | 90% | 0% | ✓ ATINGIDO |
| internal/hyprland | 84.6% | 90% | 5.4% | BAIXA |
| internal/paths | 93.8% | 90% | 0% | ✓ ATINGIDO |
| internal/syspkg/arch | 95.0% | 90% | 0% | ✓ ATINGIDO |
| internal/core | 100.0% | 90% | 0% | ✓ ATINGIDO |
| internal/transaction | 100.0% | 90% | 0% | ✓ ATINGIDO |

## Análise Detalhada

### Arquivos Sem Testes

#### 1. `cmd/upkg/main.go` (0% cobertura)
- **Status**: CRÍTICO - Ponto de entrada da aplicação
- **Impacto**: Alto - Sem testes de integração do CLI
- **Ação**: Criar testes de integração para o fluxo principal

#### 2. `internal/syspkg/provider.go` (sem testes)
- **Status**: Interface apenas - implementações possuem testes
- **Impacto**: Baixo - É uma interface
- **Ação**: Documentar que testes estão nas implementações

### Funções com 0% de Cobertura

#### `internal/ui/prompt.go` (Funções não testadas)
- `ConfirmPrompt` (0%)
- `InputPrompt` (0%)
- `SelectPrompt` (0%)
- `SelectPromptDetailed` (0%)
- `MultiSelectPrompt` (18.2%)
- `MultiSelectPromptLegacy` (0%)
- `PasswordPrompt` (0%)
- `ConfirmDangerousAction` (0%)
- `ConfirmWithDefault` (0%)

**Desafio**: Funções interativas que dependem de `lipgloss`

#### `internal/syspkg/arch/pacman.go`
- `NewPacmanProvider` (0%)
- `Name` (0%)

**Nota**: Funções simples de construtor, impacto baixo

#### `internal/cmd/install.go`
- `fixDockIcon` (0%)

#### `internal/cmd/uninstall.go`
- `runInteractiveUninstall` (0%)

#### `internal/desktop/desktop.go`
- `escapeGenericToken` (0%)

#### `internal/cache/cache_mock.go`
- Arquivo mock - não requer testes específicos

## Estratégia de Implementação

### Fase 1: Quick Wins (Semana 1-2)

#### 1.1 Arquivos com Alta Cobertura (80%+)
Estes arquivos estão próximos de 90% e requerem pouco esforço:

- [ ] **internal/config/config.go** (90.2% → 95%+)
  - Funções a testar: nenhuma crítica
  - Estimativa: +2-3% cobertura

- [ ] **internal/paths/paths.go** (93.8% → 95%+)
  - Funções a testar: edge cases em `NewResolver`
  - Estimativa: +2% cobertura

- [ ] **internal/syspkg/arch/pacman.go** (95% → 100%)
  - Funções a testar: `NewPacmanProvider`, `Name`
  - Estimativa: +5% cobertura

- [ ] **internal/desktop/desktop.go** (87.9% → 90%)
  - Funções a testar: `escapeGenericToken`
  - Estimativa: +2.1% cobertura

- [ ] **internal/security/validation.go** (89.7% → 95%+)
  - Cobrir branches remanescentes
  - Estimativa: +5% cobertura

#### 1.2 Testes de Construtores Simples
- [ ] **internal/syspkg/arch/pacman.go**
  - Testar `NewPacmanProvider` e `Name`
  - Estimativa: 30 minutos

#### 1.3 Testar Funções Utilitárias Não Cobertas
- [ ] **internal/desktop/desktop.go**: `escapeGenericToken`
- [ ] **internal/cmd/install.go**: `fixDockIcon` (se aplicável)
- [ ] **internal/icons/icons.go**: funções com 66-73%

**Impacto esperado da Fase 1**: +5-7% na cobertura global

### Fase 2: Código Crítico - Backends (Semana 3-5)

#### 2.1 Backend Tarball (57.6% → 90%)
- [ ] **internal/backends/tarball/tarball.go**
  - `Install` (49% → 90%): testar caminhos de erro
  - `extractIconsFromAsarNative` (12.3%): testar extração ASAR
  - `extractIconsFromAsar` (67.3%): branches não cobertas
  - `installIcons` (73.7%): edge cases
  - Cenários: arquivos corrompidos, permissões, paths inválidos

**Estratégia**:
- Usar `afero` para mocking de filesystem
- Criar fixtures de arquivos tarball válidos e inválidos
- Testar detecção de Electron apps
- Testar criação de wrapper scripts

#### 2.2 Backend AppImage (58% → 90%)
- [ ] **internal/backends/appimage/appimage.go**
  - `Install` (13.5% → 90%): principal lacuna
  - `extractAppImage` (68.8%): error paths
  - Cenários: AppImages válidas/inválidas, sem ícones, arquiteturas diferentes

**Estratégia**:
- Mock de execução de comandos (`unsquashfs`)
- Fixtures de AppImages de teste
- Testar integração com hyprland

#### 2.3 Backend RPM (62.1% → 90%)
- [ ] **internal/backends/rpm/rpm.go**
  - `installWithDebtap` (26.4%): fluxo de fallback
  - `installWithExtract` (30%): estratégia principal
  - `copyDir` (68.1%): edge cases
  - Cenários: RPMs com/sem debtap, dependências, pós-instalação

**Estratégia**:
- Testar ambos os estratégias (extract vs debtap)
- Mock de comandos RPM e pacman
- Testar criação de desktop files e wrappers

#### 2.4 Backend DEB (67.3% → 90%)
- [ ] **internal/backends/deb/deb.go**
  - `Install` (28.6% → 90%): fluxo principal
  - `convertWithDebtapProgress` (45.9%): progress reporting
  - Cenários: pacotes válidos, conversão debtap, pós-instalação

**Estratégia**:
- Mock de debtap e pacman
- Testar mapeamento de dependências
- Testar atualização de desktop files

**Impacto esperado da Fase 2**: +8-10% na cobertura global

### Fase 3: Camada de UI e Interação (Semana 6-7)

#### 3.1 Prompt Functions (62.9% → 90%)
- [ ] **internal/ui/prompt.go**
  - Todas as funções com 0% de cobertura
  - **Desafio**: Funções interativas dependem de `lipgloss`

**Estratégia**:
- Criar interface `PromptReader` para injeção de dependências
- Mock de input do usuário
- Testar validação de respostas
- Testar prompts em modo não-interativo

#### 3.2 Progress Tracking
- [ ] **internal/ui/progress.go**
  - Funções com baixa cobertura: testar edge cases

**Estratégia**:
- Testar updates de progresso
- Testar finalização e cleanup

#### 3.3 Commands Layer
- [ ] **internal/cmd/**
  - `install.go` (63.2%): fluxo de instalação
  - `uninstall.go` (48.6% em executeUninstall): caminhos de erro
  - `completion.go` (55.6%): testar geração de scripts

**Impacto esperado da Fase 3**: +3-5% na cobertura global

### Fase 4: Integração e Edge Cases (Semana 8)

#### 4.1 CLI Main Entry Point
- [ ] **cmd/upkg/main.go**
  - Criar testes de integração end-to-end
  - Testar subcomandos principais

**Estratégia**:
- Usar `exec.Command` para executar binário compilado
- Testar fluxos: install, uninstall, list, info, doctor
- Usar pacotes de teste temporários

#### 4.2 Helpers com Baixa Cobertura
- [ ] **internal/helpers/archive.go**
  - `ExtractTarXz` (25%)
  - `ExtractTarBz2` (30%)
  - `extractTar` (46.4%)
  - `extractFile` (66.7%)

- [ ] **internal/icons/icons.go**
  - `getImageDimensions` (58.8%)
  - `createIconThemeSection` (66.7%)
  - `parseSquareSize` (66.7%)
  - `copyIcon` (66.7%)

- [ ] **internal/db/db.go**
  - `applyMigrations` (66.7%): testar migrações

**Impacto esperado da Fase 4**: +2-3% na cobertura global

## Checklist de Testes por Arquivo

### internal/backends/tarball/tarball.go
**Cobertura atual**: 57.6%

**Funções prioritárias**:
- [ ] `Install` (49%)
  - [ ] Testar detecção de Electron app
  - [ ] Testar extração de archive
  - [ ] Testar criação de wrapper
  - [ ] Testar criação de desktop file
  - [ ] Testar instalação de ícones
  - [ ] Testar casos de erro: arquivo inválido, sem executável, permissões negadas

- [ ] `extractIconsFromAsarNative` (12.3%)
  - [ ] Testar extração bem-sucedida de ASAR
  - [ ] Testar ASAR corrompido
  - [ ] Testar comando asar não disponível

- [ ] `extractIconsFromAsar` (67.3%)
  - [ ] Branches não cobertas: diferentes estruturas de ASAR
  - [ ] Testar fallback para extração nativa

- [ ] `isElectronApp` (93.3%)
  - [ ] Quase completo, apenas edge cases

- [ ] `createWrapper` (100%)
  - [ ] Já coberto

**Mocks necessários**:
- `afero.Fs` para filesystem
- `helpers.OSCommandRunner` para comandos externos
- `icons.Manager` para instalação de ícones

### internal/backends/appimage/appimage.go
**Cobertura atual**: 58.0%

**Funções prioritárias**:
- [ ] `Install` (13.5%)
  - [ ] Testar extração de AppImage
  - [ ] Testar criação de wrapper
  - [ ] Testar instalação de ícones
  - [ ] Testar integração Hyprland
  - [ ] Testar casos de erro: AppImage inválido, sem ícone

- [ ] `extractAppImage` (68.8%)
  - [ ] Testar caminhos de erro no unsquashfs
  - [ ] Testar permissões

- [ ] `Detect` (77.8%)
  - [ ] Branches remanescentes

**Mocks necessários**:
- `afero.Fs` para filesystem
- `helpers.OSCommandRunner` para unsquashfs
- `icons.Manager`
- `hyprland.Client`

### internal/backends/rpm/rpm.go
**Cobertura atual**: 62.1%

**Funções prioritárias**:
- [ ] `Install` (80%)
  - [ ] Testar escolha de estratégia (extract vs debtap)

- [ ] `installWithExtract` (30%)
  - [ ] Testar extração com rpmextract
  - [ ] Testar cópia de diretórios
  - [ ] Testar criação de wrapper
  - [ ] Testar instalação de ícones
  - [ ] Testar casos de erro

- [ ] `installWithDebtap` (26.4%)
  - [ ] Testar conversão com debtap
  - [ ] Testar fallback
  - [ ] Testar casos de erro

- [ ] `copyDir` (68.1%)
  - [ ] Testar permissões
  - [ ] Testar symlinks
  - [ ] Testar diretórios vazios

**Mocks necessários**:
- `afero.Fs`
- `helpers.OSCommandRunner`
- `syspkg.Provider` (pacman)
- `icons.Manager`

### internal/backends/deb/deb.go
**Cobertura atual**: 67.3%

**Funções prioritárias**:
- [ ] `Install` (28.6%)
  - [ ] Testar fluxo completo de instalação
  - [ ] Testar mapeamento de dependências
  - [ ] Testar pós-instalação
  - [ ] Testar casos de erro

- [ ] `convertWithDebtapProgress` (45.9%)
  - [ ] Testar callback de progresso
  - [ ] Testar parsing de output do debtap

**Mocks necessários**:
- `afero.Fs`
- `helpers.OSCommandRunner`
- `syspkg.Provider`
- `icons.Manager`

### internal/ui/prompt.go
**Cobertura atual**: 62.9%

**Funções não testadas** (0%):
- [ ] `ConfirmPrompt`
  - [ ] Testar confirmação positiva
  - [ ] Testar confirmação negativa
  - [ ] Testar entrada inválida

- [ ] `InputPrompt`
  - [ ] Testar input válido
  - [ ] Testar validação
  - [ ] Testar entrada vazia

- [ ] `SelectPrompt`
  - [ ] Testar seleção válida
  - [ ] Testar índice inválido

- [ ] `SelectPromptDetailed`
  - [ ] Testar seleção com descrição

- [ ] `MultiSelectPrompt` (18.2%)
  - [ ] Testar múltiplas seleções
  - [ ] Testar confirmação

- [ ] `PasswordPrompt`
  - [ ] Testar input de senha

- [ ] `ConfirmDangerousAction`
  - [ ] Testar confirmação de ação perigosa

- [ ] `ConfirmWithDefault`
  - [ ] Testar valor padrão

**Estratégia de Refatoração**:
```go
// Criar interface para input
type UserInputReader interface {
    ReadLine() (string, error)
    ReadPassword() (string, error)
}

// Adicionar parâmetro opcional aos prompts
func ConfirmPrompt(message string, input UserInputReader) (bool, error) {
    if input == nil {
        input = &DefaultInputReader{}
    }
    // ...
}
```

### internal/cmd/install.go
**Cobertura atual**: 63.2% (NewInstallCmd)

**Funções prioritárias**:
- [ ] `NewInstallCmd`
  - [ ] Testar flags e argumentos
  - [ ] Testar integração com backends

- [ ] `fixDockIcon` (0%)
  - [ ] Testar chamada ao hyprland client

### internal/cmd/uninstall.go
**Cobertura atual**: 48.6% (executeUninstall)

**Funções prioritárias**:
- [ ] `executeUninstall`
  - [ ] Testar caminhos de erro
  - [ ] Testar rollback

- [ ] `runInteractiveUninstall` (0%)
  - [ ] Testar seleção de pacote

### cmd/upkg/main.go
**Cobertura atual**: 0.0%

**Estratégia**:
- [ ] Criar testes de integração end-to-end
- [ ] Testar subcomandos principais: install, uninstall, list, info
- [ ] Testar flags globais

**Exemplo de estrutura**:
```go
func TestMainIntegration(t *testing.T) {
    // Compilar binário de teste
    // Executar comandos e verificar resultado
    // Usar packages de teste temporários
}
```

### internal/helpers/archive.go
**Cobertura atual**: 80.4%

**Funções com baixa cobertura**:
- [ ] `ExtractTarXz` (25%)
  - [ ] Testar extração de tar.xz

- [ ] `ExtractTarBz2` (30%)
  - [ ] Testar extração de tar.bz2

- [ ] `extractTar` (46.4%)
  - [ ] Testar diferentes compressões

- [ ] `extractFile` (66.7%)
  - [ ] Testar extração de arquivo específico

### internal/icons/icons.go
**Cobertura atual**: 80.9%

**Funções com baixa cobertura**:
- [ ] `getImageDimensions` (58.8%)
  - [ ] Testar PNG, SVG
  - [ ] Testar formatos inválidos

- [ ] `createIconThemeSection` (66.7%)
  - [ ] Testar criação de seção

- [ ] `parseSquareSize` (66.7%)
  - [ ] Testar parsing de tamanho

- [ ] `copyIcon` (66.7%)
  - [ ] Testar cópia com redimensionamento

### internal/db/db.go
**Cobertura atual**: 80.4%

**Funções prioritárias**:
- [ ] `applyMigrations` (66.7%)
  - [ ] Testar cada migração
  - [ ] Testar versões do schema

## Recomendações Técnicas

### Setup de Infraestrutura de Testes

#### 1. Configuração de Coverage no CI/CD

Adicionar ao workflow GitHub Actions:
```yaml
- name: Run tests with coverage
  run: |
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

- name: Check coverage threshold
  run: |
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
    echo "Coverage: $COVERAGE%"
    if (( $(echo "$COVERAGE < 80" | bc -l) )); then
      echo "Coverage below 80%"
      exit 1
    fi
```

#### 2. Geração de Relatório HTML

```bash
go tool cover -html=coverage.out -o coverage.html
```

#### 3. Coverage por Pacote

```bash
go test -coverprofile=coverage.out ./internal/backends/tarball
go tool cover -func=coverage.out | grep tarball
```

### Boas Práticas

#### 1. Uso de Table-Driven Tests
```go
func TestExtractTar(t *testing.T) {
    tests := []struct {
        name    string
        archive string
        dest    string
        wantErr bool
    }{
        {"valid tar", "test.tar", "/tmp", false},
        {"invalid tar", "invalid.tar", "/tmp", true},
        {"no permission", "test.tar", "/root", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // teste aqui
        })
    }
}
```

#### 2. Setup e Teardown com t.Cleanup
```go
func TestInstall(t *testing.T) {
    tempDir := t.TempDir()
    fs := afero.NewMemMapFs()

    t.Cleanup(func() {
        // cleanup automático
    })

    // teste aqui
}
```

#### 3. Subtestes para Organização
```go
func TestBackend(t *testing.T) {
    t.Run("Detect", testDetect)
    t.Run("Install", testInstall)
    t.Run("Uninstall", testUninstall)
}
```

#### 4. Helpers de Teste Reutilizáveis
```go
// internal/testutil/helpers.go
func CreateTestTarball(t *testing.T, files map[string]string) string {
    // ...
}

func CreateMockBackend(t *testing.T) *Backend {
    // ...
}
```

### Padrões de Mock

#### 1. OSCommandRunner Mock
```go
type MockCommandRunner struct {
    Output   string
    ExitCode int
    Error    error
}

func (m *MockCommandRunner) RunCommand(cmd string, args ...string) error {
    return m.Error
}
// implementar outros métodos...
```

#### 2. Filesystem Mock com afero
```go
fs := afero.NewMemMapFs()
afero.WriteFile(fs, "/test/file", []byte("content"), 0644)
```

#### 3. Icon Manager Mock
```go
type MockIconManager struct{}

func (m *MockIconManager) InstallIcon(...) error {
    return nil
}
```

### Ferramentas Sugeridas

#### 1. Para Cobertura
- `go tool cover`: ferramenta nativa
- `gocov`: alternativas avançadas

#### 2. Para Mocking
- `afero`: filesystem abstraction
- `testify/mock`: framework de mocking (opcional)

#### 3. Para Geração de Dados
- Criar fixtures em `pkg-test/`
- Arquivos de teste pequenos e focados

#### 4. Para Testes de Integração
- Binário compilado para testes
- Pacotes de teste reais pequenos

## Métricas de Acompanhamento

### Objetivos Semanais

| Semana | Cobertura Meta | Pacotes/Módulos Focus |
|--------|----------------|----------------------|
| 1 | 75% | config, paths, pacman, security |
| 2 | 77% | desktop, icons (funções 66-73%) |
| 3 | 79% | tarball (Install, extractIcons) |
| 4 | 81% | appimage (Install, extractAppImage) |
| 5 | 83% | rpm (installWithExtract, installWithDebtap) |
| 6 | 85% | deb (Install, convertWithDebtapProgress) |
| 7 | 87% | ui (prompt functions), cmd (install, uninstall) |
| 8 | 90% | helpers, db, main (integração) |

### Verificação de Progresso

Comandos úteis:
```bash
# Cobertura total
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | grep total

# Cobertura por pacote
go test -coverprofile=coverage.out ./internal/backends/tarball && go tool cover -func=coverage.out

# Gerar HTML
go tool cover -html=coverage.out -o coverage.html

# Comparar com baseline anterior
diff <(go tool cover -func=coverage.old) <(go tool cover -func=coverage.out)
```

## Riscos e Mitigações

### Riscos Identificados

#### 1. Funções Interativas (UI/Prompt)
- **Impacto**: Alto - Difícil de testar funções que dependem de input do usuário
- **Mitigação**: Refatorar para aceitar interfaces de input injetáveis

#### 2. Dependências Externas (debtap, pacman, rpm)
- **Impacto**: Médio - Testes podem depender de ferramentas do sistema
- **Mitigação**: Usar mocks extensivos para OSCommandRunner

#### 3. Operações de Sistema Real
- **Impacto**: Médio - Algumas operações requerem sistema real
- **Mitigação**: Usar afero para filesystem, criar ambientes de teste isolados

#### 4. Testes de Integração Complexos
- **Impacto**: Médio - Testes end-to-end podem ser lentos e frágeis
- **Mitigação**: Manter testes de integração separados, usar fixtures pequenos

#### 5. Manutenção de Testes
- **Impacto**: Baixo - Testes podem se tornar obsoletos
- **Mitigação**: Revisar testes durante refatorações, manter testes simples

## Conclusão

### Resumo Executivo

O projeto upkg possui atualmente **72.3% de cobertura de testes**, com gaps significativos em áreas críticas:

1. **Backends de instalação** (tarball, appimage, rpm, deb) possuem entre 57-67% de cobertura
2. **Camada de UI interativa** (prompt functions) tem apenas 62.9%
3. **Entry point do CLI** (main.go) possui 0% de cobertura

### Próximos Passos Imediatos (Fase 1)

Recomenda-se começar com **quick wins** que oferecem alto impacto com pouco esforço:

1. **Testar construtores simples** em `internal/syspkg/arch/pacman.go` (+5%)
2. **Cobrir `escapeGenericToken`** em `internal/desktop/desktop.go` (+2%)
3. **Completar branches** em `internal/security/validation.go` (+5%)
4. **Testar funções de ícones** com 66-73% de cobertura (+3-5%)

### Estratégia de Longo Prazo

1. **Semanas 3-5**: Focar em backends de instalação (tarball, appimage, rpm, deb)
2. **Semana 6-7**: Refatorar e testar camada de UI para aceitar mocks
3. **Semana 8**: Testes de integração e edge cases finais

### Valor do Negócio

Atingir 90% de cobertura proporcionará:
- **Maior confiança** em instalações e desinstalações
- **Detecção precoce** de regressões em backends críticos
- **Melhor manutenibilidade** do código base
- **Documentação viva** através dos testes

O investimento em testes é justificado dado que upkg lida com:
- Instalação de software (operações irreversíveis)
- Múltiplos formatos de pacote (complexidade)
- Integrações com sistema (desktop, hyprland, pacman)

---

**Documento gerado em**: 2025-01-05
**Cobertura baseline**: 72.3%
**Meta**: 90%
**Estimativa de conclusão**: 8 semanas
