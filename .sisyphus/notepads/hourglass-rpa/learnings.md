## Domain Layer Implementation
- Defined core data structures in `types.go`.
- Established contracts for Scraper, Storage, and Notifier in `interfaces.go`.
- Created custom domain errors in `errors.go`.
- Ensured all interfaces use `context.Context` for cancellation and timeouts.
### Task 3: Configuração via Environment Variables
- Usado github.com/caarlos0/env/v11 para parsing de variáveis de ambiente.
- Configurações com valores default conforme especificado.
- Testes unitários cobrindo valores default e overrides.
### JSON/CSV Storage Implementation
- Used a mockable approach for json.MarshalIndent to achieve high coverage.
- Refactored saveCSV into writeCSV to allow testing with a custom io.Writer.
- Used csv.Writer.Error() to detect errors during Flush and Write.
- Achieved 98% coverage in internal/storage.
