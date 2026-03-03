# Plano de Autenticação Automática - Go Puro

## RESTRIÇÃO: Somente Golang

## Solução Escolhida: Abordagem Híbrida Go (Opção 1 + 2)

### Estratégia Principal: WebAuthn Fix (Tentativa Final)
**Tempo**: 2-3 dias
**Probabilidade de sucesso**: 70%

Investigar e corrigir definitivamente a implementação WebAuthn:
1. Comparar byte-a-byte nosso payload com o do navegador
2. Verificar formato exato da assinatura ECDSA
3. Validar todos os campos do authData
4. Testar diferentes combinações de flags/campos

Se funcionar: **Sistema 100% Go, leve e eficiente**

---

### Estratégia Fallback: chromedp (Go Browser Automation)
**Tempo**: 3-4 dias  
**Probabilidade de sucesso**: 95%

Usar **chromedp** (biblioteca Go oficial para Chrome DevTools Protocol):
- Controle Chrome/Chromium via Go puro
- Navega, clica, autentica via WebAuthn
- Extrai cookies automaticamente
- Headless (sem interface gráfica)

Vantagens:
- ✅ Go puro (sem Python/JS)
- ✅ Chrome já disponível na maioria das VPS
- ✅ Mesmo fluxo do usuário (garantido)
- ✅ Headless (roda em background)

---

## PLANO DE IMPLEMENTAÇÃO

### Fase 1: Tentativa Final WebAuthn (Dias 1-2)

#### Tarefa 1.1: Análise Detalhada
- [ ] Capturar payload EXATO do navegador (byte a byte)
- [ ] Comparar com nosso payload gerado em Go
- [ ] Identificar diferenças em:
  - Signature format (ASN.1 DER vs raw)
  - AuthData flags
  - ClientDataJSON (ordem dos campos)
  - UserHandle encoding

#### Tarefa 1.2: Correções
- [ ] Ajustar formato da assinatura conforme necessário
- [ ] Corrigir campos do authData
- [ ] Testar múltiplas variações

#### Tarefa 1.3: Validação
- [ ] Se funcionar: Prosseguir para integração
- [ ] Se falhar: Ir para Fase 2 (chromedp)

**Critério de Sucesso**: Autenticação retorna 200 com novos tokens

---

### Fase 2: Implementação chromedp (Dias 3-6)

#### Tarefa 2.1: Setup
- [ ] Instalar Chrome/Chromium na VPS
- [ ] Adicionar chromedp ao go.mod
- [ ] Criar estrutura de diretórios

#### Tarefa 2.2: Automação
- [ ] Implementar navegador headless
- [ ] Script: Abrir Hourglass → Login → WebAuthn → Extrair Cookies
- [ ] Salvar cookies em formato compatível com api.Client

#### Tarefa 2.3: Integração
- [ ] Criar TokenRefresher service
- [ ] Monitorar expiração (a cada 30 min)
- [ ] Quando próximo de expirar: Executar chromedp
- [ ] Atualizar api.Client com novos tokens

---

### Fase 3: Robustez (Dias 7-8)

#### Tarefa 3.1: Retry & Fallback
- [ ] Se WebAuthn falhar: Tentar chromedp
- [ ] Se chromedp falhar: Notificar usuário via Telegram
- [ ] Circuit breaker (parar após 3 falhas)

#### Tarefa 3.2: Monitoramento
- [ ] Logs detalhados de cada tentativa
- [ ] Métricas: tempo de renovação, taxa de sucesso
- [ ] Health check endpoint

---

## ARQUITETURA FINAL (Go Puro)

```
┌─────────────────────────────────────────────────────────────┐
│                    VPS - Go Application                      │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │  API Client  │◄───┤ Token Manager│◄───┤ WebAuthn     │   │
│  │              │    │              │    │ (Tentativa 1)│   │
│  └──────────────┘    └──────┬───────┘    └──────────────┘   │
│                              │                              │
│                              │ Falha?                        │
│                              ▼                              │
│                       ┌──────────────┐                      │
│                       │   chromedp   │                      │
│                       │   (Go)       │                      │
│                       └──────┬───────┘                      │
│                              │                              │
│                              ▼                              │
│                       ┌──────────────┐                      │
│                       │ Chrome Head- │                      │
│                       │ less (Docker)│                      │
│                       └──────────────┘                      │
└─────────────────────────────────────────────────────────────┘
```

---

## DECISÃO

**Recomendação**: Implementar ambas as fases

**Ordem**:
1. Fase 1 (WebAuthn Fix) - 2 dias
   - Se funcionar: Ótimo, sistema leve!
   - Se falhar: Partir para Fase 2

2. Fase 2 (chromedp) - 3 dias
   - Solução garantida
   - Mais pesada mas funciona

**Total**: 5 dias no máximo

---

## PRECISO DA SUA DECISÃO

1. **Quer começar pela Fase 1** (tentar corrigir WebAuthn)?
   - Menor esforço se funcionar
   - Sistema mais leve
   - 70% chance de sucesso

2. **Quer pular direto para Fase 2** (chromedp)?
   - Garantia de funcionar
   - Implementação mais complexa
   - 95% chance de sucesso

3. **Quer fazer as duas fases** (recomendado)?
   - Fase 1 primeiro (quick win)
   - Fase 2 como fallback
   - Melhor dos dois mundos

**Qual você escolhe?**
