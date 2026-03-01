# Hourglass Rejeições RPA - Plano de Trabalho

## TL;DR

> **Objetivo**: Criar projeto Go RPA para analisar sistema Hourglass (Petrobras) 2x ao dia (9h e 17h), extraindo rejeições de designação das seções "Partes Mecânicas", "Campo" e "Testemunho Público".
>
> **Stack**: Go 1.25.7, chromedp (browser headless), robfig/cron/v3 (agendamento)
>
> ⚠️ **IMPORTANTE para Agentes Executores**:
> - Todas as ferramentas usam **Free Tiers** ($0)
> - **API Keys necessárias**: Ver seção '🔐 API Keys Necessárias' antes de implementar Tasks 17, 19, 20
> - **Sempre perguntar ao usuário** no chat quando precisar de uma API Key
> - NUNCA prossiga sem a chave fornecida pelo usuário
>
> **Entregáveis**:
> - Código fonte completo (main.go, login.go, analise.go, types.go, etc.)
> - Testes unitários e E2E com 100% coverage
> - GitHub Actions CI/CD
> - Dockerfile e docker-compose.yml
> - Makefile e README.md
>
> **Esfoco Estimado**: Medium (2-3 dias)
> **Execução Paralela**: Sim (3 ondas)
> **Caminho Crítico**: Setup projeto → Core domain → Browser automation → Testes → Docker → CI/CD

---

## Contexto

### Requisitos Originais
- **Nome**: hourglass-rejeicoes-rpa
- **Funcionalidade**: RPA para análise de rejeições no sistema Hourglass (Petrobras)
- **Frequência**: 2x ao dia (9h e 17h - manhã e fim de tarde)
- **Seções**: "Partes Mecânicas", "Campo", "Testemunho Público"
- **Dados extraídos**: Quem rejeitou (tooltip ícone vermelho), o que foi rejeitado (texto célula), "pra quando" (data última coluna AG-Grid)
- **Login**: Via Google (OAuth) com persistência de cookies

### Requisitos Adicionais
- Testes E2E obrigatórios
- GitHub Actions obrigatório
- Coverage 100% de toda a codebase
- Dockerfile e docker-compose.yml obrigatórios
- Commits na branch `main` após cada implementação relevante
- Melhores práticas de desenvolvimento Go

### Pesquisa Realizada

**Padrões chromedp descobertos**:
- Flags obrigatórias para containers: `--no-sandbox`, `--disable-dev-shm-usage`
- Persistência de cookies via `chrome.Cookies` (save) e `chrome.SetCookie` (load)
- Extração de dados via `chromedp.Nodes()` + `chromedp.AttributeValue()`
- Uso de `chromedp.ByQuery` e `chromedp.ByQueryAll` para seleção

**Padrões robfig/cron/v3 descobertos**:
- Setup com `cron.New()` + `cron.WithChain()` para wrappers
- Wrappers recomendados: `cron.Recover()`, `cron.SkipIfStillRunning()`, `cron.DelayIfStillRunning()`
- Timezone via `cron.WithLocation()` ou prefixo `CRON_TZ=` nas expressões
- Graceful shutdown com `c.Stop()` que retorna `context.Context`

**Padrões de testes Go descobertos**:
- Interfaces para permitir mocking (ex: `HourglassScraper`)
- `httptest.Server` para testar chromedp com páginas HTML locais
- Build tags (`//go:build e2e`) para separar testes E2E
- `testify/mock` ou implementações manuais para mocks

**Recomendações arquiteturais (Oracle)**:
- Clean Architecture: `cmd/`, `internal/domain/`, `internal/rpa/`, `internal/scheduler/`
- Não mockar chromedp diretamente - usar `httptest.Server`
- Docker com `dumb-init` para gerenciar processos Chrome
- `log/slog` com saída JSON para observabilidade
- Captura de screenshot/HTML em caso de erro para debug

---

## Objetivos de Trabalho

### Objetivo Principal
Criar projeto Go production-ready para automação de análise de rejeições no sistema Hourglass, com agendamento automático, persistência de sessão, extração estruturada de dados e entrega de relatórios.

### Entregáveis Concretos
1. `go.mod` com módulo `hourglass-rejeicoes-rpa`
2. `main.go` - Entry point com cron scheduler
3. `internal/config/config.go` - Configuração via variáveis de ambiente
4. `internal/domain/types.go` - Models (Rejeicao, JobResult)
5. `internal/domain/interfaces.go` - Interfaces (Scraper, Storage, Notifier)
6. `internal/rpa/browser.go` - Setup chromedp
7. `internal/rpa/login.go` - Login e persistência de cookies
8. `internal/rpa/analyzer.go` - Análise de rejeições
9. `internal/scheduler/scheduler.go` - Agendamento cron
10. `internal/storage/json.go` - Persistência JSON/CSV
11. `internal/notifier/resend.go` - Notificações por email (Resend)
12. `internal/logger/logger.go` - Logging estruturado (slog + lumberjack)
13. `internal/sentry/sentry.go` - Error tracking (Sentry)
14. `internal/metrics/metrics.go` - Métricas para Grafana Cloud
15. Testes unitários (`*_test.go`) com 100% coverage
16. Testes E2E (`e2e_test.go`) com build tag
17. `Dockerfile` multi-stage com dumb-init
18. `docker-compose.yml` com Chrome headless
19. `.github/workflows/ci.yml` - CI/CD com coverage
20. `Makefile` - build, test, run, clean
21. `README.md` - Documentação completa
22. `.gitignore` - cookies.json, outputs/


### Definição de Pronto
- [ ] Todos os arquivos de código implementados
- [ ] Testes unitários com 100% coverage (comprovado via `go test -cover`)
- [ ] Testes E2E passando
- [ ] Build Docker funcionando
- [ ] Docker Compose subindo serviço completo
- [ ] CI/CD no GitHub Actions passando
- [ ] Código commitado na branch `main`

### Must Have (Obrigatórios)
- Agendamento 2x ao dia (9h, 17h)
- Análise das 3 seções especificadas
- Extração completa dos dados (Quem, OQue, PraQuando)
- Persistência de cookies para reutilização de sessão
- Output em JSON e CSV
- Graceful shutdown
- Logging estruturado (JSON)
- Tratamento de erros com retry
- Headless mode em produção
- Debug mode via flag

### 🚨 SEGURANÇA - LEIA COM ATENÇÃO

#### Proteção de Credenciais e Cookies

⚠️ **NUNCA commitar no GitHub:**
- `cookies.json` (sessão de login do Google) - está no `.gitignore`
- `.env` (variáveis de ambiente com API keys)
- Qualquer arquivo com tokens, senhas ou credenciais

✅ **Transferência segura para VPS:**
- Usar `scp` (SSH) para copiar cookies.json: `scp cookies.json user@vps:/path/`
- Ou usar `rsync` com criptografia
- NUNCA enviar por email, Slack, ou outro canal inseguro

#### Lista de Arquivos no `.gitignore` (Protegidos):
```
cookies.json      # Cookies de sessão (NUNCA commitar!)
*.json            # Arquivos de output
outputs/          # Diretório de saída
.env              # Variáveis de ambiente
*.log             # Logs
.env.local        # Configurações locais
```

#### O que ACONTECE se commitar acidentalmente:
1. Sessão do Google exposta → Risco de acesso não autorizado
2. API keys expostas → Uso indevido e cobranças
3. Precisará revogar todos os tokens e regenerar API keys

#### Checklist de Segurança antes de cada commit:
- [ ] `git status` não mostra cookies.json
- [ ] `git status` não mostra .env
- [ ] Nenhuma API key hardcoded no código
- [ ] Nenhuma senha no código

### 📋 Checklist Completo de Segurança

#### 1. Secrets Management (Gerenciamento de Segredos)

**API Keys e Tokens:**
- [ ] Nunca hardcodear API keys no código
- [ ] Usar variáveis de ambiente (`.env` no `.gitignore`)
- [ ] Validar que todas as env vars obrigatórias estão definidas no startup
- [ ] Rotacionar API keys a cada 90 dias (documentado no README)
- [ ] Usar diferentes API keys para dev/prod

**Cookies de Sessão:**
- [ ] Arquivo `cookies.json` com permissão 600 (apenas owner)
- [ ] Nunca logar conteúdo dos cookies
- [ ] Validar integridade dos cookies antes de usar
- [ ] Implementar expiry check (renovar se necessário)
- [ ] Criptografar cookies em disco (opcional, alta segurança)

**Implementação:**
```go
// internal/config/config.go
type Config struct {
    ResendAPIKey     string `env:"RESEND_API_KEY,required"`
    SentryDSN        string `env:"SENTRY_DSN,required"`
    GrafanaAPIKey    string `env:"GRAFANA_API_KEY,required"`
}

// Validação no startup
func (c *Config) Validate() error {
    if c.ResendAPIKey == "" {
        return fmt.Errorf("RESEND_API_KEY não configurada")
    }
    // ... validar outras keys
    return nil
}
```

---

#### 2. Secure Logging (Logs Seguros)

**O que NUNCA logar:**
- [ ] Tokens de autenticação
- [ ] Cookies de sessão
- [ ] API keys (mesmo parcialmente)
- [ ] Dados pessoais (emails, nomes de rejeitadores)
- [ ] Senhas ou credenciais
- [ ] Informações bancárias ou financeiras

**O que SIM logar:**
- [ ] IDs de execução (execution_id)
- [ ] Timestamps
- [ ] Status das operações (success/failure)
- [ ] Duração das operações
- [ ] Seções analisadas (sem dados pessoais)
- [ ] Quantidade de rejeições encontradas

**Implementação:**
```go
// ❌ RUIM - expõe dados sensíveis
logger.Info("Rejeição encontrada", "quem", "João da Silva", "email", "joao@email.com")

// ✅ BOM - apenas metadados
logger.Info("Rejeição encontrada", "secao", "Partes Mecânicas", "total", 5, "execution_id", execID)
```

---

#### 3. Docker Container Security

**Non-root User:**
- [ ] Criar usuário dedicado `rpa` no Dockerfile
- [ ] Rodar container como usuário não-root
- [ ] Arquivos sensíveis com ownership para o usuário `rpa`
- [ ] Nunca rodar como root em produção

**Minimal Base Image:**
- [ ] Usar `alpine:latest` ou `distroless` como base final
- [ ] Remover ferramentas desnecessárias (curl, wget, etc)
- [ ] Instalar apenas dependências essenciais

**Implementação:**
```dockerfile
# Criar usuário não-root
RUN adduser -D -u 1000 rpa
USER rpa

# Permissões seguras
COPY --chown=rpa:rpa --from=builder /app/rpa /app/rpa
RUN chmod 600 /app/cookies.json  # Se existir
```

---

#### 4. File System Security (Segurança de Arquivos)

**Permissões:**
```bash
# cookies.json - apenas owner pode ler/escrita
chmod 600 cookies.json

# outputs/ - owner pode tudo, outros nada
chmod 700 outputs/

# Binário - executável mas não editável
chmod 500 /app/rpa
```

**Estrutura de Diretórios:**
```
/app/
├── rpa              # binário (500)
├── cookies.json     # sessão (600) - criado runtime
└── outputs/         # resultados (700)
    ├── rejeicoes_20240101_0900.json
    └── rejeicoes_20240101_1700.json
```

**Rotação de Logs:**
- [ ] Logs rotacionados automaticamente (lumberjack)
- [ ] Máximo 30 dias de retenção
- [ ] Arquivos antigos removidos automaticamente

---

#### 5. Network Security (Segurança de Rede)

**Comunicação HTTPS:**
- [ ] Todas as APIs externas usam HTTPS
- [ ] Verificar certificados TLS (não desabilitar validação)
- [ ] Usar TLS 1.2 ou superior

**Firewall (VPS):**
- [ ] Apenas portas necessárias abertas (22 para SSH)
- [ ] Bloquear tráfego de entrada desnecessário
- [ ] Se houver métricas HTTP, limitar acesso por IP

**DNS:**
- [ ] Usar DNS over HTTPS (opcional)
- [ ] Validar resolução DNS de domínios externos

---

#### 6. Input Validation (Validação de Entrada)

**URLs:**
- [ ] Validar formato de URLs antes de navegar
- [ ] Whitelist de domínios permitidos (hourglass.petrobras.com)
- [ ] Rejeitar URLs malformadas

**Dados Extraídos:**
- [ ] Sanitizar dados antes de salvar
- [ ] Limitar tamanho de strings (prevenir DoS)
- [ ] Validar encoding (UTF-8)

**Implementação:**
```go
func sanitizeInput(input string) string {
    // Limitar tamanho
    if len(input) > 1000 {
        input = input[:1000]
    }
    // Remover caracteres de controle
    return strings.Map(func(r rune) rune {
        if unicode.IsControl(r) {
            return -1
        }
        return r
    }, input)
}
```

---

#### 7. Error Handling Security

**Mensagens de Erro:**
- [ ] Nunca expor stack traces em produção
- [ ] Nunca logar variáveis de ambiente completas
- [ ] Mensagens genéricas para usuário final
- [ ] Detalhes técnicos apenas em logs (nível DEBUG)

**Panic Recovery:**
- [ ] Sempre usar `recover()` em goroutines
- [ ] Logar panic de forma segura (sem expor dados)
- [ ] Graceful degradation (continuar operando mesmo com erros)

**Implementação:**
```go
func safeOperation() (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("operation failed: internal error")
            logger.Error("panic recovered", "error", r, "stack", debug.Stack())
        }
    }()
    // ... operação arriscada
}
```

---

#### 8. Browser Automation Security

**Chromedp Security:**
- [ ] Usar `--no-sandbox` apenas em containers (não em dev local)
- [ ] Desabilitar devtools em produção
- [ ] Limitar recursos do browser (memory, CPU)
- [ ] Isolar contexto de navegação por sessão

**Cookies:**
- [ ] Respeitar flags HttpOnly, Secure, SameSite
- [ ] Nunca modificar cookies de forma insegura
- [ ] Validar domínio dos cookies antes de usar

**Timeout Protection:**
- [ ] Timeout em todas as operações (prevenir hangs)
- [ ] Context withTimeout para cada operação
- [ ] Max execution time por job (evitar runaway)

---

#### 9. Data Retention & Privacy

**Retenção de Dados:**
- [ ] Política de retenção: 30 dias para outputs
- [ ] Remoção automática de arquivos antigos
- [ ] Não armazenar dados em servidores de terceiros

**Conformidade:**
- [ ] Documentar uso de dados no README
- [ ] Implementar direito ao esquecimento (se solicitado)
- [ ] Anonimização de dados quando possível

---

#### 10. Dependency Security

**Go Modules:**
- [ ] Verificar vulnerabilidades: `govulncheck ./...`
- [ ] Manter dependências atualizadas
- [ ] Usar apenas bibliotecas bem mantidas
- [ ] Revisar licenças antes de adicionar

**Container Scanning:**
- [ ] Escanear imagem Docker: `trivy image hourglass-rpa`
- [ ] Usar imagens base mínimas (alpine)
- [ ] Atualizar base images regularmente

**CI/CD:**
- [ ] Verificar vulnerabilidades no pipeline
- [ ] Falhar build se vulnerabilidades críticas encontradas

---

#### 11. Monitoring & Alerting

**Security Events:**
- [ ] Alertar em tentativas de acesso não autorizado
- [ ] Monitorar falhas de autenticação excessivas
- [ ] Detectar padrões anormais de execução
- [ ] Alertar se cookies.json for acessado indevidamente

**Audit Trail:**
- [ ] Logar todas as operações de login/logout
- [ ] Registrar alterações de configuração
- [ ] Manter log de execuções (sucesso/falha)

---

#### 12. Incident Response

**Se houver vazamento de credenciais:**
1. **Imediato (primeiros 5 minutos):**
   - Revogar API keys expostas (Resend, Sentry, Grafana)
   - Invalidar sessão do Google
   - Remover commit do GitHub (force push ou git-filter-repo)

2. **Curto prazo (primeira hora):**
   - Gerar novas API keys
   - Atualizar cookies.json na VPS
   - Verificar logs para acesso não autorizado

3. **Longo prazo:**
   - Revisar políticas de segurança
   - Implementar verificações automáticas no CI
   - Documentar lições aprendidas

**Comunicação:**
- Notificar usuários afetados se dados pessoais vazaram
- Documentar incidente internamente

---

### ✅ Pre-Deployment Security Checklist

Antes de colocar em produção, verificar:

- [ ] Todas as env vars estão configuradas na VPS
- [ ] `cookies.json` foi transferido via SCP (não email)
- [ ] Permissões dos arquivos estão corretas (600, 700)
- [ ] Container roda como non-root
- [ ] Nenhuma credencial hardcoded no código
- [ ] `.gitignore` inclui todos os arquivos sensíveis
- [ ] Logs não expõem dados pessoais
- [ ] `govulncheck` não reporta vulnerabilidades
- [ ] Imagem Docker foi escaneada
- [ ] Firewall configurado na VPS
- [ ] Backups configurados (se necessário)
- [ ] Documentação de incident response pronta

---
---

### Must NOT Have (Guardrails - padrões AI-Slop a evitar)
- NÃO usar variáveis globais para contexto/dependências
- NÃO ignorar erros (nunca `_ = algumaCoisa()` sem tratamento)
- NÃO usar `time.Sleep()` fixo sem justificativa
- NÃO deixar processos Chrome zumbis (sempre cancelar contexto)
- NÃO expor credenciais em código
- NÃO usar `panic()` exceto em `main()`
- NÃO deixar testes E2E rodando no CI (usar build tags)
- NÃO prosseguir com implementação sem API Keys fornecidas pelo usuário
- **NUNCA commitar `cookies.json`, `.env`, ou arquivos com credenciais**
- NÃO usar variáveis globais para contexto/dependências
- NÃO ignorar erros (nunca `_ = algumaCoisa()` sem tratamento)
- NÃO usar `time.Sleep()` fixo sem justificativa
- NÃO deixar processos Chrome zumbis (sempre cancelar contexto)
- NÃO expor credenciais em código
- NÃO usar `panic()` exceto em `main()`
- NÃO deixar testes E2E rodando no CI (usar build tags)
- NÃO prosseguir com implementação sem API Keys fornecidas pelo usuário

### 🔐 API Keys Necessárias (Solicitar ao Usuário)

Durante a implementação, as seguintes API Keys serão necessárias:

| Serviço | Task | Quando Solicitar |
|---------|------|------------------|
| **Resend** | Task 17 (Notificador) | Antes de testar envio de email |
| **Sentry** | Task 19 (Error Tracking) | Antes de inicializar Sentry |
| **Grafana Cloud** | Task 20 (Métricas) | Antes de configurar remote_write |

> ⚠️ **IMPORTANTE**: O agente executor DEVE interromper a implementação e perguntar ao usuário no chat quando precisar de uma API Key. NÃO prossiga sem a chave fornecida.

---

## Estratégia de Verificação

### Decisão de Testes
- **Infraestrutura existe**: NO (novo projeto)
- **Testes automatizados**: TDD + Tests-after híbrido
- **Framework**: `testing` padrão Go + `testify` (assert/mock)
- **Estratégia**:
  - TDD para lógica de negócio (domain, scheduler)
  - Tests-after para integração com chromedp
  - E2E com `httptest.Server` para simular Hourglass

### Política de QA
Toda task inclui cenários de QA executáveis por agente:
- **Backend/CLI**: Bash (`go test`, `go build`, `go run`)
- **Docker**: Bash (`docker build`, `docker-compose up`)
- **CI**: Verificação de workflow no GitHub Actions

---

## Estratégia de Execução

### Ondas de Execução Paralela

```
Wave 1 (Fundação - pode iniciar imediatamente):
├── Task 1: Setup inicial do projeto (go.mod, estrutura de dirs) [quick]
├── Task 2: Domain models e interfaces (types.go, interfaces.go) [quick]
├── Task 3: Configuração via env vars (config.go) [quick]
└── Task 4: Storage JSON/CSV (storage/json.go) [quick]

Wave 2 (Core - depende Wave 1):
├── Task 5: Browser automation setup (rpa/browser.go) [unspecified-high]
├── Task 6: Login e persistência de cookies (rpa/login.go) [unspecified-high]
├── Task 7: Analisador de rejeições (rpa/analyzer.go) [unspecified-high]
└── Task 8: Scheduler cron (scheduler/scheduler.go) [quick]

Wave 3 (Integração - depende Wave 2):
├── Task 9: Main e wiring (main.go) [quick]
├── Task 10: Testes unitários (100% coverage) [unspecified-high]
├── Task 11: Testes E2E com httptest [unspecified-high]
└── Task 12: Makefile e scripts [quick]

Wave 4 (DevOps - depende Wave 3):
├── Task 13: Dockerfile multi-stage [quick]
├── Task 14: Docker Compose [quick]
├── Task 15: GitHub Actions CI/CD [quick]
└── Task 16: README e documentação [writing]

Wave FINAL (Verificação):
├── Task F1: Code quality review (gofmt, go vet, staticcheck)
├── Task F2: Coverage verification (100%)
├── Task F3: Docker build test
└── Task F4: Compliance check

Caminho Crítico: Task 1 → Task 2 → Task 5 → Task 6 → Task 7 → Task 9 → Task 10 → Task 13 → F1-F4
Speedup Paralelo: ~60% mais rápido que sequencial
Máximo Concorrente: 4 tarefas (Wave 1 e Wave 2)
```

### Matriz de Dependências

| Task | Depende de | Bloqueia |
|------|-----------|----------|
| 1 | - | 2, 3, 4, 5 |
| 2 | 1 | 5, 6, 7, 8 |
| 3 | 1 | 8, 9 |
| 4 | 1 | 7 |
| 5 | 1, 2 | 6, 7, 9 |
| 6 | 2, 5 | 7, 9 |
| 7 | 2, 4, 5, 6 | 9, 10 |
| 8 | 2, 3 | 9 |
| 9 | 3, 5, 6, 7, 8 | 10, 11 |
| 10 | 7, 8, 9 | 12 |
| 11 | 5, 6, 7, 9 | 12 |
| 12 | 10, 11 | 13, 14, 15 |
| 13 | 12 | F1-F4 |
| 14 | 12 | F1-F4 |
| 15 | 12 | F1-F4 |
| 16 | 15 | F1-F4 |
| F1-F4 | 13, 14, 15, 16 | - |

---

## TODOs

> Cada task inclui: implementação + testes (quando aplicável)
> Cada task tem cenários de QA obrigatórios

---

## Onda Final de Verificação

> 4 agentes de review em PARALELO. Todos devem APROVAR.

- [ ] **F1. Code Quality Review**
  - Executar: `gofmt -l .`, `go vet ./...`, `staticcheck ./...`
  - Verificar: ausência de `as any`, empty catches, `panic` indevido
  - Verificar: nomes descritivos (evitar `data`, `result`, `item`)
  - Output: `Lint [PASS/FAIL] | Issues [N]`

- [ ] **F2. Coverage Verification**
  - Executar: `go test -coverprofile=coverage.out ./...`
  - Verificar: `go tool cover -func=coverage.out` = 100%
  - Output: `Coverage [100%]`

- [ ] **F3. Docker Build Test**
  - Executar: `docker build -t hourglass-rpa:test .`
  - Executar: `docker run --rm hourglass-rpa:test --version`
  - Output: `Build [PASS/FAIL]`

- [ ] **F4. Compliance Check**
  - Verificar: todos os "Must Have" implementados
  - Verificar: nenhum "Must NOT Have" presente
  - Verificar: todos os commits na branch `main`
  - Output: `Compliant [YES/NO]`

---

## Estratégia de Commits

Padrão de commits (Conventional Commits):
- `feat(scope): descrição` - nova funcionalidade
- `fix(scope): descrição` - correção
- `test(scope): descrição` - testes
- `docs(scope): descrição` - documentação
- `chore(scope): descrição` - tarefas diversas
- `ci(scope): descrição` - CI/CD

Commits por task:
- Task 1: `chore(init): setup project structure and go.mod`
- Task 2: `feat(domain): add types and interfaces`
- Task 3: `feat(config): add environment configuration`
- Task 4: `feat(storage): add JSON/CSV storage implementation`
- Task 5: `feat(rpa): add browser automation setup`
- Task 6: `feat(rpa): add login and cookie persistence`
- Task 7: `feat(rpa): add rejection analyzer`
- Task 8: `feat(scheduler): add cron scheduler`
- Task 9: `feat(main): add entry point and wiring`
- Task 10: `test(all): add unit tests with 100% coverage`
- Task 11: `test(e2e): add end-to-end tests`
- Task 12: `chore(build): add Makefile`
- Task 13: `chore(docker): add Dockerfile`
- Task 14: `chore(docker): add docker-compose.yml`
- Task 15: `ci(github): add GitHub Actions workflow`
- Task 16: `docs(readme): add documentation`

---

## Critérios de Sucesso

### Comandos de Verificação

```bash
# Build
make build

# Testes com coverage
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total

# Lint
gofmt -l .
go vet ./...

# Docker
docker build -t hourglass-rpa:latest .
docker-compose up -d

# Execução manual (modo debug)
go run ./cmd/rpa --debug
```

### Checklist Final
- [ ] `go test ./...` passa
- [ ] Coverage = 100%
- [ ] `make build` gera binário
- [ ] `docker build` completa sem erros
- [ ] `docker-compose up` sobe serviço
- [ ] CI/CD passa no GitHub Actions
- [ ] Código commitado na branch `main`
- [ ] README documenta uso completo

---

## Notas de Arquitetura

### Clean Architecture Aplicada
```
cmd/rpa/
└── main.go              # Entry point, wiring

internal/
├── config/
│   └── config.go        # Env vars, config struct
├── domain/
│   ├── types.go         # Models (Rejeicao, JobResult)
│   └── interfaces.go    # Contracts (Scraper, Storage)
├── rpa/
│   ├── browser.go       # chromedp setup
│   ├── login.go         # auth + cookies
│   └── analyzer.go      # scraping logic
├── scheduler/
│   └── scheduler.go     # cron setup
└── storage/
    └── json.go          # file persistence
```

### Padrões de Código Go
- Interfaces em `domain/` para permitir mocking
- Injeção de dependências via construtores (`NewXXX()`)
- Context como primeiro parâmetro (`ctx context.Context`)
- Errors wrapping com `fmt.Errorf("...: %w", err)`
- Nunca ignorar errors (sempre tratar ou propagar)
- `defer cancel()` para limpeza de recursos

### Segurança
- Cookies em arquivo local (não commitar)
- Credenciais via env vars ou input interativo
- User-agent randômico para evitar detecção
- Timeout estrito em todas as operações

### Observabilidade
- `log/slog` com saída JSON
- `execution_id` por job para rastreabilidade
- Screenshot + HTML em caso de erro
- Métricas Prometheus (opcional)

### Docker Best Practices
- Multi-stage build (compilar em golang:alpine, copiar para alpine final)
- `dumb-init` como PID 1 para evitar zumbis
- Instalar `chromium`, `tzdata` na imagem final
- Não rodar como root (criar usuário `rpa`)
- **⚠️ IMPORTANTE**: NÃO usar `cron` dentro do container (ver problema acima)
- Usar `robfig/cron` no Go (aplicação gerencia próprio agendamento)
- Container roda como daemon (processo permanece vivo)
- Logs via `docker logs` (não arquivos internos)
- Multi-stage build (compilar em golang:alpine, copiar para alpine final)
- `dumb-init` como PID 1 para evitar zumbis
- Instalar `chromium`, `tzdata` na imagem final
- Não rodar como root (criar usuário `rpa`)


---



## TODOs



> Cada task inclui: implementação + testes (quando aplicável)
> Cada task tem cenários de QA obrigatórios

- [x] 1. Setup Inicial do Projeto

  **O que fazer**:
  - Inicializar módulo Go: `go mod init hourglass-rejeicoes-rpa`
  - Criar estrutura de diretórios: `cmd/rpa/`, `internal/config/`, `internal/domain/`, `internal/rpa/`, `internal/scheduler/`, `internal/storage/`, `internal/notifier/`, `internal/logger/`, `internal/sentry/`, `internal/metrics/`
  - Adicionar dependências de produção: `go get github.com/chromedp/chromedp github.com/chromedp/cdproto/runtime github.com/robfig/cron/v3 github.com/resend/resend-go/v2 github.com/getsentry/sentry-go github.com/prometheus/client_golang`
  - Adicionar dependências de desenvolvimento: `go get github.com/stretchr/testify go.uber.org/mock/mockgen`
  - Criar `.gitignore` com: `cookies.json`, `*.json`, `outputs/`, `*.log`, `.env`, `.env.local`
  - **Criar `.golangci.yml`** com configuração completa:
    ```yaml
    run:
      timeout: 5m
      go: '1.25'
    linters:
      enable:
        - staticcheck
        - revive
        - errcheck
        - gosec
        - ineffassign
        - misspell
        - gocognit
        - dupl
        - goconst
        - unconvert
        - unparam
        - nakedret
    linters-settings:
      gocognit:
        min-complexity: 15
    issues:
      exclude-use-default: false
    ```
  - **Instalar ferramentas de desenvolvimento** (documentar no README):
    ```bash
    go install mvdan.cc/gofumpt@latest
    go install golang.org/x/tools/cmd/goimports@latest
    go install golang.org/x/vuln/cmd/govulncheck@latest
    # golangci-lint via brew ou binário
    ```
  - Criar arquivo `.env.example` com todas as variáveis necessárias (sem valores reais)
  - Inicializar módulo Go: `go mod init hourglass-rejeicoes-rpa`
  - Criar estrutura de diretórios: `cmd/rpa/`, `internal/config/`, `internal/domain/`, `internal/rpa/`, `internal/scheduler/`, `internal/storage/`
  - Adicionar dependências: `go get github.com/chromedp/chromedp github.com/chromedp/cdproto/runtime github.com/robfig/cron/v3`
  - Criar `.gitignore` com: `cookies.json`, `*.json`, `outputs/`, `*.log`, `.env`
  - Corrigir `go.mod` existente (está com conteúdo corrompido)

  **NÃO fazer**:
  - Não criar código de negócio ainda
  - Não commitar credentials

  **Agente Recomendado**:
  - **Categoria**: `quick`
  - **Skills**: `git-master` (para setup inicial)

  **Paralelização**:
  - Pode executar imediatamente (sem dependências)
  - Bloqueia: Tasks 2, 3, 4, 5

  **Referências**:
  - `go mod init` documentation
  - Clean Architecture Go project structure

  **Critérios de Aceitação**:
  - [ ] `go mod tidy` executa sem erros
  - [ ] Estrutura de diretórios criada
  - [ ] `.gitignore` configurado
  - [ ] Dependências baixadas

  **QA**:
  ```bash
  # Verificar estrutura
  ls -la cmd/rpa internal/
  
  # Verificar go.mod
  cat go.mod | grep 'module hourglass'
  
  # Verificar dependências
  go mod tidy && echo 'OK'
  ```

  **Commit**: `chore(init): setup project structure and dependencies`

---

- [x] 2. Domain Models e Interfaces

  **O que fazer**:
  - Criar `internal/domain/types.go` com structs:
    ```go
    type Rejeicao struct {
        Secao     string    `json:"secao"`
        Quem      string    `json:"quem"`
        OQue      string    `json:"oque"`
        PraQuando string    `json:"pra_quando"`
        Timestamp time.Time `json:"timestamp"`
    }
    ```
  - Criar `internal/domain/interfaces.go` com interfaces `Scraper`, `Storage`, `Scheduler`
  - Criar `internal/domain/errors.go` com erros customizados

  **NÃO fazer**:
  - Não implementar as interfaces ainda
  - Não adicionar lógica de negócio

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 1
  - Pode executar em paralelo com: Tasks 3, 4
  - Bloqueia: Tasks 5, 6, 7, 8

  **Referências**:
  - Código de exemplo na seção "Arquitetura Detalhada"

  **Critérios de Aceitação**:
  - [ ] `go build ./internal/domain` compila sem erros
  - [ ] Interfaces definem todos os métodos necessários
  - [ ] Structs têm tags JSON corretas

  **QA**:
  ```bash
  go build ./internal/domain
  go vet ./internal/domain
  ```

  **Commit**: `feat(domain): add types and interfaces`

---

- [ ] 3. Configuração via Environment Variables

  **O que fazer**:
  - Criar `internal/config/config.go` usando `github.com/caarlos0/env` ou parsing manual
  - Definir struct Config com:
    - `HOURGLASS_URL` (default: https://hourglass.petrobras.com)
    - `COOKIE_FILE` (default: cookies.json)
    - `OUTPUT_DIR` (default: ./outputs)
    - `DEBUG` (default: false)
    - `SCHEDULE_MORNING` (default: "0 9 * * *")
    - `SCHEDULE_EVENING` (default: "0 17 * * *")
    - `TIMEOUT` (default: 60s)

  **NÃO fazer**:
  - Não hardcodear valores
  - Não validar ainda (faz na Task 9)

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 1
  - Pode executar em paralelo com: Tasks 2, 4
  - Bloqueia: Tasks 8, 9

  **Referências**:
  - `github.com/caarlos0/env/v11` para parsing de env vars

  **Critérios de Aceitação**:
  - [ ] Struct Config carrega de env vars
  - [ ] Valores default funcionam
  - [ ] Testes unitários cobrem parsing

  **QA**:
  ```bash
  go test ./internal/config -v
  HOURGLASS_URL=test.com go run ./cmd/rpa --dry-run 2>&1 | grep -q 'test.com' && echo 'OK'
  ```

  **Commit**: `feat(config): add environment configuration`

---

- [ ] 4. Storage JSON/CSV

  **O que fazer**:
  - Criar `internal/storage/json.go` implementando interface `Storage`
  - Métodos:
    - `Save(rejeicoes []Rejeicao) error` - salva JSON e CSV
    - `LoadCookies() ([]Cookie, error)` - carrega cookies do arquivo
    - `SaveCookies(cookies []Cookie) error` - salva cookies no arquivo
  - Nomear arquivos: `rejeicoes_YYYYMMDD_HHMM.json` e `.csv`

  **NÃO fazer**:
  - Não usar banco de dados (arquivos locais apenas)
  - Não complicar com rotação de logs

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 1
  - Pode executar em paralelo com: Tasks 2, 3
  - Bloqueia: Task 7

  **Referências**:
  - `encoding/json` e `encoding/csv` da stdlib

  **Critérios de Aceitação**:
  - [ ] Arquivos JSON gerados corretamente
  - [ ] Arquivos CSV gerados corretamente
  - [ ] Cookies salvos/carregados corretamente
  - [ ] Testes unitários com 100% coverage

  **QA**:
  ```bash
  go test ./internal/storage -cover
  go test ./internal/storage -coverprofile=coverage.out
  go tool cover -func=coverage.out | grep 'total:'
  ```

  **Commit**: `feat(storage): add JSON/CSV storage implementation`

---

- [ ] 5. Browser Automation Setup (chromedp)

  **O que fazer**:
  - Criar `internal/rpa/browser.go` com struct `Browser`
  - Implementar `NewBrowser(cfg *config.Config) *Browser`
  - Implementar `Setup(ctx context.Context) error`:
    - Configurar `chromedp.DefaultExecAllocatorOptions`
    - Adicionar flags: `--disable-dev-shm-usage`, `--disable-blink-features=AutomationControlled`
    - Configurar User-Agent realista
    - Suportar modo debug (headless=false quando DEBUG=true)
  - Implementar `Close() error` para cleanup

  **NÃO fazer**:
  - Não navegar ainda (só setup)
  - Não tratar cookies aqui (Task 6)

  **Agente Recomendado**:
  - **Categoria**: `unspecified-high`

  **Paralelização**:
  - Depende da: Task 1, 2
  - Pode executar em paralelo com: Task 6, 8
  - Bloqueia: Tasks 6, 7, 9

  **Referências**:
  - Código na seção "Arquitetura Detalhada"
  - `chromedp.DefaultExecAllocatorOptions`

  **Critérios de Aceitação**:
  - [ ] Browser inicializa sem erros
  - [ ] Modo debug funciona (visível)
  - [ ] Modo produção é headless
  - [ ] Cleanup libera recursos
  - [ ] Testes unitários com mocks

  **QA**:
  ```bash
  go test ./internal/rpa -run TestBrowser -v
  DEBUG=true go run ./cmd/rpa --dry-run 2>&1 | grep -q 'browser' && echo 'OK'
  ```

  **Commit**: `feat(rpa): add browser automation setup`

---

- [ ] 6. Login e Persistência de Cookies (Google OAuth)

  **⚠️ ESTRATÉGIA DE LOGIN DEFINIDA**:
  
  **Problema**: VPS é headless (sem interface gráfica). Não é possível abrir browser visual na VPS.
  
  **Solução**: Login Local → Copiar Cookies → VPS
  
  **Passos**:
  1. **Na máquina local** (com interface gráfica):
     ```bash
     # Rodar localmente para fazer login
     go run ./cmd/rpa --setup
     # ou
     docker run -it --rm -v $(pwd)/cookies.json:/app/cookies.json hourglass-rpa --setup
     ```
  2. **Login manual aparece**: Browser abre, usuário faz login no Google
  3. **Cookies são salvos** automaticamente em `cookies.json`
  4. **Copiar para VPS**: `scp cookies.json user@vps:/path/to/project/`
  5. **VPS usa cookies** copiados para autenticação nas execuções agendadas
  
  **O que fazer**:
  - Criar flag `--setup` no main.go para modo "primeira configuração"
  - No modo setup:
    - Detectar se estamos em ambiente com display (`DISPLAY` env var)
    - Se sim: abrir browser VISÍVEL (headless=false) para login
    - Se não: mostrar erro pedindo para rodar em máquina com interface
  - Após login bem-sucedido, salvar cookies em `cookies.json`
  - Implementar `loadCookies()` em `internal/rpa/login.go`
  - Na VPS, o container carrega cookies existentes automaticamente
  - Se cookies expirarem/inválidos: enviar alerta (email) pedindo reconfiguração

  **🔐 SEGURANÇA na Transferência:**
  - `cookies.json` contém sessão ativa do Google (SENSÍVEL!)
  - **NUNCA** enviar por email, WhatsApp, Slack, etc.
  - **SEMPRE** usar transferência criptografada:
    ```bash
    # Opção 1: SCP (SSH) - RECOMENDADO
    scp cookies.json user@sua-vps:/home/user/hourglass-rejeicoes-rpa/

    # Opção 2: rsync com SSH
    rsync -avz -e ssh cookies.json user@sua-vps:/home/user/hourglass-rejeicoes-rpa/
    ```
  - **Verificar** que cookies.json está no `.gitignore` antes de qualquer commit
  - **Após transferência**, verificar permissões: `chmod 600 cookies.json` na VPS
  
  **Implementação em `internal/rpa/login.go`**:
  - `SetupMode()` - detecta ambiente gráfico e abre browser visível
  - `PerformLogin()` - fluxo de login manual
  - `SaveCookies()` - salva cookies após login
  - `LoadCookies()` - carrega cookies existentes
  - `IsAuthenticated()` - verifica se sessão ainda é válida
  - Criar `internal/rpa/login.go`
  - Implementar `Login(ctx context.Context) error`:
    - Navegar para Hourglass URL
    - Clicar no botão "Google" (XPath: `//button[contains(text(),'Google')]`)
    - Aguardar 5s para redirect
    - Pausar para login manual: `fmt.Print("Pressione ENTER..."); bufio.NewReader(os.Stdin).ReadString('\n')`
    - Salvar cookies via `network.GetCookies()`
  - Implementar `loadCookies()` para restaurar sessão
  - Implementar `IsAuthenticated() bool` para verificar sessão válida

  **NÃO fazer**:
  - Não automatizar o login Google (muito complexo/quebradiço)
  - Não salvar credentials em texto plano

  **Agente Recomendado**:
  - **Categoria**: `unspecified-high`

  **Paralelização**:
  - Depende da: Task 2, 5
  - Pode executar em paralelo com: Task 8
  - Bloqueia: Tasks 7, 9

  **Referências**:
  - Pesquisa chromedp: `network.GetCookies()` e `network.SetCookie()`
  - Código na seção "Arquitetura Detalhada"

  **Critérios de Aceitação**:
  - [ ] Login manual funciona
  - [ ] Cookies salvos em arquivo
  - [ ] Cookies carregados na próxima execução
  - [ ] Verificação de autenticação funciona
  - [ ] Testes com `httptest.Server`

  **QA**:
  ```bash
  # Teste de integração
  go test ./internal/rpa -run TestLogin -v
  
  # Teste manual (requer browser)
  go run ./cmd/rpa --login-only
  ls -la cookies.json && echo 'Cookies saved'
  ```

  **Commit**: `feat(rpa): add Google OAuth login and cookie persistence`

---

- [ ] 7. Analisador de Rejeições (AG-Grid Scraping)

  **O que fazer**:
  - Criar `internal/rpa/analyzer.go`
  - Implementar `AnalyzeSection(ctx, secao string) (*JobResult, error)`:
    - Clicar na seção (Partes Mecânicas, Campo, Testemunho Público)
    - Aguardar `.ag-root` visível
    - Extrair ícones vermelhos: `.ag-cell *[title][style*="red"]`
    - Para cada ícone:
      - `title` → campo "Quem"
      - texto da célula → campo "OQue"
      - última coluna da linha → campo "PraQuando"
    - Retornar `JobResult` com slice de `Rejeicao`

  **NÃO fazer**:
  - Não usar Sleep fixo sem WaitVisible
  - Não ignorar erros de parsing

  **Agente Recomendado**:
  - **Categoria**: `unspecified-high`

  **Paralelização**:
  - Depende da: Task 2, 4, 5, 6
  - Bloqueia: Tasks 9, 10

  **Referências**:
  - Pesquisa chromedp: `chromedp.Nodes()`, `chromedp.AttributeValue()`
  - Código na seção "Arquitetura Detalhada"

  **Critérios de Aceitação**:
  - [ ] Navegação para seções funciona
  - [ ] Extração de dados correta
  - [ ] Todos os 3 campos populados
  - [ ] Timestamp adicionado
  - [ ] Testes com HTML de mock

  **QA**:
  ```bash
  go test ./internal/rpa -run TestAnalyzer -v
  go test ./internal/rpa -cover
  ```

  **Commit**: `feat(rpa): add rejection analyzer for AG-Grid`

---

- [ ] 8. Scheduler Cron (Agendamento)

  **O que fazer**:
  - Criar `internal/scheduler/scheduler.go`
  - Implementar interface `Scheduler`:
    - `New() *CronScheduler` com wrappers `Recover` e `SkipIfStillRunning`
    - `AddJob(spec string, job func()) error`
    - `Start() error` com graceful shutdown
    - `Stop() context.Context`
  - Suportar timezone via `CRON_TZ=` ou config
  - Agendamentos: 9h e 17h

  **NÃO fazer**:
  - Não usar goroutines próprias (deixar cron gerenciar)
  - Não ignorar sinais de shutdown

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 2, 3
  - Pode executar em paralelo com: Tasks 5, 6
  - Bloqueia: Task 9

  **Referências**:
  - Pesquisa cron/v3: `cron.New()`, `cron.WithChain()`
  - Código na seção "Arquitetura Detalhada"

  **Critérios de Aceitação**:
  - [ ] Cron inicia sem erros
  - [ ] Jobs agendados corretamente
  - [ ] Graceful shutdown funciona
  - [ ] Timezone respeitado
  - [ ] Testes unitários

  **QA**:
  ```bash
  go test ./internal/scheduler -v
  go test ./internal/scheduler -cover
  ```

  **Commit**: `feat(scheduler): add cron scheduler with graceful shutdown`

---

- [ ] 9. Main e Wiring

  **O que fazer**:
  - Criar `cmd/rpa/main.go` (único arquivo com `package main`)
  - Implementar função `main()`:
    - Carregar config: `config.Load()`
    - Criar storage: `storage.New(cfg)`
    - Criar browser: `rpa.NewBrowser(cfg)`
    - Setup browser: `browser.Setup(ctx)`
    - Verificar/Realizar login: `browser.Login()` ou carregar cookies
    - Criar scheduler: `scheduler.New()`
    - Adicionar jobs: 9h e 17h chamando `browser.AnalyzeSection()`
    - Iniciar scheduler: `scheduler.Start()`
  - Adicionar flag `-debug` para modo desenvolvimento
  - Adicionar flag `-once` para execução única (sem cron)

  **NÃO fazer**:
  - Não colocar lógica de negócio em main (apenas wiring)
  - Não acessar chromedp diretamente de main

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Tasks 3, 5, 6, 7, 8
  - Bloqueia: Tasks 10, 11

  **Referências**:
  - Clean Architecture: main é o único lugar com dependências

  **Critérios de Aceitação**:
  - [ ] `go build ./cmd/rpa` gera binário
  - [ ] Binário executa sem panic
  - [ ] Flags funcionam (--debug, --once)
  - [ ] Wiring correto (injeção de dependências)

  **QA**:
  ```bash
  go build -o rpa ./cmd/rpa
  ./rpa --help
  ./rpa --once --debug 2>&1 | head -20
  ```

  **Commit**: `feat(main): add entry point and wiring`

---

- [ ] 10. Testes Unitários (100% Coverage OBRIGATÓRIO)

  **⚠️ REQUISITO CRÍTICO**: O projeto NÃO será aprovado se não tiver 100% de cobertura em TODOS os arquivos da codebase.

  **O que fazer**:

  **O que fazer**:
  - Criar testes em cada pacote:
    - `internal/config/config_test.go`
    - `internal/domain/types_test.go`
    - `internal/storage/json_test.go`
    - `internal/rpa/browser_test.go` (com mocks)
    - `internal/rpa/login_test.go` (com httptest)
    - `internal/rpa/analyzer_test.go` (com HTML mock)
    - `internal/scheduler/scheduler_test.go`
  - Usar `testify/assert` e `testify/mock`
  - Criar mocks para interfaces:
    - `mocks/mock_scraper.go`
    - `mocks/mock_storage.go`
  - Garantir 100% coverage em todos os pacotes

  **NÃO fazer**:
  - Não testar chromedp diretamente (usar mocks)
  - Não deixar testes flaky

  **Agente Recomendado**:
  - **Categoria**: `unspecified-high`

  **Paralelização**:
  - Depende da: Tasks 7, 8, 9
  - Bloqueia: Task 12

  **Referências**:
  - `testify` library
  - `go test -cover`

  **Critérios de Aceitação OBRIGATÓRIOS**:
  - [ ] `go test ./...` passa (todos os testes)
  - [ ] Coverage 100% em **TODOS** os pacotes (sem exceções)
  - [ ] Nenhum arquivo sem teste (cobertura total)
  - [ ] Testes cobrem: happy path, erros, edge cases
  - [ ] Mocks gerados corretamente (gomock)
  - [ ] Testes não flaky (rodar 3x seguidas sem falhas)
  - [ ] `go test -race` não encontra race conditions
  
  **Verificação de Coverage**:
  ```bash
  # Deve mostrar 100% em TODOS os pacotes
  go test -race -coverprofile=coverage.out ./...
  go tool cover -func=coverage.out
  
  # Exemplo de saída esperada:
  # internal/config/config.go:100.0%
  # internal/domain/types.go:100.0%
  # internal/rpa/browser.go:100.0%
  # ... (todos os arquivos)
  # total: 100.0%
  ```
  - [ ] `go test ./...` passa
  - [ ] Coverage 100% em todos os pacotes
  - [ ] Mocks gerados corretamente
  - [ ] Testes não flaky

  **QA**:
  ```bash
  go test -race -coverprofile=coverage.out ./...
  go tool cover -func=coverage.out | grep total
  # Deve mostrar 100%
  ```

  **Commit**: `test(all): add unit tests with 100% coverage`

---

- [ ] 11. Testes E2E

  **O que fazer**:
  - Criar `e2e/e2e_test.go` com build tag: `//go:build e2e`
  - Subir `httptest.Server` com HTML simulando Hourglass
  - Testar fluxo completo: login → análise → salvamento
  - Usar chromedp real navegando no servidor local
  - Criar `e2e/fixtures/hourglass_mock.html` com AG-Grid

  **NÃO fazer**:
  - Não rodar E2E no CI por padrão (usar build tags)
  - Não depender de serviços externos

  **Agente Recomendado**:
  - **Categoria**: `unspecified-high`

  **Paralelização**:
  - Depende da: Tasks 5, 6, 7, 9
  - Bloqueia: Task 12

  **Referências**:
  - `httptest` package
  - Build tags: `//go:build e2e`

  **Critérios de Aceitação**:
  - [ ] Testes E2E passam localmente
  - [ ] `go test -tags=e2e ./e2e` funciona
  - [ ] HTML mock representa estrutura real do Hourglass

  **QA**:
  ```bash
  go test -tags=e2e -v ./e2e
  ```

  **Commit**: `test(e2e): add end-to-end tests`

---

- [ ] 12. Makefile

  **O que fazer** (TODOS os targets obrigatórios):
  - Criar `Makefile` com targets obrigatórios:
    
    ## Build e Execução
    - `build`: compila binário (`go build -o rpa ./cmd/rpa`)
    - `run`: executa aplicação (`go run ./cmd/rpa`)
    - `clean`: limpa build artifacts
    
    ## Testes (OBRIGATÓRIO: 100% coverage)
    - `test`: roda testes unitários (`go test -v -race ./...`)
    - `test-e2e`: roda testes E2E (`go test -tags=e2e -v ./e2e`)
    - `coverage`: gera relatório de cobertura
      ```
      go test -race -coverprofile=coverage.out ./...
      go tool cover -func=coverage.out
      go tool cover -html=coverage.out -o coverage.html
      ```
    - `coverage-check`: verifica se coverage é 100%
      ```
      @echo "Checking coverage..."
      @go test -coverprofile=coverage.out ./... 2>/dev/null
      @coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//')
      @if [ "$$(echo "$$coverage < 100" | bc)" -eq 1 ]; then \
        echo "❌ Coverage is $$coverage% (must be 100%)"; \
        exit 1; \
      else \
        echo "✅ Coverage is 100%"; \
      fi
      ```
    
    ## Formatação (OBRIGATÓRIO: gofumpt)
    - `fmt`: formata código com gofumpt (`gofumpt -l -w .`)
    - `fmt-check`: verifica se código está formatado
      ```
      @if [ -n "$$(gofumpt -l .)" ]; then \
        echo "❌ Code is not formatted. Run: make fmt"; \
        gofumpt -l .; \
        exit 1; \
      fi
      ```
    - `imports`: organiza imports com goimports (`goimports -w .`)
    
    ## Linting (OBRIGATÓRIO: zero issues)
    - `lint`: roda golangci-lint (`golangci-lint run ./...`)
    - `lint-fix`: tenta corrigir issues automaticamente (`golangci-lint run --fix ./...`)
    
    ## Segurança (OBRIGATÓRIO: zero vulnerabilidades)
    - `sec-check`: verifica vulnerabilidades (`govulncheck ./...`)
    
    ## Verificação Completa (OBRIGATÓRIO antes de commit)
    - `check`: roda TODAS as verificações obrigatórias
      ```
      make fmt-check
      make imports-check
      make lint
      make sec-check
      make test
      make coverage-check
      @echo "✅ All checks passed!"
      ```
    
    ## Dependências
    - `deps`: baixa dependências (`go mod download`)
    - `deps-update`: atualiza dependências (`go get -u ./...`)
    - `tidy`: limpa go.mod (`go mod tidy`)
    
    ## Mocks (para testes)
    - `generate`: gera mocks (`go generate ./...`)
    
  **⚠️ OBRIGATÓRIO**: O projeto NÃO está concluído se:
  - `make lint` encontrar qualquer issue
  - `make sec-check` encontrar vulnerabilidades
  - `make coverage-check` não for 100%
  - `make fmt-check` mostrar arquivos não formatados
  
  **CI/CD usará**: `make check` (que roda todas as verificações)
  - Criar `Makefile` com targets:
    - `build`: compila binário
    - `test`: roda testes unitários
    - `test-e2e`: roda testes E2E
    - `coverage`: gera relatório de cobertura
    - `run`: executa aplicação
    - `clean`: limpa build artifacts
    - `lint`: roda gofmt, go vet, staticcheck
    - `deps`: baixa dependências

  **NÃO fazer**:
  - Não complicar com targets desnecessários

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Tasks 10, 11
  - Bloqueia: Tasks 13, 14, 15

  **Referências**:
  - Makefile conventions

  **Critérios de Aceitação**:
  - [ ] `make build` funciona
  - [ ] `make test` funciona
  - [ ] `make coverage` funciona

  **QA**:
  ```bash
  make build
  make test
  make coverage
  ```

  **Commit**: `chore(build): add Makefile`

---

- [ ] 13. Dockerfile

  **O que fazer**:
  - Criar `Dockerfile` multi-stage para VPS com cronjob:
    - Stage 1 (builder): `golang:1.25-alpine`
      - Instalar git, ca-certificates
      - Copiar go.mod, go.sum
      - `go mod download`
      - Copiar código fonte
      - `go build -o rpa ./cmd/rpa`
    - Stage 2 (runtime): `alpine:latest`
      - Instalar: chromium, tzdata, ca-certificates, dumb-init, **cron**
      - Criar usuário não-root: `rpa`
      - Configurar crontab para rodar 9h e 17h:
        ```
        0 9 * * * /app/rpa --once >> /var/log/cron.log 2>&1
        0 17 * * * /app/rpa --once >> /var/log/cron.log 2>&1
  **O que fazer**:
  - ✅ **APROVADO PELA PESQUISA**: Usar `robfig/cron` (Go native) - NÃO usar cron no Docker
  - Criar `Dockerfile` multi-stage:
    - Stage 1 (builder): `golang:1.25-alpine`
      - Instalar git, ca-certificates
      - Copiar go.mod, go.sum
      - `go mod download`
      - Copiar código fonte
      - `go build -o rpa ./cmd/rpa`
    - Stage 2 (runtime): `alpine:latest`
      - Instalar: chromium, tzdata, ca-certificates, dumb-init
      - Criar usuário não-root: `rpa`
      - Copiar binário do stage 1
      - ENTRYPOINT: `["/usr/bin/dumb-init", "--", "./rpa"]`
      - CMD: vazio (aplicação gerencia próprio agendamento via robfig/cron)

  **Por que NÃO usar cron no Docker** (conforme pesquisa):
  - ❌ Problemas com environment variables (cron não herda ENV do Docker)
  - ❌ Logs perdidos (cron loga para /var/log/syslog, não para docker logs)
  - ❌ Processos zumbis (cron não gerencia lifecycle de child processes)
  - ❌ Complexidade desnecessária (wrappers scripts, redirecionamentos)

  **Por que robfig/cron no Go é melhor**:
  - ✅ Processo único permanece vivo (sem exit issues)
  - ✅ Logs unificados via `docker logs`
  - ✅ Environment variables funcionam nativamente
  - ✅ Built-in error handling e graceful shutdown
  - ✅ Usado em produção por AWS Copilot, Argo Workflows, DataDog
      - Script de entrypoint que inicia o cron:
        ```bash
        #!/bin/sh
        echo "Starting cron..."
        crond -f -l 2
        ```
      - ENTRYPOINT: `["/usr/bin/dumb-init", "--", "/app/start.sh"]`
      - CMD: vazio (cron gerencia a execução)

  **Nota sobre arquitetura**:
  - Container roda **permanentemente** na VPS (daemon)
  - Cron dentro do container dispara o RPA 2x ao dia
  - Logs via `docker logs` ou arquivo `/var/log/cron.log`
  - Ferramentas externas (Sentry, Grafana, Resend): SaaS via API Key
  - Criar `Dockerfile` multi-stage:
    - Stage 1 (builder): `golang:1.25-alpine`
      - Instalar git, ca-certificates
      - Copiar go.mod, go.sum
      - `go mod download`
      - Copiar código fonte
      - `go build -o rpa ./cmd/rpa`
    - Stage 2 (runtime): `alpine:latest`
      - Instalar: chromium, tzdata, ca-certificates, dumb-init
      - Criar usuário não-root: `rpa`
      - Copiar binário do stage 1
      - ENTRYPOINT: `["/usr/bin/dumb-init", "--"]`
      - CMD: `["./rpa"]`

  **NÃO fazer**:
  - Não usar imagem golang no runtime (muito grande)
  - Não rodar como root

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 12
  - Bloqueia: Tasks F1-F4

  **Referências**:
  - Docker multi-stage build best practices
  - `dumb-init` para evitar zumbis

  **Critérios de Aceitação**:
  - [ ] `docker build` completa sem erros
  - [ ] Imagem < 200MB
  - [ ] Container roda sem erros
  - [ ] Chrome headless funciona no container

  **QA**:
  ```bash
  docker build -t hourglass-rpa:test .
  docker run --rm hourglass-rpa:test --version
  docker images | grep hourglass-rpa
  ```

  **Commit**: `chore(docker): add Dockerfile`

---

- [ ] 14. Docker Compose

  **O que fazer**:
  - Criar `docker-compose.yml`:
    - Service `rpa`:
      - Build: contexto atual
      - Environment: todas as env vars do config
      - Volumes: `./outputs:/app/outputs`, `./cookies.json:/app/cookies.json`
      - Restart: `unless-stopped`
      - Cap_add (se necessário): `SYS_ADMIN`

  **NÃO fazer**:
  - Não expor portas desnecessárias

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 12
  - Bloqueia: Tasks F1-F4

  **Referências**:
  - Docker Compose syntax

  **Critérios de Aceitação**:
  - [ ] `docker-compose up -d` sobe serviço
  - [ ] Logs visíveis via `docker-compose logs`
  - [ ] Restart automático funciona

  **QA**:
  ```bash
  docker-compose up -d
  docker-compose ps
  docker-compose logs -f
  docker-compose down
  ```

  **Commit**: `chore(docker): add docker-compose.yml`

---

- [ ] 15. GitHub Actions CI/CD

  **O que fazer**:
  - Criar `.github/workflows/ci.yml`:
    - Trigger: push/pull_request na branch `main`
    - Jobs:
      - `test`: setup-go, go test, coverage report
      - `lint`: gofmt, go vet, staticcheck
      - `build`: compila binário
      - `docker`: build da imagem (sem push)
    - Usar `actions/setup-go@v5`
    - Usar `codecov/codecov-action` para upload de coverage

  **NÃO fazer**:
  - Não rodar E2E no CI (testes que precisam de browser)

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 12
  - Bloqueia: Tasks F1-F4

  **Referências**:
  - GitHub Actions documentation
  - `actions/setup-go`

  **Critérios de Aceitação**:
  - [ ] Workflow executa no push
  - [ ] Todos os jobs passam
  - [ ] Coverage report enviado para Codecov

  **QA**:
  ```bash
  # Verificar workflow localmente com act (opcional)
  act push
  ```

  **Commit**: `ci(github): add GitHub Actions workflow`

---

- [ ] 16. README e Documentação

  **O que fazer**:
  - Criar `README.md` profissional e completo
  - **ADICIONAR BADGES NO TOPO** (cópia exata do markdown abaixo):
    
    ```markdown
    # 🤖 Hourglass Rejeições RPA
    
    <!-- BUILD & CI -->
    [![CI](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/actions/workflows/ci.yml/badge.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/actions/workflows/ci.yml)
    [![Build Status](https://img.shields.io/badge/build-passing-brightgreen)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/actions)
    
    <!-- CODE QUALITY -->
    [![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen.svg?style=flat)](https://codecov.io/gh/SEU_USUARIO/hourglass-rejeicoes-rpa)
    [![Go Report Card](https://goreportcard.com/badge/github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)](https://goreportcard.com/report/github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    [![Code Quality](https://img.shields.io/badge/code%20quality-A-brightgreen.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    
    <!-- GO VERSION -->
    [![Go Version](https://img.shields.io/github/go-mod/go-version/SEU_USUARIO/hourglass-rejeicoes-rpa?logo=go)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/blob/main/go.mod)
    [![Go](https://img.shields.io/badge/go-1.25-blue?logo=go)](https://go.dev/)
    
    <!-- SECURITY -->
    [![Security Audit](https://img.shields.io/badge/security-audit-success-brightgreen)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/actions/workflows/security.yml)
    [![Vulnerabilities](https://img.shields.io/badge/vulnerabilities-0-brightgreen.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    
    <!-- DOCKER -->
    [![Docker](https://img.shields.io/badge/docker-ready-blue.svg?logo=docker)](https://hub.docker.com/r/SEU_USUARIO/hourglass-rejeicoes-rpa)
    [![Docker Image Size](https://img.shields.io/docker/image-size/SEU_USUARIO/hourglass-rejeicoes-rpa?logo=docker)](https://hub.docker.com/r/SEU_USUARIO/hourglass-rejeicoes-rpa)
    
    <!-- MAINTENANCE -->
    [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://choosealicense.com/licenses/mit/)
    [![Last Commit](https://img.shields.io/github/last-commit/SEU_USUARIO/hourglass-rejeicoes-rpa?style=flat-square)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/commits/main)
    [![Maintenance](https://img.shields.io/badge/Maintained%3F-yes-green.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/graphs/commit-activity)
    
    <!-- GITHUB STATS -->
    [![GitHub stars](https://img.shields.io/github/stars/SEU_USUARIO/hourglass-rejeicoes-rpa?style=social)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/stargazers)
    [![GitHub issues](https://img.shields.io/github/issues/SEU_USUARIO/hourglass-rejeicoes-rpa)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/issues)
    [![GitHub release](https://img.shields.io/github/v/release/SEU_USUARIO/hourglass-rejeicoes-rpa)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/releases)
    
    <!-- TECH STACK -->
    [![Chromedp](https://img.shields.io/badge/chromedp-latest-blue?logo=googlechrome)](https://github.com/chromedp/chromedp)
    [![GitHub Actions](https://img.shields.io/badge/GitHub%20Actions-2088FF?logo=githubactions&logoColor=white)](https://github.com/features/actions)
    ```
    
  **Estrutura do README** (seções obrigatórias):
  1. **Título** com emoji e descrição curta (1-2 linhas)
  2. **Badges** (código acima)
  3. **📋 Table of Contents** (índice com links)
  4. **🚀 Features** - Lista de funcionalidades principais
  5. **📦 Prerequisites** - Go 1.25.7, Docker (opcional)
  6. **🔧 Installation** - Passo a passo de instalação
  7. **⚙️ Configuration** - Variáveis de ambiente necessárias
  8. **🔐 First Login Setup** - Como fazer login inicial
  9. **🎯 Usage** - Exemplos de uso (comandos)
  10. **🐳 Docker Deployment** - Como rodar com Docker
  11. **🧪 Testing** - Como rodar testes (coverage 100%)
  12. **📁 Project Structure** - Árvore de diretórios
  13. **🔒 Security** - Notas de segurança importantes
  14. **🛡️ Security Best Practices** - Checklist de segurança
  15. **📄 License** - MIT License
  16. **👤 Author** - Seu nome e contato
  
  **Dicas de formatação:**
  - Usar emojis para facilitar navegação visual
  - Código em blocos com syntax highlighting
  - Tabelas para comparar opções
  - Screenshots ou GIFs (se aplicável)
  - Links clicáveis para tudo
  
  **O que NÃO esquecer:**
  - Instruções claras de como obter API keys
  - Exemplo de `.env` (sem valores reais)
  - Comandos copy-paste friendly
  - Troubleshooting section (problemas comuns)
  - Como contribuir (CONTRIBUTING.md opcional)
  - Criar `README.md` com seções completas (ver lista abaixo)
  - **Adicionar Badges no topo do README** (após o título):
    ```markdown
    <!-- Badges de Build e CI -->
    [![CI](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/workflows/CI/badge.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/actions)
    [![Go Version](https://img.shields.io/badge/go%20version-1.25.7-00ADD8?style=flat&logo=go)](https://golang.org)
    
    <!-- Badges de Qualidade -->
    [![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen.svg?style=flat)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/actions)
    [![Go Report Card](https://goreportcard.com/badge/github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)](https://goreportcard.com/report/github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    [![Code Quality](https://img.shields.io/badge/code%20quality-A-brightgreen.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    
    <!-- Badges de Segurança -->
    [![Security](https://img.shields.io/badge/security-passing-brightgreen.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    [![Vulnerabilities](https://img.shields.io/badge/vulnerabilities-0-brightgreen.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    
    <!-- Badges de Docker -->
    [![Docker](https://img.shields.io/badge/docker-ready-blue.svg?logo=docker)](https://hub.docker.com/r/SEU_USUARIO/hourglass-repa)
    
    <!-- Badges de Manutenção -->
    [![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
    [![Last Commit](https://img.shields.io/github/last-commit/SEU_USUARIO/hourglass-rejeicoes-rpa.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/commits/main)
    [![Maintenance](https://img.shields.io/badge/maintained-yes-green.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/graphs/commit-activity)
    
    <!-- Badge de Tamanho -->
    [![Repo Size](https://img.shields.io/github/repo-size/SEU_USUARIO/hourglass-rejeicoes-rpa.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    [![Code Size](https://img.shields.io/github/languages/code-size/SEU_USUARIO/hourglass-rejeicoes-rpa.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    
    <!-- Badge de Linguagem -->
    [![Go](https://img.shields.io/github/languages/top/SEU_USUARIO/hourglass-rejeicoes-rpa.svg?color=00ADD8&logo=go)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa)
    
    <!-- Badges de Issues e PRs -->
    [![Issues](https://img.shields.io/github/issues/SEU_USUARIO/hourglass-rejeicoes-rpa.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/issues)
    [![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/pulls)
    
    <!-- Badge de Versão -->
    [![Version](https://img.shields.io/badge/version-1.0.0-blue.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/releases)
    
    <!-- Badge de Release -->
    [![GitHub release](https://img.shields.io/github/release/SEU_USUARIO/hourglass-rejeicoes-rpa.svg)](https://github.com/SEU_USUARIO/hourglass-rejeicoes-rpa/releases)
    ```
  - **Nota**: Substituir `SEU_USUARIO` pelo seu username do GitHub
  - Criar estrutura completa do README:
    - Título com emoji (ex: 🤖 Hourglass Rejeições RPA)
    - Badges (acima)
    - Descrição curta do projeto
    - 📋 Índice (Table of Contents)
    - 🚀 Features
    - 📦 Pré-requisitos
    - 🔧 Instalação
    - ⚙️ Configuração
    - 🎯 Uso
    - 🔐 Primeiro Login (Setup)
    - 📊 Output (JSON/CSV)
    - 🐳 Docker
    - 🧪 Testes
    - 📁 Estrutura do Projeto
    - 🔒 Segurança
    - 🤝 Contribuição
    - 📄 Licença
    - 👤 Autor
    - 🙏 Agradecimentos
  - Adicionar screenshots/gifs demonstrativos (se possível)
  - Incluir exemplos de código
  - Documentar troubleshooting comum
  - Criar `README.md` com:
    - Descrição do projeto
    - Requisitos (Go 1.25.7, Docker)
    - Instalação: `git clone`, `go mod download`
    - Configuração: variáveis de ambiente
    - Uso: `make build`, `make run`
    - Docker: `docker-compose up`
    - Estrutura do projeto
    - Primeiro login (modo manual)
    - Agendamento (cron)
    - Saída (JSON/CSV)
    - Contribuição
    - Licença

  **NÃO fazer**:
  - Não deixar sem documentação de setup

  **Agente Recomendado**:
  - **Categoria**: `writing`

  **Paralelização**:
  - Depende da: Task 15
  - Bloqueia: Tasks F1-F4

  **Referências**:
  - README templates

  **Critérios de Aceitação**:
  - [ ] README completo e claro
  - [ ] Instruções de setup funcionam
  - [ ] Exemplos de uso incluídos

  **QA**:
  ```bash
  # Testar instruções do README
  git clone <repo> /tmp/test
  cd /tmp/test && go mod download && go build ./cmd/rpa
  ```

  **Commit**: `docs(readme): add documentation`

---

- [ ] 17. Notificações por Email (Resend)

  ⚠️ **BLOQUEADO até receber API Key**
  - Solicitar ao usuário: `RESEND_API_KEY`
  - Perguntar no chat antes de prosseguir

  **O que fazer**:
  - Criar `internal/notifier/notifier.go` com interface `Notifier`
  - Criar `internal/notifier/resend.go` implementando `Notifier`:
    - `github.com/resend/resend-go/v2`
    - `NewResendClient(apiKey, fromEmail string) (*ResendClient, error)`
    - `SendJobCompletion(summary string, duration time.Duration) error`
    - `SendJobFailure(step string, err error) error`
    - `SendDailyReport(stats DailyStats) error`
  - Templates HTML para emails bonitos
  - Configuração via env vars:
    - `RESEND_API_KEY` (solicitar ao usuário)
    - `EMAIL_FROM` (ex: onboarding@seudominio.com)
    - `EMAIL_TO` (default: `4drade@gmail.com` ← email do usuário para testes)
  - **Email para testes**: `4drade@gmail.com`

  **O que fazer**:
  - Criar `internal/notifier/notifier.go` com interface `Notifier`
  - Criar `internal/notifier/resend.go` implementando `Notifier`:
    - `github.com/resend/resend-go/v2`
    - `NewResendClient(apiKey, fromEmail string) (*ResendClient, error)`
    - `SendJobCompletion(summary string, duration time.Duration) error`
    - `SendJobFailure(step string, err error) error`
    - `SendDailyReport(stats DailyStats) error`
  - Templates HTML para emails bonitos
  - Configuração via env vars:
    - `RESEND_API_KEY`
    - `EMAIL_FROM`
    - `EMAIL_TO`

  **NÃO fazer**:
  - Não expor API key em logs
  - Não enviar emails em modo dry-run

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 2
  - Pode executar em paralelo com: Tasks 3-16
  - Bloqueia: Task 9 (wiring)

  **Referências**:
  - `github.com/resend/resend-go/v2`
  - Resend docs: https://resend.com/docs

  **Critérios de Aceitação**:
  - [ ] SDK Resend integrado
  - [ ] Emails HTML enviados corretamente
  - [ ] Templates responsivos
  - [ ] Testes unitários com mocks

  **QA**:
  ```bash
  go test ./internal/notifier -v
  go test ./internal/notifier -cover
  ```

  **Commit**: `feat(notifier): add Resend email notifications`

---

- [ ] 18. Logging Estruturado (slog + lumberjack)

  **O que fazer**:
  - Usar `log/slog` da standard library (Go 1.21+)
  - Adicionar `gopkg.in/natefinch/lumberjack.v3` para rotação de logs
  - Criar `internal/logger/logger.go`:
    - `NewLogger(debug bool, logFile string) *slog.Logger`
    - TextHandler colorido para desenvolvimento
    - JSONHandler para produção (machine-readable)
    - Lumberjack para rotação: 5MB max, 7 backups, 30 dias retenção
  - Substituir todos os `fmt.Print` e `log.Print` pelo logger
  - Adicionar contexto estruturado:
    - `execution_id` para rastreabilidade
    - `secao` nas operações de análise
    - `duration` nos logs de timing
  - Integração com Sentry: erros ERROR/FATAL capturados automaticamente
  - Adicionar `github.com/charmbracelet/log` ao projeto
  - Criar `internal/logger/logger.go`:
    - `NewLogger(debug bool) *log.Logger`
    - Configurar cores e formatação para desenvolvimento
    - JSON output para produção (via flag ou env)
  - Substituir todos os `fmt.Print` e `log.Print` pelo logger
  - Adicionar contexto estruturado:
    - `execution_id` para rastreabilidade
    - `secao` nas operações de análise
    - `duration` nos logs de timing

  **NÃO fazer**:
  - Não usar logs em hot paths sem nível apropriado
  - Não logar dados sensíveis (cookies, tokens)

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 1
  - Pode executar em paralelo com: Tasks 2-17
  - Bloqueia: Todas as tasks que usam logging

  **Referências**:
  - `log/slog` - standard library Go 1.21+
  - `gopkg.in/natefinch/lumberjack.v3` - rotação de logs
  - Código exemplo:
    ```go
    import (
        "log/slog"
        "os"
        "gopkg.in/natefinch/lumberjack.v3"
    )
    
    func setupLogger() *slog.Logger {
        lumber := &lumberjack.Logger{
            Filename:   "/var/log/rpa/hourglass.log",
            MaxSize:    5,  // MB
            MaxBackups: 7,
            MaxAge:     30, // dias
        }
        handler := slog.NewJSONHandler(lumber, &slog.HandlerOptions{
            Level: slog.LevelInfo,
        })
        return slog.New(handler)
    }
    
    // Uso
    logger.Info("Cron run iniciada", "secao", "Partes Mecânicas")
    logger.Error("Login falhou", "error", err, "tentativa", 2)
    ```
  - `github.com/charmbracelet/log` - logs coloridos e bonitos
  - Pesquisa: Charmbracelet/log venceu em beleza

  **Critérios de Aceitação**:
  - [ ] Logs coloridos em desenvolvimento
  - [ ] Logs JSON em produção
  - [ ] Níveis de log respeitados (Debug, Info, Error)
  - [ ] Campos estruturados em todos os logs

  **QA**:
  ```bash
  tail -f /var/log/rpa/hourglass.log | jq
  DEBUG=true go run ./cmd/rpa 2>&1 | head -20
  go test ./internal/logger -v
  ```
  ```bash
  DEBUG=true go run ./cmd/rpa 2>&1 | head -20
  # Deve mostrar logs coloridos
  ```

  **Commit**: `feat(logger): add structured logging with slog and lumberjack`

---

- [ ] 19. Error Tracking com Sentry

  ⚠️ **BLOQUEADO até receber DSN**
  - Solicitar ao usuário: `SENTRY_DSN`
  - Perguntar no chat antes de prosseguir

  **O que fazer**:

  **O que fazer**:
  - Adicionar `github.com/getsentry/sentry-go` ao projeto
  - Criar `internal/sentry/sentry.go`:
    - `Init(dsn string, environment string) error` - inicializa Sentry
    - `CaptureError(err error, tags map[string]string)` - captura erros
    - `AddBreadcrumb(category, message string)` - adiciona breadcrumbs
    - `Flush(timeout time.Duration)` - garante envio antes de sair
  - Integrar com logger: erros ERROR/FATAL vão para Sentry automaticamente
  - Adicionar contexto aos erros:
    - `secao` - qual seção estava sendo analisada
    - `execution_id` - rastreabilidade
    - `duration` - tempo até o erro
  - Configuração via env vars:
    - `SENTRY_DSN` (obrigatório para ativar)
    - `SENTRY_ENVIRONMENT` (default: production)
    - `SENTRY_SAMPLE_RATE` (default: 1.0)

  **NÃO fazer**:
  - Não enviar erros em modo DEBUG
  - Não enviar dados sensíveis (cookies, tokens, senhas)
  - Não usar Sentry para logs comuns (apenas erros/exceções)

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 1, 18 (logger)
  - Pode executar em paralelo com: Tasks 2-17
  - Bloqueia: Tasks que precisam de error tracking

  **Referências**:
  - `github.com/getsentry/sentry-go`
  - Sentry Go Docs: https://docs.sentry.io/platforms/go/
  - Free tier: 5,000 errors/mês
  - Código exemplo:
    ```go
    sentry.Init(sentry.ClientOptions{
        Dsn: os.Getenv("SENTRY_DSN"),
        Environment: os.Getenv("SENTRY_ENVIRONMENT"),
    })
    defer sentry.Flush(2 * time.Second)
    
    sentry.CaptureException(err)
    ```

  **Critérios de Aceitação**:
  - [ ] Sentry inicializa sem erros quando DSN configurado
  - [ ] Erros capturados aparecem no dashboard Sentry
  - [ ] Tags e contexto enrichados nos erros
  - [ ] Breadcrumbs ajudam a rastrear fluxo
  - [ ] Testes unitários com Sentry desativado

  **QA**:
  ```bash
  SENTRY_DSN=test go test ./internal/sentry -v
  go test ./internal/sentry -cover
  ```

  **Commit**: `feat(sentry): add error tracking and monitoring`

---

- [ ] 20. Métricas com Grafana Cloud

  ⚠️ **BLOQUEADO até receber credenciais**
  - Solicitar ao usuário: `GRAFANA_CLOUD_USER` e `GRAFANA_CLOUD_API_KEY`
  - Perguntar no chat antes de prosseguir

  **O que fazer**:

  **O que fazer**:
  - Adicionar `github.com/prometheus/client_golang` ao projeto
  - Criar `internal/metrics/metrics.go`:
    - `Init()` - inicializa registry e métricas
    - Expor métricas HTTP em `/metrics` (para Grafana Agent)
    - Métricas customizadas para RPA:
      - `rpa_jobs_total` (Counter) - total de jobs executados
      - `rpa_jobs_failed` (Counter) - jobs que falharam
      - `rpa_rejeicoes_found` (Counter) - rejeições encontradas por seção
      - `rpa_last_run_timestamp` (Gauge) - timestamp da última execução
      - `rpa_job_duration_seconds` (Histogram) - duração dos jobs
      - `rpa_scrape_duration_seconds` (Histogram) - tempo de scraping
  - Configuração via env vars:
    - `METRICS_ENABLED` (default: true)
    - `METRICS_PORT` (default: 9090)
    - `GRAFANA_CLOUD_USER` (para remote_write)
    - `GRAFANA_CLOUD_API_KEY` (para remote_write)
  - Implementar pusher para Grafana Cloud (opcional):
    - Usar `github.com/prometheus/client_golang/prometheus/push`
    - Push gateway ou remote_write direto

  **NÃO fazer**:
  - Não expor métricas em produção sem autenticação
  - Não enviar métricas sensíveis (dados pessoais)
  - Não usar labels com alta cardinalidade (ex: timestamps)

  **Agente Recomendado**:
  - **Categoria**: `quick`

  **Paralelização**:
  - Depende da: Task 1
  - Pode executar em paralelo com: Tasks 2-19
  - Bloqueia: Tasks que precisam de métricas

  **Referências**:
  - `github.com/prometheus/client_golang`
  - Grafana Cloud Docs: https://grafana.com/docs/grafana-cloud/
  - Free tier: 10,000 métricas ativas, 30 dias retenção
  - Código exemplo:
    ```go
    import (
        "github.com/prometheus/client_golang/prometheus"
        "github.com/prometheus/client_golang/prometheus/promhttp"
    )
    
    var jobsCompleted = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "rpa_jobs_completed_total",
            Help: "Total de jobs completados",
        },
        []string{"secao", "status"},
    )
    
    // Incrementar
    jobsCompleted.WithLabelValues("Partes Mecânicas", "success").Inc()
    ```

  **Critérios de Aceitação**:
  - [ ] Métricas expostas em `/metrics`
  - [ ] Todas as métricas RPA implementadas
  - [ ] Labels apropriados (secao, status)
  - [ ] Integração com Grafana Cloud funcionando
  - [ ] Dashboard básico configurado

  **QA**:
  ```bash
  curl http://localhost:9090/metrics | grep rpa_
  go test ./internal/metrics -v
  go test ./internal/metrics -cover
  ```

  **Commit**: `feat(metrics): add Grafana Cloud metrics export`

---

---

## Onda Final de Verificação (Pós-Implementação)

> Executar APÓS todas as tasks 1-20 estarem completas

- [ ] F1. Verificação de Arquitetura
  - Verificar: cada arquivo tem UMA responsabilidade única
  - Verificar: não há mistura de concerns (domain não importa chromedp)
  - Verificar: interfaces estão no pacote correto (domain)
  - Verificar: injeção de dependências via construtores

- [ ] F2. Verificação de Qualidade de Código (OBRIGATÓRIO: zero issues)
  **⚠️ O projeto NÃO está concluído se QUALQUER verificação falhar!**

  ### Formatação (gofumpt - mais estrito que gofmt)
  - Executar: `gofumpt -l .`
  - **Deve retornar vazio** (senão: rodar `gofumpt -w .`)
  - Executar: `goimports -l .`
  - **Deve retornar vazio** (todos os imports organizados)

  ### Linting (golangci-lint - TODOS os linters habilitados)
  - Executar: `golangci-lint run ./...`
  - Linters obrigatórios (zero issues tolerados):
    - `staticcheck` - análise estática avançada
    - `revive` - regras de estilo e boas práticas
    - `errcheck` - tratamento explícito de erros
    - `gosec` - vulnerabilidades de segurança
    - `ineffassign` - atribuições nunca lidas
    - `misspell` - erros de digitação
    - `gocognit` - complexidade cognitiva
    - `dupl` - código duplicado
    - `goconst` - strings que deveriam ser constantes
    - `unconvert` - conversões desnecessárias
    - `unparam` - parâmetros não variáveis
    - `nakedret` - returns nus em funções longas
  - **Resultado esperado**: `0 issues` (qualquer issue = FALHA)

  ### Segurança de Dependências (govulncheck)
  - Executar: `govulncheck ./...`
  - **Deve retornar**: "No vulnerabilities found"
  - Se encontrar vulnerabilidades: atualizar dependências

  ### Qualidade de Código Manual
  - Verificar: sem `panic()` exceto em `main()`
  - Verificar: sem variáveis globais
  - Verificar: todos os errors tratados (nunca `_ =`)
  - Verificar: nenhuma credencial hardcoded
  - Verificar: todos os comentários de exported items

  ### Comando Único de Verificação
  ```bash
  make check  # Roda TODAS as verificações acima
  ```
  **Se `make check` falhar, o projeto NÃO está concluído!**

- [ ] F3. Verificação de Coverage 100% (OBRIGATÓRIO)
  **⚠️ O projeto NÃO está concluído se coverage < 100%!**

  ### Comandos de Verificação
  ```bash
  # Gerar relatório de cobertura
  go test -race -coverprofile=coverage.out ./...
  
  # Verificar coverage por arquivo
  go tool cover -func=coverage.out
  ```

  ### Critérios de Aceitação OBRIGATÓRIOS
  - [ ] **100% em TODOS os arquivos** (não apenas o total)
  - [ ] Nenhum arquivo com coverage < 100%
  - [ ] Todos os pacotes testados: internal/config, internal/domain, internal/rpa, internal/scheduler, internal/storage, internal/notifier, internal/logger, internal/sentry, internal/metrics
  - [ ] Happy path testado
  - [ ] Paths de erro testados
  - [ ] Edge cases testados
  
  ### Comando de Verificação Automática
  ```bash
  make coverage-check
  # ou
  ./scripts/check-coverage.sh  # Deve retornar 0 (sucesso)
  ```
  
  ### Saída Esperada
  ```
  internal/config/config.go:100.0%
  internal/domain/types.go:100.0%
  internal/domain/interfaces.go:100.0%
  internal/rpa/browser.go:100.0%
  internal/rpa/login.go:100.0%
  internal/rpa/analyzer.go:100.0%
  internal/scheduler/scheduler.go:100.0%
  internal/storage/json.go:100.0%
  internal/notifier/resend.go:100.0%
  internal/logger/logger.go:100.0%
  internal/sentry/sentry.go:100.0%
  internal/metrics/metrics.go:100.0%
  ...
  total: 100.0%
  ✅ Coverage check passed: 100%
  ```
  
  **Se qualquer arquivo estiver abaixo de 100%, o projeto NÃO está concluído!**
  - Executar: `go vet ./...` (sem warnings)
  - Executar: `staticcheck ./...` (sem issues)
  - Verificar: sem `panic()` exceto em main
  - Verificar: sem variáveis globais
  - Verificar: todos os errors tratados

- [ ] F3. Verificação de Coverage 100%
  - Executar: `go test -race -coverprofile=coverage.out ./...`
  - Executar: `go tool cover -func=coverage.out`
  - Verificar: todas as funções cobertas
  - Verificar: paths de erro também cobertos

- [ ] F4. Verificação Docker
  - Executar: `docker build -t hourglass-rpa:test .`
  - Executar: `docker run --rm hourglass-rpa:test`
  - Verificar: imagem usa dumb-init
  - Verificar: não roda como root
  - Verificar: chrome headless funciona

- [ ] F5. Verificação de Documentação
  - Verificar: README.md existe e está completo
  - Verificar: instruções de setup funcionam
  - Verificar: apenas README.md existe (sem outros .md)
  - Verificar: comentários de código claros onde necessário

- [ ] F6. Verificação Git
  - Verificar: todos os commits na branch `main`
  - Verificar: mensagens seguem Conventional Commits
  - Verificar: `.gitignore` configurado corretamente


---

## ✅ Critérios de Conclusão OBRIGATÓRIOS

**O projeto só será considerado CONCLUÍDO quando TODOS os critérios abaixo forem atendidos:**

### 🔴 Requisitos Bloqueantes (ZERO tolerância)

| # | Requisito | Ferramenta | Comando de Verificação |
|---|-----------|------------|------------------------|
| 1 | **Zero issues de linting** | golangci-lint | `golangci-lint run ./...` |
| 2 | **Zero vulnerabilidades** | govulncheck | `govulncheck ./...` |
| 3 | **100% test coverage** | go test | `go tool cover -func=coverage.out` |
| 4 | **Código formatado** | gofumpt | `gofumpt -l .` (vazio) |
| 5 | **Imports organizados** | goimports | `goimports -l .` (vazio) |
| 6 | **Sem race conditions** | go test -race | `go test -race ./...` |
| 7 | **Zero erros de compilação** | go build | `go build ./...` |
| 8 | **Docker build funciona** | docker | `docker build -t hourglass-rpa .` |
| 9 | **Testes unitários passam** | go test | `go test ./...` |
| 10 | **Testes E2E passam** | go test | `go test -tags=e2e ./e2e` |

### 📋 Checklist Final (Obrigatório)

```bash
# Comando único para verificar TUDO
make check

# Se o comando acima passar, o projeto está aprovado!
```

### ❌ Se QUALQUER um falhar:

- 🚫 **Projeto NÃO está concluído**
- 🚫 **Não fazer deploy**
- 🚫 **Não mergear para main**
- ✅ **Corrigir e rodar novamente**

### 📊 Exemplo de Saída Aprovada:

```
$ make check
✅ gofumpt check passed
✅ goimports check passed
✅ golangci-lint passed (0 issues)
✅ govulncheck passed (0 vulnerabilities)
✅ go test passed
✅ coverage check passed: 100.0%
✅ All checks passed!
```

---


## Resumo de Decisões Técnicas

### Stack Final
- **Go**: 1.25.7 (versão disponível na máquina)
- **Browser Automation**: chromedp (chromedp/chromedp)
- **Agendamento**: robfig/cron/v3
- **Email**: Resend (resend/resend-go/v2)
- **Logging**: slog (std lib) + lumberjack (rotação)
- **Error Tracking**: Sentry (getsentry/sentry-go)
- **Metrics**: Grafana Cloud (prometheus/client_golang)
- **Config**: env vars (padrão Go)

### 💰 Custo Total: $0 (Free Tiers Only)

Todas as ferramentas selecionadas têm **Free Tiers suficientes** para o seu uso:

| Ferramenta | Free Tier | Seu Uso | % Utilizado |
|------------|-----------|---------|-------------|
| **Resend** | 3,000 emails/mês | ~60 emails | **2%** ✅ |
| **Sentry** | 5,000 errors/mês | ~0-10 errors | **<1%** ✅ |
| **Grafana Cloud** | 10,000 métricas | ~10-20 métricas | **<1%** ✅ |
| **GitHub Actions** | 2,000 minutos/mês | ~100 minutos | **5%** ✅ |

**Total mensal: $0**

### 🎨 Badges do README

O README incluirá **15+ badges** organizados por categoria:

| Categoria | Badges |
|-----------|--------|
| **Build/CI** | CI Status, Go Version |
| **Qualidade** | Coverage (100%), Go Report Card, Code Quality |
| **Segurança** | Security Status, Vulnerabilities (0) |
| **Docker** | Docker Ready |
| **Manutenção** | License (MIT), Last Commit, Maintenance |
| **Tamanho** | Repo Size, Code Size |
| **Linguagem** | Go (primary) |
| **Issues/PRs** | Open Issues, PRs Welcome |
| **Versão** | Version, GitHub Release |

**Exemplo visual:**
```markdown
[![CI](https://github.com/USER/REPO/workflows/CI/badge.svg)]
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen.svg)]
[![Go Report Card](https://goreportcard.com/badge/...)]
[![License](https://img.shields.io/badge/license-MIT-blue.svg)]
```

> **Nota**: Badges usam shields.io e integrations nativas do GitHub
> ⚠️ **Nota**: Se algum dia precisar de mais recursos, alternativas 100% gratuitas/self-hosted estão documentadas no apêndice.

### 🏗️ Decisões Arquiteturais Importantes

#### 1. Agendamento: robfig/cron (Go Native) ✅

**Decisão**: Usar `robfig/cron` biblioteca Go ao invés de `cron` no Docker.

**Por quê** (baseado em pesquisa comparativa):
- Docker + cron interno tem problemas bem documentados:
  - ❌ Environment variables não são herdadas pelo cron
  - ❌ Logs vão para /var/log/syslog (não capturados por `docker logs`)
  - ❌ Processos zumbis (cron não gerencia child processes)
  - ❌ Complexidade com wrappers scripts
- robfig/cron no Go é usado em produção por AWS Copilot, Argo Workflows, DataDog
- Logs unificados, graceful shutdown builtin, sem processos zumbis

**Implementação**: Container Docker roda como daemon permanente, aplicação Go gerencia próprio agendamento interno via robfig/cron (9h e 17h).

#### 2. Ferramentas Externas: SaaS via API Key

**Decisão**: Todas as ferramentas de monitoramento (Sentry, Grafana, Resend) são SaaS acessadas via API Key.

**Por quê**:
- Nenhuma precisa rodar localhost/self-hosted
- Free tiers suficientes para o uso (~60 execuções/mês)
- Menos manutenção operacional
- Arquitetura: VPS (Docker) → APIs externas (SaaS)

#### 3. Container Docker: Daemon Permanente

**Decisão**: Container roda 24/7 na VPS (não one-shot).

**Por quê**:
- robfig/cron precisa do processo rodando para agendar
- `restart: unless-stopped` garante disponibilidade
- Logs acessíveis via `docker logs` a qualquer momento

- **Agendamento**: robfig/cron/v3
- **Email**: Resend (resend/resend-go/v2)
- **Logging**: Charmbracelet/log (logs bonitos coloridos)
- **Error Tracking**: Sentry (getsentry/sentry-go)
- **Config**: env vars (padrão Go)
- **Testes**: testing + testify (mock/assert)
- **Metrics**: Grafana Cloud (prometheus/client_golang)
- **Agendamento**: robfig/cron/v3
- **Email**: Resend (resend/resend-go/v2)
- **Logging**: Charmbracelet/log (logs bonitos coloridos)
- **Config**: env vars (padrão Go)
- **Testes**: testing + testify (mock/assert)

### Arquitetura
- Clean Architecture com separação clara
- Domain em `internal/domain/` (sem dependências externas)
- Implementações em `internal/rpa/`, `internal/scheduler/`, `internal/storage/`
- Interfaces para permitir mocking
- Injeção de dependências via construtores

### Qualidade
- 100% test coverage obrigatório
- Logs estruturados e coloridos
- Graceful shutdown em todos os pontos
- Tratamento de erros completo (nunca ignorar)
- Commits na branch `main` após cada task

---

## Próximos Passos

Para iniciar a execução deste plano, execute:

```bash
/start-work
```

O Sisyphus (orquestrador) irá:
1. Ler este plano em `.sisyphus/plans/hourglass-rpa.md`
2. Distribuir as tasks para os agentes apropriados
3. Executar em paralelo onde possível
4. Garantir que cada task tenha seus critérios de aceitação atendidos
5. Fazer commits na branch `main` após cada task

⚠️ **Importante**: Este é um plano READ-ONLY. As implementações serão feitas pelos agentes de execução, não por você (Prometheus).

---

## Apêndice: Alternativas 100% Gratuitas (Futuro)

Caso algum dia os Free Tiers não sejam suficientes, aqui estão alternativas **totalmente gratuitas**:

### 📧 Email (Alternativas ao Resend)

1. **Gmail SMTP** (Google Workspace ou conta pessoal)
   - Limite: 500 emails/dia (via web) / 100 emails/dia (via SMTP)
   - Setup: `smtp.gmail.com:587` com App Password
   - 100% gratuito se você já tem Gmail

2. **Outlook SMTP** (Microsoft)
   - Limite: 300 emails/dia
   - Setup: `smtp-mail.outlook.com:587`

3. **Self-hosted**: Postfix + Dovecot na sua VPS
   - Custo: $0 (usa sua própria infraestrutura)
   - Requer: VPS com IP limpo (não em blacklist)

### 🐛 Error Tracking (Alternativas ao Sentry)

1. **Self-hosted Sentry**
   - Docker compose completo disponível
   - Custo: $0 (usa sua VPS)
   - GitHub: `getsentry/self-hosted`

2. **Bugsink**
   - Self-hosted error tracking
   - Leve, simples, 100% gratuito

3. **GlitchTip**
   - Open source Sentry alternative
   - API compatível com Sentry

### 📊 Métricas (Alternativas ao Grafana Cloud)

1. **Self-hosted Prometheus + Grafana**
   - Docker compose disponível
   - Custo: $0 (usa sua VPS)
   - Armazenamento local ilimitado

2. **Uptrace**
   - Free tier: 1TB storage, 50,000 séries temporais
   - Mais generoso que Grafana Cloud

### 📝 Observações Importantes

- As ferramentas atuais (Resend, Sentry, Grafana Cloud) são **SaaS gerenciado** = menos trabalho operacional
- Alternativas self-hosted requerem **manutenção** (backups, updates, monitoramento)
- Para 2 execuções/dia, os Free Tiers atuais são **suficientes por anos**
- Se precisar escalar no futuro, a arquitetura permite troca fácil (interfaces abstratas)

**Recomendação**: Continue com o stack atual (Free Tiers SaaS). Só mude para self-hosted se realmente necessário.


