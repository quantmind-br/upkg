# BUG Report

## 1. `go test ./...` falha por `fmt.Errorf` com string dinâmica
- **Impacto**: pipelines de CI com Go ≥1.25 não conseguem compilar `internal/backends`; qualquer desenvolvedor que execute a suíte total de testes recebe erro imediato.
- **Reprodução**:
  ```bash
  go test ./...
  ```
  Saída relevante: `internal/backends/backend.go:117:20: non-constant format string in call to fmt.Errorf`.
- **Diagnóstico**: `fmt.Errorf(errorMsg)` em `internal/backends/backend.go:117` envia uma string construída dinamicamente para o formatador. Go 1.25 roda `go vet` automaticamente durante `go test` e agora exige que o argumento de formato seja constante, fazendo o teste falhar antes de rodar qualquer caso.
- **Correção sugerida**: trocar por `errors.New(errorMsg)` ou `fmt.Errorf("%s", errorMsg)` para manter compatibilidade com vet.

## 2. Validações de segurança permitem entradas perigosas
- **Impacto**: entradas malformadas (path traversal, null bytes, strings com shell injection) passam pelos guard rails, anulando proteções em torno de caminhos e identificadores.
- **Reprodução**:
  ```bash
  go test ./internal/security -run ValidateVersion
  go test ./internal/security -run ValidateFilePath
  go test ./internal/security -run IsPathWithinDirectory
  go test ./internal/security -run SanitizeString
  ```
  Cada comando lista múltiplas falhas (detalhes em `internal/security/validation_test.go`).
- **Diagnóstico**:
  - `ValidateVersion` (`internal/security/validation.go:68`) aceita string vazia, não verifica null byte nem caracteres como `;` ou `../`, contrariando os testes que esperam erro.
  - `ValidateFilePath` (`internal/security/validation.go:85`) recusa `/usr/bin/app` (deveria ser válido) e não invalida `\x00`, caminhos enormes ou diretórios ocultos, resultando nas falhas dos testes.
  - `IsPathWithinDirectory` (`internal/security/validation.go:184`) retorna `false` sem erro para caminhos relativos, mas os testes exigem erro para esse caso e `true` para caminhos absolutamente contidos.
  - `SanitizeString` (`internal/security/validation.go:111`) apenas remove controle e espaços nas extremidades; não substitui espaços ou caracteres especiais por hífens como os testes e a documentação esperam.
- **Correção sugerida**: alinhar as funções ao contrato exercitado pelos testes — reforçar verificações de caracteres perigosos, devolver erros quando apropriado e sanitizar conforme os padrões documentados.

## 3. Backend de tarball não suporta `.tar.xz`/`.tar.bz2` apesar da documentação
- **Impacto**: usuários que tentam instalar pacotes comprimidos em XZ ou BZip2 (citados como suportados no README e na mensagem de erro de detecção) recebem “unsupported package” imediatamente.
- **Reprodução**:
  1. `tmpdir=$(mktemp -d); mkdir -p "$tmpdir/src"; echo test > "$tmpdir/src/app"; tar -C "$tmpdir/src" -cJf "$tmpdir/app.tar.xz" .`
  2. Execute um snippet local importando `internal/helpers`/`internal/backends/tarball` (o script `tmp_main.go` usado nesta análise mostra): `helpers.DetectFileType` devolve `unknown` e `tarball.Detect` retorna `false` para o arquivo `.tar.xz`.
- **Diagnóstico**: `helpers.DetectFileType` (`internal/helpers/detection.go:49-112`) não reconhece cabeçalhos XZ/BZip2, retornando `FileTypeUnknown`. Consequentemente `TarballBackend.Detect` (`internal/backends/tarball/tarball.go:42-58`) rejeita os arquivos. Mesmo que passasse, `extractArchive` (`tarball.go:265-275`) delega `.tar.bz2`/`.tar.xz` para `helpers.ExtractTarGz`, que usa `gzip.NewReader`, falhando na extração.
- **Correção sugerida**: adicionar detecção de magic numbers para XZ/BZ2, encaminhar `.tar.xz` para um extrator baseado em `xz` (por exemplo `xz.Reader`/`compress/flate` alternativo) e `.tar.bz2` para `bzip2.NewReader`, ajustando `TarballBackend.Detect` para aceitar os novos `FileType`.
