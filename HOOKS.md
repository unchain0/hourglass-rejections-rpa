# Git Hooks Setup (Lefthook)

Este projeto usa **Lefthook** - um gerenciador de git hooks escrito em Go nativo.

## Instalação

```bash
# Instalar o lefthook
go install github.com/evilmartians/lefthook@latest

# Instalar os hooks no projeto
lefthook install
```

## Hooks Configurados

### Pre-commit (executado antes de cada commit)
- `format` - Formata código Go com gofmt
- `lint` - Executa golangci-lint
- `test` - Executa testes com race detector
- `govulncheck` - Verifica vulnerabilidades
- `build` - Verifica se o projeto compila

### Pre-push (executado antes de cada push)
- `test` - Executa todos os testes
- `coverage` - Verifica se cobertura está acima de 70%

## Uso

Após instalar, os hooks serão executados automaticamente:

```bash
# Commit normal - executa pre-commit hooks automaticamente
git commit -m "feat: nova funcionalidade"

# Push - executa pre-push hooks automaticamente
git push origin main
```

## Executar manualmente

```bash
# Executar pre-commit manualmente
lefthook run pre-commit

# Executar pre-push manualmente
lefthook run pre-push

# Executar todos os hooks
lefthook run
```

## Configuração

Os hooks estão configurados em `lefthook.yml`.
