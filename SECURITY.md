# Security Configuration Guide

## Pre-commit Setup

Install pre-commit to enforce CI checks before every commit:

```bash
pip install pre-commit
pre-commit install
```

## Database Encryption

The SQLite database is encrypted using SQLCipher with AES-256 encryption.

### Setting up encryption key:

```bash
# Generate a secure encryption key
go run ./cmd/generate-key

# Set environment variable
export DB_ENCRYPTION_KEY="your-generated-key-here"
```

Add to your `.env` file:
```
DB_ENCRYPTION_KEY=your-secure-key-here
```

### Database Location

The database is stored in a secure location:
- Linux/macOS: `~/.local/share/hourglass-rpa/`
- Windows: `%USERPROFILE%\AppData\Local\hourglass-rpa\`

File permissions are set to 0600 (owner read/write only).

## Security Best Practices

1. **Never commit the encryption key** - It's in `.env` which is gitignored
2. **Restrict file permissions** - Database files have 0600 permissions
3. **Audit logging** - All data access is logged for GDPR compliance
4. **Data retention** - User data auto-deletes after 90 days
5. **PII protection** - Chat IDs are classified as high-sensitivity data

## GDPR Compliance

- All data access is logged in the audit_logs table
- Users can request data deletion (right to be forgotten)
- Data retention period: 90 days
- Database is encrypted at rest
