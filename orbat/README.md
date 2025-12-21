# Orbat - 6th Maroon Division Homepage

Next.js application for managing military operations, slotting, and team organization.

## Overview

This container hosts the 6th Maroon Division Homepage (Orbat system) with automatic git pull on updates, showing a maintenance page during deployment.

**Live URL**: https://orbat.chilla55.de

## Features

- ✅ Auto-pulls from GitHub on container restart
- ✅ **Periodic update checking** (configurable interval)
- ✅ Shows maintenance page during updates
- ✅ **Real-time progress tracking** with live status API
- ✅ Built-in Prisma database migrations
- ✅ Discord & Steam OAuth integration
- ✅ **All secrets properly configured** (NEXTAUTH_SECRET, OAuth credentials)
- ✅ Persistent data with Docker volumes
- ✅ Deployed on web node with 1 replica
- ✅ Health checks for container monitoring
- ✅ Zero-downtime updates with graceful restarts

## Architecture

- **Base Image**: `node:20-alpine`
- **Repository**: https://github.com/6th-Maroon-Division/Homepage
- **Framework**: Next.js 14+ with TypeScript
- **Database**: PostgreSQL with Prisma ORM
- **Authentication**: NextAuth.js (Discord + Steam)

## Quick Start

### 1. Create Required Secrets

```bash
# Create NextAuth secret (generate a random string)
openssl rand -base64 32 | docker secret create nextauth_secret -

# Create Discord OAuth secrets (prompts for input, won't save to history)
read -sp "Discord Client ID: " discord_id && echo "$discord_id" | docker secret create discord_client_id - && echo
read -sp "Discord Client Secret: " discord_secret && echo "$discord_secret" | docker secret create discord_client_secret - && echo

# Create Steam API secret (prompts for input)
read -sp "Steam API Key: " steam_key && echo "$steam_key" | docker secret create steam_api_key - && echo

# Create Database password secret (prompts for input)
read -sp "Database Password: " db_password && echo "$db_password" | docker secret create database_password - && echo
```

Or use the Makefile helper (also uses secure prompts):
```bash
make create-secrets
```

### 2. Configure Environment

Edit `docker-compose.swarm.yml` and update database connection details:
- `DATABASE_HOST` - PostgreSQL host (default: postgresql)
- `DATABASE_PORT` - PostgreSQL port (default: 5432)
- `DATABASE_NAME` - Database name (default: orbat)
- `DATABASE_USER` - Database user (default: orbat)
- `DATABASE_SCHEMA` - Schema name (default: public)

The `DATABASE_URL` will be automatically constructed from these values plus the password secret.

### 3. Deploy

```bash
# Build and push image
make build
make push

# Deploy to swarm
make deploy
```

## Environment Variables

### Required (in docker-compose.swarm.yml)
- `NODE_ENV` - Set to "production"
- `PORT` - Application port (default: 3000)
- `UPDATE_CHECK_INTERVAL` - Seconds between update checks (default: 300 = 5 min, 0 = disabled)
- `SHOW_EXTENDED_INFO` - Show detailed progress during updates (default: "false")
- `NEXTAUTH_URL` - Full URL of the application
- `DATABASE_HOST` - PostgreSQL server hostname
- `DATABASE_PORT` - PostgreSQL server port
- `DATABASE_NAME` - Database name
- `DATABASE_USER` - Database username
- `DATABASE_SCHEMA` - Database schema (usually "public")

### Required Secrets
- `nextauth_secret` - NextAuth.js session secret (generate with `openssl rand -base64 32`)
- `discord_client_id` - Discord OAuth application ID
- `discord_client_secret` - Discord OAuth secret
- `steam_api_key` - Steam Web API key
- `database_password` - PostgreSQL database password

**How Secrets Work:**
1. Secrets are mounted as files in `/run/secrets/`
2. The Go entrypoint (`main.go`) reads these files on startup
3. They are exported as environment variables for Next.js:
   - `NEXTAUTH_SECRET` - Read from `nextauth_secret` secret
   - `DISCORD_CLIENT_ID` - Read from `discord_client_id` secret
   - `DISCORD_CLIENT_SECRET` - Read from `discord_client_secret` secret
   - `STEAM_API_KEY` - Read from `steam_api_key` secret
   - `DATABASE_URL` - Constructed from components + `database_password` secret

### OAuth Configuration
- `DISCORD_REDIRECT_URI` - https://orbat.chilla55.de/api/auth/callback/discord
- `STEAM_REDIRECT_URI` - https://orbat.chilla55.de/api/auth/callback/steam

## Volume Persistence

One volume stores all persistent data:
- `orbat_app` - Git repository, node_modules, and all application files

## Update Process

The container automatically checks for updates in two ways:

### 1. On Container Restart
When the container restarts:
- **First Run**: Clones repository from GitHub
- **Subsequent Runs**: 
  - Checks for new commits
  - If updates found:
    - Shows maintenance page
    - Pulls latest code
    - Rebuilds application
    - Runs Prisma migrations
  - If no updates: Starts immediately

### 2. Periodic Background Checks
A background process runs continuously and:
- Checks GitHub every `UPDATE_CHECK_INTERVAL` seconds (default: 300 = 5 minutes)
- Compares local and remote commit hashes
- When updates are detected:
  - Shows maintenance page to users
  - Pulls and rebuilds application
  - Runs database migrations
  - Gracefully restarts the Next.js server
  - Returns to normal operation

**Configure the check interval** by setting `UPDATE_CHECK_INTERVAL` in docker-compose.swarm.yml:
- `300` = 5 minutes (default)
- `600` = 10 minutes
- `1800` = 30 minutes
- `3600` = 1 hour
- `0` = Disable periodic checks (only check on restart)

## Maintenance Page

During updates, users see a styled maintenance page at port 3000 with:
- 6th Maroon Division branding
- **Real-time progress bar** showing update status
- **Live status updates** via `/api/status` endpoint
- Current step indicators (checking, pulling, building, etc.)
- Auto-refresh when deployment completes

### Extended Information Mode

Set `SHOW_EXTENDED_INFO=true` in docker-compose.swarm.yml to display:
- Current deployment step name
- Exact progress percentage
- Detailed operation descriptions
- Timestamp of last update

When set to `false` (default), shows a clean progress bar with current step only.

## Commands

```bash
# Build image
make build

# Push to registry
make push

# Deploy to swarm
make deploy

# Full update (build + push + deploy)
make update

# View logs
make logs

# Restart service
make restart

# Show version
make version

# Clean local images
make clean
```

## Health Checks

The container includes a health check script that:
- Returns healthy during maintenance mode
- Checks Next.js API or homepage availability
- Runs every 30 seconds with 60s start period

## Database Setup

The application uses Prisma with PostgreSQL. On startup:
1. Generates Prisma Client
2. Runs migrations (`npx prisma migrate deploy`)
3. Ensures database schema is current

## OAuth Setup

### Discord
1. Create application at https://discord.com/developers/applications
2. Add redirect URL: `https://orbat.chilla55.de/api/auth/callback/discord`
3. Copy Client ID and Secret to Docker secrets

### Steam
1. Get API key at https://steamcommunity.com/dev/apikey
2. Set domain to `chilla55.de`
3. Copy API key to Docker secret

## Nginx Configuration

Add to nginx sites-available:

```nginx
server {
    listen 443 ssl http2;
    server_name orbat.chilla55.de;

    ssl_certificate /etc/nginx/certs/live/chilla55.de/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/chilla55.de/privkey.pem;

    location / {
        proxy_pass http://orbat_orbat:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Deployment Constraints

- **Node Label**: `node.labels.web.node == web`
- **Replicas**: 1
- **Network**: web-net (external)
- **Resources**:
  - CPU: 0.5-2 cores
  - Memory: 512MB-2GB

## Troubleshooting

### Container won't start
```bash
# Check logs
make logs

# Check service status
docker service ps orbat_orbat

# Verify secrets exist
make list-secrets
```

### Database connection issues
- Verify `DATABASE_URL` format
- Ensure PostgreSQL service is accessible
- Check if database exists

### OAuth not working
- Verify redirect URLs match exactly
- Check secrets are populated correctly
- Ensure HTTPS is enabled

### Git pull fails
- Container needs network access to GitHub
- Check if repository is accessible
- Verify no local uncommitted changes in volume

## Version

Current version: `1.0.0`

## Repository

- **Source**: https://github.com/6th-Maroon-Division/Homepage
- **Container Registry**: ghcr.io/chilla55/orbat

## License

Proprietary - 6th Maroon Division
