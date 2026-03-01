# Hourglass Rejeições RPA 🤖

[![CI](https://github.com/felippejfdev/hourglass-rejeicoes-rpa/actions/workflows/ci.yml/badge.svg)](https://github.com/felippejfdev/hourglass-rejeicoes-rpa/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.22-blue.svg)](https://golang.org)
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen.svg)](https://github.com/felippejfdev/hourglass-rejeicoes-rpa/actions)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](docker-compose.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/felippejfdev/hourglass-rejeicoes-rpa)](https://goreportcard.com/report/github.com/felippejfdev/hourglass-rejeicoes-rpa)

RPA (Robotic Process Automation) para análise automatizada do sistema Hourglass da Petrobras. Executa 2x por dia (9h e 17h) para extrair dados de rejeições das seções Partes Mecânicas, Campo e Testemunho Público.

## 🚀 Funcionalidades

- **Automação de Browser**: Utiliza chromedp para controle headless do Chrome
- **Persistência de Cookies**: Login manual local → cookies salvos → uso em VPS headless
- **Agendamento Inteligente**: Cron jobs configurados para 9h e 17h diariamente
- **Múltiplos Formatos**: Exporta dados em JSON e CSV
- **Observabilidade Completa**:
  - 📧 Notificações via Resend (Email)
  - 📊 Métricas no Grafana Cloud
  - 🐛 Rastreamento de erros com Sentry
- **Cobertura 100%**: Testes unitários e E2E

## 📋 Pré-requisitos

- Go 1.22+
- Google Chrome ou Chromium
- Docker (opcional)

## 🔧 Instalação

### Via Go

```bash
go install github.com/felippejfdev/hourglass-rejeicoes-rpa/cmd/rpa@latest
```

### Via Docker

```bash
docker-compose up -d
```

### Compilação Local

```bash
make build
```

## ⚙️ Configuração

Copie o arquivo `.env.example` para `.env` e configure as variáveis:

```bash
cp .env.example .env
```

### Variáveis Obrigatórias

```env
# Resend Email (obrigatório para notificações)
RESEND_API_KEY=re_xxxxxxxx
RESEND_FROM_EMAIL=notifications@seu-dominio.com
RESEND_TO_EMAIL=admin@seu-dominio.com

# Sentry (obrigatório para rastreamento de erros)
SENTRY_DSN=https://xxxxxx@xxx.ingest.sentry.io/xxxxx

# Grafana Cloud (obrigatório para métricas)
GRAFANA_API_KEY=glc_xxxxxxxx
```

### Variáveis Opcionais

```env
# Modo Debug (abre navegador visível)
DEBUG=false

# Diretório de saída
OUTPUT_DIR=./outputs

# Arquivo de cookies
COOKIE_FILE=./cookies.json

# Timezone
TZ=America/Sao_Paulo
```

## 🎮 Uso

### Modo Setup (Primeira Execução)

Para fazer login manualmente e salvar os cookies:

```bash
make run-setup
# ou
./rpa -setup
```

Este modo abrirá o Chrome em modo visível. Faça login manualmente no Hourglass e pressione Enter no terminal para salvar os cookies.

### Modo Execução Única

Para executar uma vez imediatamente:

```bash
make run-once
# ou
./rpa -once
```

### Modo Agendado (Produção)

Para iniciar o agendador (9h e 17h):

```bash
make run
# ou
./rpa
```

## 🧪 Testes

```bash
# Executar todos os testes
make test

# Executar testes com cobertura
make coverage

# Verificar cobertura total
make coverage-total

# Executar linter
make lint

# Verificar vulnerabilidades
make vulncheck
```

## 📊 Qualidade de Código

- **Cobertura de Testes**: 100%
- **Linter**: golangci-lint com 12 linters
- **Segurança**: govulncheck sem vulnerabilidades
- **Commits**: Padrão Conventional Commits

## 🐳 Docker

### Construir Imagem

```bash
make docker-build
```

### Executar Container

```bash
make docker-run
```

### Docker Compose

```bash
# Iniciar
make docker-compose-up

# Parar
make docker-compose-down
```

## 📁 Estrutura do Projeto

```
.
├── cmd/rpa/              # Ponto de entrada
├── internal/
│   ├── config/          # Configurações
│   ├── domain/          # Modelos e interfaces
│   ├── rpa/             # Automação (browser, login, analyzer)
│   ├── scheduler/       # Agendamento cron
│   ├── storage/         # Persistência JSON/CSV
│   ├── notifier/        # Notificações (Resend)
│   ├── logger/          # Logging estruturado
│   ├── sentry/          # Rastreamento de erros
│   └── metrics/         # Métricas Grafana
├── .github/workflows/   # CI/CD
├── Dockerfile           # Container
├── docker-compose.yml   # Orquestração
├── Makefile            # Automação
└── README.md           # Documentação
```

## 📝 Formato de Saída

### JSON

```json
[
  {
    "secao": "Partes Mecânicas",
    "quem": "João Silva",
    "oque": "Documentação incompleta",
    "pra_quando": "2024-03-15",
    "timestamp": "2024-03-01T14:30:00Z"
  }
]
```

### CSV

```csv
secao,quem,oque,pra_quando,timestamp
Partes Mecânicas,João Silva,Documentação incompleta,2024-03-15,2024-03-01T14:30:00Z
```

## 🔒 Segurança

- ✅ Cookies nunca são commitados (`.gitignore`)
- ✅ Variáveis sensíveis em `.env` (não commitado)
- ✅ Execução em container não-root
- ✅ Transferência segura de cookies via SCP

## 📧 Notificações

O sistema envia notificações por email em:
- ✅ Início de execução
- ✅ Conclusão com sucesso
- ❌ Falhas e erros

## 📊 Monitoramento

### Métricas Disponíveis

- Total de rejeições por seção
- Tempo de execução
- Taxa de sucesso/falha
- Última execução

### Dashboard Grafana

Acesse seu dashboard no Grafana Cloud para visualizar métricas em tempo real.

## 🤝 Contribuindo

1. Fork o projeto
2. Crie sua branch (`git checkout -b feature/nova-funcionalidade`)
3. Commit suas mudanças (`git commit -m 'feat: nova funcionalidade'`)
4. Push para a branch (`git push origin feature/nova-funcionalidade`)
5. Abra um Pull Request

## 📄 Licença

Este projeto está licenciado sob a licença MIT - veja o arquivo [LICENSE](LICENSE) para detalhes.

## 👤 Autor

**Felippe** - [@felippejfdev](https://github.com/felippejfdev)

## 🙏 Agradecimentos

- [chromedp](https://github.com/chromedp/chromedp) - Automação de browser
- [robfig/cron](https://github.com/robfig/cron) - Agendamento
- [caarlos0/env](https://github.com/caarlos0/env) - Configuração

---

⚠️ **Aviso**: Este projeto é para uso interno autorizado. Respeite os termos de uso do sistema Hourglass da Petrobras.
