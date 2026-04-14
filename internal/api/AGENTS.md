# AGENTS.md - API Client

**Directory**: `internal/api/`  
**Purpose**: Hourglass API client and data analyzer  
**Complexity**: Medium  
**Lines**: ~2,000

## 🎯 Overview

Cliente REST para a API do Hourglass. Inclui cliente HTTP, análise de dados e modelos de domínio.

## 📁 Structure

```
internal/api/
├── client.go          # HTTP client with retry logic
├── client_test.go     # Client tests
├── analyzer.go        # Data analysis logic
├── analyzer_test.go   # Analyzer tests
└── models.go          # API data models
```

## 🔍 Key Components

### Client
REST client features:
- Cookie-based authentication
- XSRF token handling
- Automatic retries with backoff
- Request/response logging
- Timeout configuration

### Analyzer
Data analysis:
- User loading and caching
- Notification fetching
- Meeting analysis
- Section-based filtering
- Rejection detection

## 🏗️ API Flow

```
1. Load users (cached)
2. Get notifications for date range
3. Analyze by section:
   - Partes Mecânicas
   - Campo
   - Testemunho Público
4. Return formatted results
```

## ⚙️ Authentication

Uses cookie-based auth:
- `hglogin` cookie
- `X-Hourglass-XSRF-Token` header
- Token refresh support

## 🧪 Testing

- httptest for HTTP mocking
- Mock responses for API calls
- Table-driven tests
- Error path coverage

## 📊 Data Models

Key structs:
- `User` - Hourglass user
- `Assignment` - Assignment data
- `Notification` - Rejection notification
- `Meeting` - Meeting schedule

## 📝 Usage Example

```go
client := NewClient(baseURL)
client.SetXSRFToken(token)
client.SetHGLoginCookie(cookie)

users, err := client.GetUsers(ctx)
if err != nil {
    return err
}
```

## 🔗 Dependencies

- Standard library `net/http`
- `internal/domain` - Shared models
- `internal/cache` - User caching

## 📚 See Also

- Root `AGENTS.md` for project overview
- `internal/domain/` for shared models
- `internal/cache/` for caching logic
