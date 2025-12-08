# Quick Deploy from GitHub

## On Your Server (mail)

### Option 1: One-Line Deploy

```bash
ssh root@mail
curl -fsSL https://raw.githubusercontent.com/chilla55/docker-images/docker-swarm/deploy-from-github.sh | bash
```

This will:
- Clone the repository to `/serverdata/docker`
- Make all scripts executable
- Show you the next steps

---

### Option 2: Manual Clone

```bash
ssh root@mail

# Clone repository
cd /serverdata
rm -rf docker  # Remove old if exists
git clone -b docker-swarm https://github.com/chilla55/docker-images.git docker
cd docker

# Make scripts executable
chmod +x scripts/*.sh
chmod +x deploy-from-github.sh

# Follow migration steps
cat MIGRATION_STEPS.md
```

---

## After Cloning

The repository will be at `/serverdata/docker` with this structure:

```
/serverdata/docker/
├── scripts/
│   ├── 00-check-prerequisites.sh
│   ├── 01-setup-networks.sh
│   ├── 02-setup-node-labels.sh
│   ├── 03-setup-secrets.sh
│   ├── 04-setup-nginx-config.sh
│   └── 05-migrate-data.sh
├── mariadb/
├── nginx/
├── petrodactyl/
├── postgresql/
├── redis/
├── vaultwarden/
├── certbot/
├── MIGRATION_STEPS.md
└── QUICKSTART.md
```

---

## Start Migration

```bash
cd /serverdata/docker

# Step 1: Create secrets
./scripts/03-setup-secrets.sh

# Step 2: Create nginx config
./scripts/04-setup-nginx-config.sh

# Step 3: Continue with MIGRATION_STEPS.md
cat MIGRATION_STEPS.md
```

---

## Update Repository Later

```bash
cd /serverdata/docker
git pull origin docker-swarm
chmod +x scripts/*.sh
```

---

## Commit & Push Your Changes

After deployment, you may want to commit your local changes:

```bash
cd /serverdata/docker

# See what changed
git status

# Add your changes
git add .

# Commit
git commit -m "Updated configuration for production deployment"

# Push (if you have write access)
git push origin docker-swarm
```

---

## Branch Information

- **Repository**: https://github.com/chilla55/docker-images
- **Branch**: `docker-swarm`
- **Deploy Location**: `/serverdata/docker`
