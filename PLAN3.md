# upkg – Planos de Implementação (Arquivos .md Separados)

> **Nota:** Abaixo estão os conteúdos de **arquivos `.md` separados**, apresentados em um único documento para facilitar cópia/commit. Cada seção começa com o **nome do arquivo** que você deve criar no repositório.

---

## 📄 PR1-context-and-interfaces.md

# PR1 — Interfaces + Propagação de context.Context

## Objetivo
Introduzir interfaces para dependências críticas (especialmente cache) e propagar corretamente `context.Context` por todas as camadas, permitindo cancelamento, timeouts coerentes e testes mais fáceis.

## Problemas Atuais
- Uso de `context.Background()` dentro do `CacheManager` impede cancelamento centralizado.
- Dependência concreta (`*CacheManager`) dificulta mocks e testes.

## Escopo
- Criar interface `CacheUpdater`.
- Ajustar `CacheManager` e `MockCacheManager`.
- Propagar `ctx` em backends e comandos CLI.

## Passos Detalhados
1. Criar `internal/cache/interface.go` com a interface `CacheUpdater`.
2. Alterar métodos do `CacheManager` para receber `ctx`.
3. Remover `context.Background()` interno e usar `context.WithTimeout(ctx, ...)`.
4. Ajustar `MockCacheManager` para a nova assinatura.
5. Trocar dependências concretas por interface nos backends.
6. Garantir que comandos CLI criem e propaguem `ctx`.

## Critérios de Aceite
- Operações respeitam cancelamento (Ctrl+C).
- Nenhuma regressão funcional.

---

## 📄 PR2-cache-batching-and-debounce.md

# PR2 — Agregação e Debounce de Atualizações de Cache

## Objetivo
Evitar múltiplas execuções redundantes de `update-desktop-database` e `gtk-update-icon-cache` em instalações/desinstalações em lote.

## Escopo
- Introduzir `CacheUpdateBatcher`.
- Backends apenas sinalizam necessidade de atualização.
- Execução única ao final do comando.

## Design
- `MarkDesktopDatabase(dir)` / `MarkIconCache(dir)`
- `Flush(ctx, log)` executa 1x por diretório.

## Passos Detalhados
1. Criar `internal/cache/batcher.go`.
2. Injetar batcher nos backends.
3. Remover chamadas diretas aos updates de cache.
4. Executar `Flush()` no final do comando CLI.

## Critérios de Aceite
- Em batch install, cache é atualizado apenas uma vez.

---

## 📄 PR3-base-backend-and-options.md

# PR3 — BaseBackend + Construtores com Options

## Objetivo
Reduzir duplicação entre backends e padronizar injeção de dependências.

## Escopo
- Criar `Deps` e `BackendOption`.
- Criar `BaseBackend` comum.
- Migrar backends existentes.

## Passos Detalhados
1. Criar `internal/backends/common/deps.go`.
2. Definir opções: `WithRunner`, `WithCache`, `WithLogger`.
3. Criar `BaseBackend` com helpers comuns.
4. Migrar backends (Binary → AppImage → Deb/Rpm).

## Critérios de Aceite
- Menos código duplicado.
- Construtores padronizados.

---

## 📄 PR4-deb-backend-robust-conversion.md

# PR4 — Conversão DEB Robusta (Sem Heurísticas Hardcoded)

## Objetivo
Eliminar filtros frágeis baseados em nomes hardcoded e tornar a descoberta do pacote convertido determinística.

## Escopo
- Controlar diretório de saída do `debtap`.
- Descobrir artefato gerado de forma confiável.

## Passos Detalhados
1. Executar `debtap` em diretório controlado.
2. Implementar `findGeneratedPackage(outputDir, pkgHint)`.
3. Remover heurísticas como `goose`/`cursor`.
4. Adicionar testes com runner fake.

## Critérios de Aceite
- Conversão funciona para qualquer pacote.
- Testes cobrem 0/1/N artefatos.

---

## 📄 PR5-cli-ux-json-and-progress.md

# PR5 — UX/CLI: JSON, Logs e Progresso Reutilizável

## Objetivo
Melhorar automação, consistência de saída e experiência do usuário.

## Escopo
- Flag `--json`.
- Separação clara entre UI e logs.
- Progress reutilizável.

## Passos Detalhados
1. Criar structs de output JSON.
2. Implementar `--json` em `list`, `info`, `doctor`.
3. Introduzir helpers `ui.Info`, `ui.Error`.
4. Extrair progress spinner reutilizável.

## Critérios de Aceite
- CLI scriptável.
- Logs estruturados consistentes.

---

## 📄 PR6-tests-hermetic-and-contracts.md

# PR6 — Testes Herméticos e Contract Tests

## Objetivo
Aumentar cobertura e confiabilidade dos testes sem dependência do sistema.

## Escopo
- Fake `CommandRunner`.
- Contract tests para backends.
- Fixtures minimalistas.

## Passos Detalhados
1. Implementar `FakeRunner` configurável.
2. Criar harness de testes com FS temporário.
3. Definir contract tests (Detect/Install/Uninstall).
4. Criar fixtures simuladas para fluxos felizes.

## Critérios de Aceite
- Testes passam em ambiente limpo.
- Cobertura aumenta nos backends.

---

## Ordem Recomendada de Execução
1. PR1
2. PR2
3. PR4
4. PR3
5. PR6
6. PR5

