# Refactoring/Design Plan: Implementação de Cobertura de Testes para 80%

## 1. Executive Summary & Goals
O objetivo principal deste plano é identificar lacunas na cobertura de testes existentes e implementar novos testes de unidade e integração para atingir uma meta mínima de **80% de cobertura** em todas as áreas principais do projeto `upkg`. O foco será em lógica de negócio crítica, especialmente nos *backends*, na camada de segurança e nas funções de utilidade.

### Key Goals:
1.  **Atingir 80%+ de Cobertura:** Garantir que o `make test-coverage` reporte uma cobertura total superior a 80%, focando especialmente nos pacotes com lógica complexa.
2.  **Testar Lógica Crítica:** Implementar testes para fluxos de `Install` e `Uninstall` em backends que atualmente têm cobertura insuficiente (e.g., `tarball`, `deb`, `rpm` com extração), mocks de FS e comandos externos.
3.  **Validar Segurança e Utilidade:** Garantir 100% de cobertura nas funções de `internal/security` e `internal/helpers/naming`.

## 2. Current Situation Analysis
O projeto `upkg` é um gerenciador de pacotes Go baseado em um Padrão Estratégico (backends) e um Gerenciador de Transações.

* **Estrutura de Arquivos:** A arquitetura é modular (`internal/`).
* **Pontos de Dor Relevantes:**
    * **Cobertura Baixa/Inexistente:** Muitos arquivos cruciais de lógica (backends, helpers, db, desktop) têm cobertura baixa ou inexistente (arquivos sem `*_test.go`).
    * **Testes Dependem do Sistema:** Os testes existentes, especialmente em `internal/backends/binary/binary_test.go` e `internal/helpers/detection_test.go`, dependem da existência de binários do sistema (`/bin/ls`) ou da capacidade de criar arquivos ELF, o que compromete a hermeticidade.
    * **Lógica de Backends não testada:** O core da lógica de instalação (extração, criação de wrappers, desktop files) na maioria dos backends é testado de forma mínima, como visto em `appimage_test.go`, onde o teste `TestInstall` espera por falha de extração.

**Arquivos de Maior Prioridade para Cobertura (sem `*_test.go` ou muito incompletos):**
* `internal/backends/tarball/tarball.go`
* `internal/backends/deb/deb.go` (lógica de `debtap` e limpeza de dependências)
* `internal/backends/rpm/rpm.go` (lógica de `rpmextract.sh`)
* `internal/desktop/desktop.go` (Parse, Write, InjectWaylandEnvVars)
* `internal/helpers/archive.go` (extração com checagens de segurança)
* `internal/db/db.go` (CRUD completo)
* `internal/security/paths.go` e `internal/security/validation.go`

## 3. Proposed Solution / Refactoring Strategy

A estratégia será o **Teste Baseado em Camadas (Layered Testing)**, focando primeiro nas camadas de utilidade e segurança, e em seguida na lógica de negócio (backends).

### 3.1. High-Level Design / Architectural Overview
A principal mudança arquitetural será a introdução de **mocks e injeção de dependência** para isolar a lógica de negócio de I/O de disco (usando `afero.Fs`) e chamadas de comando externo (`os/exec`).

1.  **Mocking de I/O de Disco (Afero):** Injetar `afero.Fs` nos backends e utilitários que interagem com o sistema de arquivos.
2.  **Mocking de Comandos Externos:** Criar uma interface e implementação de mock para as chamadas em `internal/helpers/exec.go` (RunCommand, CommandExists).
3.  **Mocks de Dependência:** Criar mocks para `internal/syspkg.Provider` e `internal/heuristics.Scorer` para testes de backends.

### 3.2. Key Components / Modules
| Componente | Responsabilidade | Descrição da Modificação |
| :--- | :--- | :--- |
| `internal/backends/*` | Implementar `Install`/`Uninstall` | Injetar dependências mockáveis (FS, comandos, syspkg). Mocks para `transaction.Manager` (já em uso). |
| `internal/db/db.go` | Persistência SQLite | Testar CRUD completo com *in-memory* SQLite. |
| `internal/helpers/exec.go` | Execução de Comandos | **Refatoração:** Introduzir um `CommandRunner` injetável para mockar `exec.Command`. |
| `internal/helpers/archive.go` | Extração Segura | Testar extração com `afero.Fs` e checagens de segurança (Zip Slip). |
| `internal/security/*` | Validação de Input | Cobertura total nas funções de sanitização e validação. |

### 3.3. Detailed Action Plan / Phases

#### Phase 1: Ferramentas de Teste e Estrutura (S/M)
**Objective(s):** Isolar dependências externas para tornar os testes herméticos.
**Priority:** High

| Task | Rationale/Goal | Estimated Effort | Deliverable/Criteria for Completion |
| :--- | :--- | :--- | :--- |
| **1.1: Refatorar `internal/helpers/exec`** | Permitir mocking de comandos externos globalmente. | M | Interface `CommandRunner` e injeção em `internal/helpers/*` e `internal/cache/*`. |
| **1.2: Refatorar `internal/backends/`** | Permitir mocking de FS. | S | Backends recebem `afero.Fs` (ou abstração) para todas operações de arquivo. |
| **1.3: Mock de `syspkg.Provider`** | Testar fluxo DEB/RPM (via pacman) sem sudo/pacman real. | S | `internal/syspkg/mock.go` com implementação de `Provider`. |

#### Phase 2: Cobertura de Lógica Crítica (M/L)
**Objective(s):** Atingir 100% de cobertura em pacotes de utilidade e segurança.
**Priority:** High

| Task | Rationale/Goal | Estimated Effort | Deliverable/Criteria for Completion |
| :--- | :--- | :--- | :--- |
| **2.1: Testes `security/` (Validation & Paths)** | Cobertura total para sanitização, nomes de pacotes e path traversal. | S | Arquivos `internal/security/*_test.go` com 100% de cobertura. |
| **2.2: Testes `helpers/naming`** | Cobertura total para `CleanAppName` e `FormatDisplayName`. | S | Arquivo `internal/helpers/naming_test.go` completo. |
| **2.3: Testes `desktop/desktop`** | Cobertura para `Parse`, `Write` e `InjectWaylandEnvVars` (incluindo validação de custom vars). | M | Arquivo `internal/desktop/desktop_test.go` completo. |
| **2.4: Testes `helpers/detection`** | Cobrir todas as detecções de tipo de arquivo (`IsELF`, `IsAppImage`, `DetectFileType`) com arquivos mockados (em vez de `/bin/ls`). | M | Testes de `internal/helpers/detection_test.go` herméticos. |
| **2.5: Testes `db/db`** | CRUD completo com `modernc.org/sqlite` em memória. | M | `internal/db/db_test.go` com testes para `Create`, `Get`, `List`, `Update`, `Delete` e `applyMigrations`. |

#### Phase 3: Cobertura de Backends (L)
**Objective(s):** Atingir cobertura de 80%+ nos métodos `Install` e `Uninstall` de cada backend.
**Priority:** Medium

| Task | Rationale/Goal | Estimated Effort | Deliverable/Criteria for Completion |
| :--- | :--- | :--- | :--- |
| **3.1: Testes `tarball/tarball`** | Simular extração, `FindExecutables` e criação de wrapper/desktop file. Mocks para FS e `heuristics.Scorer`. | L | `internal/backends/tarball/tarball_test.go` com mock de extração/heurísticas. |
| **3.2: Testes `appimage/appimage`** | Simular extração bem-sucedida, parsing de metadata e instalação de ícones/desktop. Mocks de comandos e FS. | L | `internal/backends/appimage/appimage_test.go` melhorado (passando no `TestInstall`). |
| **3.3: Testes `binary/binary`** | Testar `Detect`, `Install` (criação de binário e desktop), `Uninstall`. | S | `internal/backends/binary/binary_test.go` completo. |
| **3.4: Testes `deb/deb`** | Simular fluxo via `debtap`/`pacman` com mocks de comandos e `syspkg.Provider`. Foco no `fixDependencyLine`. | M | `internal/backends/deb/deb_test.go` completo (com teste para `fixMalformedDependencies`). |
| **3.5: Testes `rpm/rpm`** | Simular `installWithExtract` com mocks de `rpmextract.sh` e FS. Simular `installWithDebtap` com mocks. | L | `internal/backends/rpm/rpm_test.go` completo. |

## 4. Key Considerations & Risk Mitigation

### 4.1. Technical Risks & Challenges
| Risco | Descrição | Mitigação |
| :--- | :--- | :--- |
| **Mocking `os/exec`** | A refatoração de `internal/helpers/exec.go` para usar uma interface pode exigir mudanças em muitos locais de chamada. | A interface deve ser minimalista. Foco na injeção apenas em backends e helpers de cache que precisam de testes herméticos. |
| **Testes de Extração** | Simular extração de arquivos complexos (tar, zip) de forma segura e determinística. | Usar `afero.MemMapFs` para criar arquivos em memória e *mini*-arquivos de teste no disco para simular a extração real em testes de integração de alto nível (fora do `Extract*` em si). |
| **Regressão de Código de Produção** | Alterar backends para injeção de dependência pode introduzir bugs na lógica de produção. | Usar injeção de dependência via campo de struct/parâmetro opcional para manter compatibilidade, testando as implementações *reais* (não mockadas) na fase final. |

### 4.2. Dependencies
* **Interna (Task-to-Task):** Fase 1 é estritamente pré-requisito para as Fases 2 e 3.
* **Externa:** Nenhuma dependência externa nova. Requer `go` e ferramentas de dev (como o `golangci-lint` para o `make lint`).

### 4.3. Non-Functional Requirements (NFRs) Addressed
* **Testabilidade (Manutenção):** A principal NFR. A injeção de dependência e a hermeticidade dos testes reduzirão o tempo de depuração e aumentarão a confiança nas futuras refatorações.
* **Segurança:** Atingir 100% de cobertura em `internal/security` garante que todas as validações de caminhos (Zip Slip, Path Traversal) e sanitização de input sejam verificadas.
* **Confiabilidade:** Aumentar a cobertura da lógica de `Install`/`Uninstall` (e.g., rollback) assegura a estabilidade do produto final.

## 5. Success Metrics / Validation Criteria
* **Métrica Principal:** `go test -v -race -cover ./...` deve reportar **>= 80%** de cobertura.
* **Métrica Secundária:** Todos os pacotes cruciais (`backends/*`, `db`, `security`, `desktop`) devem ter **>= 75%** de cobertura.
* **Qualitativa:** Todos os fluxos de sucesso e erro (e.g., falha de extração, falha de validação de nome) devem ser testados com mocks para garantir o tratamento correto.

## 6. Assumptions Made
* A injeção de `afero.Fs` e do mock de `os/exec` é a abordagem mais eficiente para atingir a meta de cobertura sem depender do estado real do sistema.
* O uso de *in-memory* SQLite (`modernc.org/sqlite`) é suficiente para testar a camada `internal/db`.

## 7. Open Questions / Areas for Further Investigation
* Qual o *timing* ideal para refatorar `internal/logging/logger.go` para injetar `os.Stderr` (para testes)? (A ser investigado na Fase 1, se o tempo permitir).