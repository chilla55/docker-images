#!/bin/bash
# ============================================================================
# Deploy to Server - Download from GitHub and Execute Migration
# ============================================================================
# Run this script on your mail server to download files and start migration
# ============================================================================

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "============================================================================"
echo "Docker Swarm Migration - Deploy from GitHub"
echo "============================================================================"

# Configuration
REPO_URL="https://github.com/chilla55/docker-images.git"
BRANCH="main"
DEPLOY_DIR="/serverdata/docker-swarm"

echo ""
echo -e "${BLUE}This script will:${NC}"
echo "  1. Clone/update docker-images repository from GitHub"
echo "  2. Set up scripts and configuration"
echo "  3. Guide you through the migration process"
echo ""
echo -e "${YELLOW}Repository:${NC} $REPO_URL"
echo -e "${YELLOW}Branch:${NC} $BRANCH"
echo -e "${YELLOW}Deploy to:${NC} $DEPLOY_DIR"
echo ""
read -p "Press Enter to continue or Ctrl+C to cancel..."

# Check if git is installed
echo ""
echo "Checking prerequisites..."
if ! command -v git &> /dev/null; then
    echo -e "${YELLOW}Git not found, installing...${NC}"
    apt-get update && apt-get install -y git
fi
echo -e "${GREEN}✓ Git is installed${NC}"

# Check if Docker Swarm is initialized
if ! docker node ls &>/dev/null; then
    echo -e "${YELLOW}⚠ This node is not a Swarm manager${NC}"
    echo "Please run this script on a Swarm manager node (mail)"
    exit 1
fi
echo -e "${GREEN}✓ Running on Swarm manager${NC}"

# Handle existing deployment directory
if [ -d "$DEPLOY_DIR" ]; then
    echo ""
    echo -e "${YELLOW}Existing deployment found at: $DEPLOY_DIR${NC}"
    echo ""
    echo "Choose an action:"
    echo "  1) Clean install (remove everything and clone fresh)"
    echo "  2) Update (pull latest changes, preserve .env files)"
    echo "  3) Cancel"
    echo ""
    read -p "Enter choice [1-3]: " choice
    
    case $choice in
        1)
            echo -e "${BLUE}Performing clean install...${NC}"
            # Backup .env files if they exist
            BACKUP_DIR="/tmp/docker-swarm-backup-$(date +%s)"
            mkdir -p "$BACKUP_DIR"
            find "$DEPLOY_DIR" -name ".env" -exec cp --parents {} "$BACKUP_DIR/" \; 2>/dev/null || true
            
            # Remove old directory
            rm -rf "$DEPLOY_DIR"
            mkdir -p "$DEPLOY_DIR"
            
            if [ -d "$BACKUP_DIR/serverdata/docker-swarm" ]; then
                echo -e "${GREEN}✓ .env files backed up to: $BACKUP_DIR${NC}"
                echo "  Restore them after deployment if needed"
            fi
            ;;
        2)
            echo -e "${BLUE}Updating existing deployment...${NC}"
            ;;
        3)
            echo "Cancelled."
            exit 0
            ;;
        *)
            echo "Invalid choice. Cancelled."
            exit 1
            ;;
    esac
fi

# Create deploy directory if it doesn't exist
mkdir -p "$DEPLOY_DIR"
cd "$DEPLOY_DIR"

# Clone or update repository
echo ""
if [ -d ".git" ]; then
    echo -e "${BLUE}Updating existing repository...${NC}"
    git fetch origin
    git checkout "$BRANCH"
    git pull origin "$BRANCH"
    echo -e "${GREEN}✓ Repository updated${NC}"
else
    echo -e "${BLUE}Cloning repository to $DEPLOY_DIR...${NC}"
    mkdir -p "$(dirname "$DEPLOY_DIR")"
    cd "$(dirname "$DEPLOY_DIR")"
    rm -rf "$(basename "$DEPLOY_DIR")"
    git clone -b "$BRANCH" "$REPO_URL" "$(basename "$DEPLOY_DIR")"
    cd "$DEPLOY_DIR"
    echo -e "${GREEN}✓ Repository cloned${NC}"
fi

# Make scripts executable
echo ""
echo "Setting up scripts..."
chmod +x scripts/*.sh
echo -e "${GREEN}✓ Scripts are now executable${NC}"

# Display current configuration
echo ""
echo "============================================================================"
echo "Repository Setup Complete!"
echo "============================================================================"
echo ""
echo "Location: $DEPLOY_DIR"
echo "Branch: $(git rev-parse --abbrev-ref HEAD)"
echo "Latest commit: $(git log -1 --pretty=format:'%h - %s')"
echo ""
echo "============================================================================"
echo "Next Steps - Migration Process"
echo "============================================================================"
echo ""
echo "1. Create Docker Secrets (generates random passwords):"
echo "   cd $DEPLOY_DIR"
echo "   ./scripts/03-setup-secrets.sh"
echo ""
echo "2. Create Nginx Config:"
echo "   ./scripts/04-setup-nginx-config.sh"
echo ""
echo "3. Verify Prerequisites:"
echo "   ./scripts/00-check-prerequisites.sh"
echo ""
echo "4. Deploy MariaDB Stack:"
echo "   cd mariadb"
echo "   cat > .env <<'EOF'"
echo "VERSION=latest"
echo "MYSQL_DATABASE=panel"
echo "MYSQL_USER=ptero"
echo "EOF"
echo "   echo \"MYSQL_ROOT_PASSWORD=\$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)\" >> .env"
echo "   echo \"MYSQL_PASSWORD=\$(docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d)\" >> .env"
echo "   echo \"REPLICATION_PASSWORD=\$(docker secret inspect mysql_replication_password -f '{{.Spec.Data}}' | base64 -d)\" >> .env"
echo "   echo \"MAXSCALE_USER=admin\" >> .env"
echo "   echo \"MAXSCALE_PASSWORD=\$(docker secret inspect maxscale_password -f '{{.Spec.Data}}' | base64 -d)\" >> .env"
echo "   docker stack deploy -c docker-compose.swarm.yml mariadb"
echo "   # Wait for services: watch -n 2 'docker service ls | grep mariadb'"
echo ""
echo "5. Migrate Data from Old Container:"
echo "   cd $DEPLOY_DIR"
echo "   ./scripts/05-migrate-data.sh"
echo "   # Enter old container name and password (123lol789)"
echo ""
echo "6. Deploy Remaining Services:"
echo "   cd redis && docker stack deploy -c docker-compose.swarm.yml redis"
echo "   cd ../petrodactyl && docker stack deploy -c docker-compose.swarm.yml pterodactyl"
echo "   cd ../vaultwarden && docker stack deploy -c docker-compose.swarm.yml vaultwarden"
echo "   cd ../certbot && docker stack deploy -c docker-compose.swarm.yml certbot"
echo "   cd ../nginx && docker stack deploy -c docker-compose.swarm.yml nginx"
echo ""
echo "============================================================================"
echo "For detailed instructions, see:"
echo "  $DEPLOY_DIR/MIGRATION_STEPS.md"
echo "  $DEPLOY_DIR/QUICKSTART.md"
echo "  $DEPLOY_DIR/ZERO_DOWNTIME_MIGRATION.md"
echo "============================================================================"
