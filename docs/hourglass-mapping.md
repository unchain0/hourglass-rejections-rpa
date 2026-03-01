# Mapeamento do Hourglass - RPA

## 🔗 URL Base
```
https://app.hourglass-app.com/v2/page/app
```

## 🔐 Página de Login

### Seletores Identificados:

| Elemento | Seletor | Tipo |
|----------|---------|------|
| Logo | `.App-logo` | CSS Class |
| Título | `h4.App-logo-text` ou texto "Hourglass Sign In" | CSS Class / Texto |
| **Campo Email** | `input#email` ou `[name="email"]` | ID / Name |
| **Campo Senha** | `input#password` ou `[name="password"]` | ID / Name |
| **Botão Login** | `button[type='submit']` | Type |
| Link "I forgot" | `button:has-text("I forgot")` | Texto |
| Google Sign-In | `.gsi-material-button` ou link com texto "Google Sign-In" | Class / Texto |
| Apple Sign-In | `button:has-text("Sign in with Apple")` | Texto |

### Estrutura HTML:
```html
<div id="root">
  <div class="min-vh-100 d-flex align-items-center justify-content-center py-5 container-md">
    <div class="w-100">
      <!-- Logo e Título -->
      <div class="justify-content-center mt-4 mb-5 row">
        <div class="text-center col-lg-4 col-md-5 col-sm-8">
          <div class="mb-3">
            <img class="App-logo" alt="logo" src="...">
          </div>
          <h4 class="App-logo-text fw-light text-muted mb-4">Hourglass Sign In</h4>
        </div>
      </div>
      
      <!-- Formulário de Login -->
      <form>
        <div class="justify-content-center mt-2 row">
          <div class="col-lg-4 col-md-5 col-sm-8">
            <div class="mb-3">
              <label class="fw-medium text-muted form-label" for="email">Email Address</label>
              <input required="" autocomplete="username webauthn" id="email" class="py-2 border-2 form-control" type="email" value="" name="email">
            </div>
          </div>
        </div>
        
        <div class="justify-content-center row">
          <div class="col-lg-4 col-md-5 col-sm-8">
            <div class="mb-3">
              <div class="d-flex justify-content-between align-items-center mb-2">
                <label class="fw-medium text-muted mb-0 form-label" for="password">Password</label>
                <button type="button" tabindex="-1" class="p-0 text-decoration-none fw-normal text-primary border-0 btn btn-link btn-sm">I forgot</button>
              </div>
              <input required="" autocomplete="current-password webauthn" id="password" class="py-2 border-2 form-control" type="password" value="" name="password">
            </div>
            <button type="submit" style="transition: 0.15s ease-in-out;" class="mt-3 w-100 py-2 fw-medium btn btn-primary">Log In</button>
          </div>
        </div>
      </form>
      
      <!-- Separador e Botões OAuth -->
      <div class="justify-content-center mt-4 mb-5 row">
        <div class="text-center col-lg-4 col-md-5 col-sm-8">
          <div class="position-relative mb-4">
            <hr class="text-muted">
            <span class="position-absolute top-50 start-50 translate-middle px-3 text-muted small fw-medium text-center" style="background-color: var(--bs-body-bg, #fff);"></span>
          </div>
          <div class="d-flex gap-3 justify-content-center row">
            <div class="d-flex justify-content-center col-12">
              <a href="https://accounts.google.com/o/oauth2/v2/auth?..." class="gsi-material-button">
                <!-- Google Sign-In -->
              </a>
            </div>
            <div class="d-flex justify-content-center col-12">
              <button type="button" class="w-100 d-flex align-items-center justify-content-center py-2 apple-signin-btn btn btn-dark">
                <!-- Apple Sign-In -->
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</div>
```

### Fluxo de Login Automático:
```javascript
// 1. Navegar para a página
await page.goto('https://app.hourglass-app.com/v2/page/app');

// 2. Aguardar o formulário carregar
await page.waitForSelector('input#email');

// 3. Preencher email
await page.fill('input#email', 'seu-email@petrobras.com.br');

// 4. Preencher senha
await page.fill('input#password', 'sua-senha');

// 5. Clicar no botão de login
await page.click('button[type="submit"]');

// 6. Aguardar redirecionamento (login bem-sucedido)
await page.waitForURL('**/dashboard**', { timeout: 10000 });
```

## 🏠 Dashboard/App Interno

> ⚠️ **Nota**: Não foi possível acessar o dashboard sem credenciais válidas.
> 
> Para completar o mapeamento das seções (Partes Mecânicas, Campo, Testemunho Público),
> é necessário fazer login primeiro.

### Próximos Passos para Mapeamento Completo:

1. **Fazer login no sistema** (manualmente ou automaticamente)
2. **Navegar para cada seção**:
   - Partes Mecânicas
   - Campo
   - Testemunho Público
3. **Identificar os seletores da tabela/grid**:
   - Container da tabela
   - Linhas
   - Colunas (Quem, O Que, Pra Quando)
4. **Documentar URLs ou mecanismo de navegação** entre seções

## 🔧 Código Atualizado

O código em `internal/rpa/login.go` já foi atualizado com os seletores corretos:
- ✅ Campo email: `input#email`
- ✅ Campo senha: `input#password`
- ✅ Botão login: `button[type='submit']`

## 📝 Instruções para Completar o Mapeamento

Para completar o mapeamento e fazer o RPA funcionar perfeitamente:

1. **Configure o `.env`:**
```env
HOURGLASS_EMAIL=seu.email@petrobras.com.br
HOURGLASS_PASSWORD=sua-senha
```

2. **Execute o modo setup:**
```bash
./rpa -setup
```

3. **Após fazer login manualmente**, anote:
   - As URLs de cada seção (ou se são abas)
   - Os seletores CSS das tabelas
   - Os nomes das colunas exatos

4. **Atualize o `analyzer.go`** com os seletores encontrados

## 📸 Screenshot

O screenshot da página de login foi salvo em: `hourglass-login-page.png`

---

**Data do Mapeamento**: 2026-03-01
**Ferramenta**: Playwright MCP
