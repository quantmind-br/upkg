# Relatório de Bugs e Riscos

Auditoria ampla realizada em 2026-07-22 sobre os fluxos de instalação, reinstalação, detecção, desinstalação, persistência, wrappers e testes do `upkg`.

## Resumo

| Severidade | Quantidade |
|---|---:|
| Alta | 4 |
| Média | 6 |
| Baixa | 2 |
| **Total** | **12** |

Os problemas mais graves podem apagar uma instalação funcional durante `--force`, remover o registro do banco mesmo quando arquivos continuam instalados e manter Flatpaks removidos como se ainda estivessem instalados.

## Alta

### BUG-001 — `--force` apaga a instalação anterior antes de validar a substituta

**Afetados:**

- `internal/backends/appimage/appimage.go:190-218`
- `internal/backends/binary/binary.go:114-138`
- `internal/backends/tarball/tarball.go:157-186`
- `internal/backends/rpm/rpm.go:182-209`

**Problema:** os quatro backends removem o binário ou diretório instalado assim que encontram `opts.Force`. O backup anterior não é registrado na transação. Os rollbacks adicionados posteriormente somente removem a nova instalação.

**Impacto:** se a cópia, extração, busca de executável, criação de wrapper ou desktop falhar, a versão funcional anterior é perdida. Executar `tx.Rollback()` não a restaura.

**Evidência:** reprodução de runtime no backend binário: a reinstalação forçada foi induzida a falhar depois da remoção; o conteúdo antigo desapareceu e o rollback não conseguiu recuperá-lo. O mesmo padrão destrutivo aparece nos quatro arquivos acima.

**Correção sugerida:** instalar em caminho temporário e fazer troca atômica somente depois de validar todos os artefatos; alternativamente, renomear a instalação anterior para backup e registrar sua restauração no rollback antes de qualquer remoção.

---

### BUG-002 — Desinstaladores ocultam falhas de remoção e retornam sucesso

**Afetados:**

- `internal/backends/appimage/appimage.go:325-367`
- `internal/backends/binary/binary.go:238-286`
- `internal/backends/tarball/tarball.go:333-382`
- `internal/backends/rpm/rpm.go:408-448`
- `internal/cmd/uninstall.go:455-482`

**Problema:** falhas de `Remove`/`RemoveAll` em binários, diretórios, wrappers e arquivos desktop são apenas registradas como warning. Cada backend termina com `return nil`. `performUninstall` interpreta isso como sucesso e prossegue para excluir o registro do banco.

**Impacto:** arquivos executáveis e integrações podem permanecer no sistema sem registro no `upkg`, tornando a instalação órfã e difícil de remover. A CLI informa sucesso falso.

**Correção sugerida:** agregar erros obrigatórios de remoção e retornar erro. O registro no banco somente deve ser excluído quando todos os artefatos essenciais forem removidos; falhas de cache podem continuar não fatais, mas devem ser diferenciadas de falhas de desinstalação.

---

### BUG-003 — Flatpak removido continua registrado no banco

**Afetado:** `internal/cmd/uninstall.go:469-475`

**Problema:** `performUninstall` contém um desvio explícito para `PackageTypeFlatpak` que imprime sucesso sem chamar `database.Delete`.

**Impacto:** `upkg list` e operações posteriores continuam exibindo um pacote já removido. Uma nova desinstalação trabalha com estado obsoleto.

**Evidência:** reprodução com executável `flatpak` falso: o comando de desinstalação retornou sucesso, mas a linha correspondente permaneceu no SQLite.

**Correção sugerida:** remover a exceção de Flatpak. Após `Backend.Uninstall` bem-sucedido, excluir o registro de todos os tipos de pacote pelo mesmo fluxo transacional.

---

### BUG-004 — Falha ao excluir do banco é tratada como sucesso

**Afetado:** `internal/cmd/uninstall.go:471-482`

**Problema:** quando `database.Delete` falha, a CLI imprime somente um aviso; em seguida registra `"uninstallation completed successfully"` e retorna `nil`.

**Impacto:** automações recebem status zero apesar da inconsistência. O banco mantém um pacote removido e comandos futuros operam sobre dados incorretos.

**Correção sugerida:** retornar erro contextualizado quando `database.Delete` falhar. Se a remoção física já ocorreu, registrar claramente um estado de reconciliação necessário, sem declarar sucesso integral.

## Média

### BUG-005 — Instalação de AppImage altera permanentemente o arquivo de origem

**Afetado:** `internal/backends/appimage/appimage.go:119-142`

**Problema:** `Install` executa `Chmod(packagePath, 0755)` no pacote fornecido pelo usuário antes da extração. A permissão original não é preservada nem restaurada em caso de falha ou sucesso.

**Impacto:** um arquivo baixado com modo `0644` passa a `0755`, mesmo quando a instalação falha. Isso é uma mutação inesperada fora do diretório gerenciado pelo aplicativo.

**Evidência:** reprodução de runtime com AppImage inválida confirmou alteração de `0644` para `0755` após erro de instalação.

**Correção sugerida:** copiar o AppImage para um arquivo temporário e tornar somente a cópia executável, ou capturar o modo original e restaurá-lo com `defer`.

---

### BUG-006 — Fallback `unsquashfs` extrai no diretório de trabalho errado

**Afetado:** `internal/backends/appimage/appimage.go:382-403`

**Problema:** a tentativa principal usa `RunCommandInDir(..., destDir, ...)`, mas o fallback chama `RunCommand(..., "unsquashfs", "-d", "squashfs-root", ...)`. O destino relativo é resolvido no CWD do processo, não em `destDir`. Depois, `Install` procura `filepath.Join(tmpDir, "squashfs-root")`.

**Impacto:** o fallback pode extrair dados no diretório atual, deixar artefatos `squashfs-root/` e ainda falhar dizendo que a extração não foi encontrada.

**Evidência:** reprodução de runtime criou `squashfs-root` no CWD e falhou ao procurar o diretório sob o temporário esperado.

**Correção sugerida:** usar `RunCommandInDir(extractCtx, destDir, "unsquashfs", "-d", "squashfs-root", absAppImagePath)` ou passar um caminho de destino absoluto.

---

### BUG-007 — Detecção confia na extensão antes de verificar magic bytes

**Afetado:** `internal/helpers/detection.go:30-69`

**Problema:** `.deb`, `.rpm`, `.zip`, `.tar`, `.tar.gz`, `.tar.xz` e `.tar.bz2` retornam um tipo imediatamente pela extensão. A leitura de magic bytes só ocorre depois.

**Impacto:** arquivos arbitrários renomeados entram no backend errado e podem disparar ferramentas externas ou mensagens enganosas. Isso contradiz a regra arquitetural documentada de detecção por assinatura.

**Evidência:** um arquivo de texto chamado `fake.deb` foi classificado como DEB e o fluxo tentou executar `debtap`.

**Correção sugerida:** detectar primeiro pela assinatura e usar a extensão apenas para desambiguar formatos que compartilham compressão, nunca como prova suficiente de formato.

---

### BUG-008 — Wrappers gerados quebram com aspas em caminhos

**Afetado:** `internal/helpers/wrapper.go:22-51`

**Problema:** `ExecPath`, `execDir` e `execName` são interpolados diretamente entre aspas duplas em shell. Aspas, barras invertidas, crases e expansões presentes no caminho não são escapadas para Bash.

**Impacto:** nomes de diretório válidos podem produzir wrapper sintaticamente inválido. Dependendo do conteúdo, também há risco de interpretação shell indesejada.

**Evidência:** reprodução com caminho contendo `"` gerou script com aspas não balanceadas e falha no `bash`.

**Correção sugerida:** aplicar quoting POSIX/Bash robusto a cada argumento, por exemplo aspas simples com escape de `'`, ou evitar gerar shell e criar um launcher que execute argumentos sem reinterpretação.

---

### BUG-009 — `--timeout` aceita zero e valores negativos

**Afetados:**

- `internal/cmd/install.go:87-95,210-216`
- `internal/cmd/uninstall.go:60-75`

**Problema:** as flags são inteiros sem validação de limite. `context.WithTimeout` com duração zero ou negativa cria um contexto já expirado.

**Impacto:** o comando falha imediatamente, frequentemente durante a abertura do banco, com erro de deadline pouco relacionado ao parâmetro inválido.

**Evidência:** `--timeout 0` e `--timeout -1` foram aceitos; a operação alcançou o banco com o contexto expirado. O teste `TestInstallCmd_InvalidTimeout` também executa `-1`, mas descarta o resultado.

**Correção sugerida:** validar `timeoutSecs > 0` antes de criar o contexto e retornar erro de uso claro do Cobra.

---

### BUG-010 — Testes paralelos alteram `HOME` global e poluem o repositório

**Afetados:** numerosos testes em:

- `internal/backends/appimage/*_test.go`
- `internal/backends/tarball/*_test.go`

**Problema:** testes usam `t.Parallel()` e, no mesmo processo, chamam `os.Setenv("HOME", ...)` ou `os.Unsetenv("HOME")`. Variáveis de ambiente são globais ao processo, não isoladas por teste. Vários resolvers acabam capturando o `HOME` de outro teste ou o diretório de trabalho.

**Impacto:** testes dependentes de ordem, falhas intermitentes, remoção concorrente de `TempDir` e criação de artefatos dentro da árvore fonte. Foram observados `internal/backends/appimage/.local/`, `internal/backends/tarball/.local/` e `internal/backends/tarball/~/.npm/`.

**Evidência:** `go test -race -count=10 ./internal/backends/tarball` falhou com:

- `already_installed_without_force` chegando à extração em vez de detectar instalação existente;
- `TempDir RemoveAll cleanup: ... directory not empty`;
- falhas adicionais dependentes de poluição entre testes.

**Correção sugerida:** nunca combinar `t.Parallel` com mutação de ambiente global. Preferir `paths.NewResolverWithHome` e configuração explícita de diretórios. Quando inevitável, usar `t.Setenv` sem paralelismo. Remover os artefatos gerados da árvore fonte.

## Baixa

### BUG-011 — `make lint` está inutilizável com golangci-lint atual

**Afetados:** `.golangci.yml` e `Makefile`

**Problema:** a configuração usa o esquema legado sem campo `version`. O `golangci-lint 2.12.2` encerra antes da análise com `unsupported version of the configuration: ""`.

**Impacto:** `make lint`, `make quick-check` e `make validate` não cumprem sua função; regressões que deveriam ser detectadas pelo lint passam sem verificação local/CI.

**Evidência:** `golangci-lint run ./...` retornou código 3 com o erro acima.

**Correção sugerida:** migrar a configuração para o formato v2, incluindo `version: "2"` e as chaves equivalentes conforme o guia de migração.

---

### BUG-012 — Testes críticos descartam resultados ou ficam permanentemente ignorados

**Exemplos:**

- `internal/cmd/install_test.go:489-505` executa timeout negativo e ignora o erro;
- `internal/backends/appimage/appimage_test.go:1240-1265` ignora o resultado da reinstalação forçada;
- `internal/backends/appimage/appimage_extra_test.go` possui vários `_ = resultPath` / `_ = err`;
- `internal/backends/tarball/tarball_extra_test.go:53-76` ignora com `t.Skip` os contratos de força, executáveis e desktop.

**Problema:** testes aparentam cobrir contratos importantes, mas não fazem assertions úteis ou não executam.

**Impacto:** bugs como perda da instalação anterior, timeout inválido e falhas de geração de desktop permanecem sem proteção contra regressão.

**Correção sugerida:** substituir descartes por `require`/`assert`, criar fixtures pequenas e determinísticas em runtime e remover skips permanentes. Cada teste deve provar um comportamento observável e falhar antes da correção correspondente.

## Verificações Executadas

| Verificação | Resultado |
|---|---|
| Índice estrutural completo do repositório | Pronto; 1.800 nós e 9.022 relações analisadas |
| Diagnósticos LSP do workspace | Nenhum problema |
| `go vet ./...` | Passou |
| `go test -race -count=10 ./internal/backends/tarball` | Falhou; confirmou interferência e poluição entre testes |
| `golangci-lint run ./...` | Não iniciou; configuração incompatível |
| Teste completo com race detector | Uma execução inicial passou; execução posterior falhou em `TestTarballBackend_Install_MorePaths/already_installed_with_force`, consistente com poluição global |
| `staticcheck`, `govulncheck`, análise `nilness` | Não disponíveis no ambiente |

## Ordem Recomendada de Correção

1. Tornar reinstalações `--force` atômicas e recuperáveis.
2. Fazer desinstaladores e `database.Delete` propagarem falhas corretamente.
3. Corrigir a exclusão de registros Flatpak.
4. Isolar `HOME` nos testes e limpar artefatos gerados.
5. Corrigir extração AppImage, detecção por assinatura e quoting de wrappers.
6. Validar timeouts.
7. Migrar golangci-lint e fortalecer os testes ignorados.
