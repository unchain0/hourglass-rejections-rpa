# Token Renewal & Deployment Guide

## ⚠️ URGENT: Tokens Expired

The deployment is currently failing due to expired authentication tokens.

## Step 1: Renew Tokens (YOU MUST RUN THIS)

### Prerequisites
- Chrome or Chromium browser installed locally
- Access to Hourglass credentials

### Commands to Run

```bash
# 1. Save new tokens (opens Chrome, you log in, tokens auto-saved)
make save-tokens

# 2. Verify tokens were saved
cat ~/.hourglass-rpa/auth-tokens.json
# Should show: {"xsrf_token":"...","hglogin_cookie":"...","timestamp":"..."}
```

### If Using SSH Key Authentication
```bash
# 3. Copy tokens to VPS
make copy-to-vps VPS=seu-usuario@31.97.244.252
```

### If Using Password Authentication
```bash
# 3. Copy tokens to VPS (with password prompt)
make copy-to-vps-password VPS=seu-usuario@31.97.244.252
```

### Manual Copy (if above fails)
```bash
# Show token content
cat ~/.hourglass-rpa/auth-tokens.json

# SSH to VPS and create file
ssh seu-usuario@31.97.244.252
mkdir -p ~/.hourglass-rpa
nano ~/.hourglass-rpa/auth-tokens.json
# Paste content from local machine, save (Ctrl+X, Y, Enter)
chmod 600 ~/.hourglass-rpa/auth-tokens.json
```

## Step 2: Verify Tokens on VPS

```bash
ssh seu-usuario@31.97.244.252
cat ~/.hourglass-rpa/auth-tokens.json
# Should show valid tokens with recent timestamp
```

## Step 3: Restart Deployment

After you confirm tokens are renewed, I will:

1. Trigger redeployment to Coolify VPS
2. Verify container health checks pass
3. Confirm no more 401 Unauthorized errors
4. Test bot responsiveness

## Current Coolify Configuration

- **URL**: ps8ooks4kogcw4wk0kk8o40o.31.97.244.252.sslip.io
- **Environment Variables**:
  - `TELEGRAM_BOT_TOKEN` (set)
  - `TELEGRAM_WHITELIST` (set)
  - `TOKENS_PATH=/home/rpa/.hourglass-rpa/auth-tokens.json`
- **Volume Mount**: `/home/user/.hourglass-rpa` → `/home/rpa/.hourglass-rpa`

## Expected Timeline

- Token renewal: ~5 minutes
- Deployment: ~3 minutes
- Verification: ~2 minutes

**Total: ~10 minutes after you run the commands**

---

## Troubleshooting

### "401 Unauthorized" errors
- Tokens expired (8-hour lifetime)
- Solution: Run `make save-tokens` again

### "Permission denied" on token file
```bash
chmod 600 ~/.hourglass-rpa/auth-tokens.json
```

### Chrome not opening
```bash
# Install Chrome if missing
# macOS:
brew install --cask google-chrome

# Ubuntu/Debian:
wget -q -O - https://dl.google.com/linux/linux_signing_key.pub | sudo apt-key add -
sudo sh -c 'echo "deb [arch=amd64] http://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google.list'
sudo apt-get update
sudo apt-get install google-chrome-stable
```

### Bot already running (conflict)
- Check Coolify dashboard for duplicate containers
- Stop old containers before deploying

---

**Run the commands above, then tell me "tokens renewed" and I'll complete the deployment.**
