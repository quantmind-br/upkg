# Aumentar Cobertura de Testes para 80%

## Arquivos abaixo de 80% (ordenados por prioridade)

- [ ] `internal/backends/deb` - 44.9% → 80%
- [ ] `internal/backends/tarball` - 50.9% → 80%
- [ ] `internal/backends/appimage` - 54.5% → 80%
- [ ] `internal/ui` - 57.2% → 80%
- [ ] `internal/cmd` - 57.3% → 80%
- [ ] `internal/backends/rpm` - 57.9% → 80%
- [ ] `internal/helpers` - 70.8% → 80%
- [ ] `internal/icons` - 72.0% → 80%
- [ ] `internal/db` - 74.5% → 80%
- [ ] `internal/heuristics` - 77.8% → 80%
- [ ] `cmd/upkg` - 0.0% → 80%
- [ ] `internal/syspkg` - sem testes

## Correções realizadas
- [x] Corrigir testes falhando em `tarball_extra_test.go` (Paths resolver)

## Verificação Final
- [ ] Rodar cobertura completa e validar >= 80% em todos