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
- ✅ Maintenance-mode updates with coordinated web and scheduler restarts

## Architecture

- **Base Image**: `node:24-alpine`
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

## Scheduler lifecycle

The active image entrypoint is built from `main.go` and `runtime.go`. The legacy
`entrypoint.sh` and checked-in `entrypoint` binary are not used by the Dockerfile.

After dependency installation, Prisma generation/migrations, and the Next.js
build, the container runs `npm start` and `npm run scheduler` from `/app/repo`.
Both inherit the same environment and loaded secrets, including `DATABASE_URL`.
The reviewed branch runs `node --import tsx scripts/scheduler.ts`; `tsx` and
`@next/env` are production dependencies. Development dependencies also remain
installed for the Next.js build and Prisma CLI. The scheduler requires no extra
secrets, ports, or volume beyond the existing database and application files.

The supervisor restarts an exited process after five seconds without restarting
its healthy sibling. Updates stop both process groups before changing files or
running migrations; neither process restarts during the build. Shutdown sends
SIGTERM to the entire npm process groups, waits up to 150 seconds, then kills
remaining descendants. Tini reaps orphaned children. Swarm allows 180 seconds for
container shutdown. Before changing application files and before starting the web
server, the supervisor verifies that the configured web port can be bound. A
leftover listener stops the update with an error instead of launching a competing
Next.js server.

Keep **one replica**. Swarm updates and rollbacks use `stop-first` to avoid overlap
between old/new schedulers and concurrent writes to the shared application volume.
This introduces deployment downtime. The reviewed scheduler uses a transaction-held
database lock to serialize job execution, but that does not protect shared files
from concurrent container builds. Keep the single-replica deployment.

Health checks establish process liveness, not successful job execution. A scheduler
heartbeat would be needed to detect a live but stuck worker. The deployed app must
define a long-running `scheduler` npm script; set `SCHEDULER_ENABLED=false` when
running an older revision without that script.

### Release order

The scheduler was verified on application `main` at commit
`a5f85b55bf1f9bc307ecf1e2cebf525a0d9903bb` (PR #77). Its package manifest
requires Node `^24.11.0`; the image uses Node 24 Alpine. The container continues
cloning and updating **main**, with `SCHEDULER_ENABLED=true` in the stack.
The existing `prisma migrate deploy` step creates `SchedulerState` and
`SchedulerJob` before the scheduler starts.

Build and publish this container image, then update the Swarm stack to use it.
An application git pull inside an older container does not upgrade Node or add
the scheduler supervisor. When rolling the application back to a revision without
the scheduler script, set `SCHEDULER_ENABLED=false`.

The worker ticks every 30 seconds. Each job transaction and its error-recovery
transaction can take up to 60 seconds; the shutdown allowance accommodates both
plus connection cleanup. Job failures are logged as `scheduler.tick_failed` or
`scheduler.job_failed`; successful jobs log `scheduler.job_committed`.

## Environment Variables

### Required (in docker-compose.swarm.yml)
- `SCHEDULER_ENABLED` - Run `npm run scheduler` alongside the web server (default: `true`; set exactly `false` to disable)
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
  - Reinstalls dependencies, applies migrations, and builds on every startup

### 2. Periodic Background Checks
A background process runs continuously and:
- Checks GitHub every `UPDATE_CHECK_INTERVAL` seconds (default: 300 = 5 minutes)
- Compares local and remote commit hashes
- When updates are detected:
  - Shows maintenance page to users
  - Pulls and rebuilds application
  - Runs database migrations
  - Stops both web server and scheduler before modifying code, dependencies, or the database
  - Restarts both processes after a successful build
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
- Accepts the maintenance server only during intentional startup/build work
- Requires live web and scheduler processes during normal operation (scheduler optional when disabled)
- Checks Next.js API or homepage availability with bounded HTTP timeouts
- Fails after a clone/build/update error, even if the maintenance page still responds
- Runs every 30 seconds with a 5-minute start period

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

Current version: `1.1.0`

## Repository

- **Source**: https://github.com/6th-Maroon-Division/Homepage
- **Container Registry**: ghcr.io/chilla55/orbat

## License

Proprietary - 6th Maroon Division
