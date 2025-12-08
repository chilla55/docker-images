#!/bin/bash
# ============================================================================
# Setup Script for Certbot with Hetzner Storage Box
# ============================================================================
# This script helps you set up the required Docker secrets for certbot
# ============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# ──────────────────────────────────────────────────────────────────────────
# Check if Docker is available
# ──────────────────────────────────────────────────────────────────────────
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running or you don't have permission"
        exit 1
    fi
    
    log_info "Docker is available"
}

# ──────────────────────────────────────────────────────────────────────────
# Setup Cloudflare credentials secret
# ──────────────────────────────────────────────────────────────────────────
setup_cloudflare_secret() {
    log_info "Setting up Cloudflare credentials secret..."
    
    if docker secret inspect cloudflare_credentials &> /dev/null; then
        log_warn "Secret 'cloudflare_credentials' already exists"
        read -p "Do you want to remove and recreate it? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker secret rm cloudflare_credentials
        else
            log_info "Skipping Cloudflare secret creation"
            return
        fi
    fi
    
    # Check for cloudflare.ini file
    if [ -f "$SCRIPT_DIR/cloudflare.ini" ]; then
        log_info "Found cloudflare.ini file"
        docker secret create cloudflare_credentials "$SCRIPT_DIR/cloudflare.ini"
        log_info "Created secret 'cloudflare_credentials'"
    else
        log_error "cloudflare.ini not found!"
        log_info "Please create cloudflare.ini from cloudflare.ini.example"
        log_info "Example: cp cloudflare.ini.example cloudflare.ini"
        log_info "Then edit cloudflare.ini with your Cloudflare API token"
        exit 1
    fi
}

# ──────────────────────────────────────────────────────────────────────────
# Setup Storage Box password secret
# ──────────────────────────────────────────────────────────────────────────
setup_storagebox_secret() {
    log_info "Setting up Storage Box password secret..."
    
    if docker secret inspect storagebox_password &> /dev/null; then
        log_warn "Secret 'storagebox_password' already exists"
        read -p "Do you want to remove and recreate it? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker secret rm storagebox_password
        else
            log_info "Skipping Storage Box secret creation"
            return
        fi
    fi
    
    # Check for storagebox.txt file or prompt for password
    if [ -f "$SCRIPT_DIR/storagebox.txt" ]; then
        log_info "Found storagebox.txt file"
        docker secret create storagebox_password "$SCRIPT_DIR/storagebox.txt"
        log_info "Created secret 'storagebox_password'"
    else
        log_warn "storagebox.txt not found"
        log_info "Please enter your Storage Box password (input will be hidden):"
        read -s -p "Password: " password
        echo
        if [ -n "$password" ]; then
            echo "$password" | docker secret create storagebox_password -
            log_info "Created secret 'storagebox_password'"
        else
            log_error "No password provided!"
            log_info "Alternative: Create storagebox.txt with your password and run again"
            exit 1
        fi
    fi
}

# ──────────────────────────────────────────────────────────────────────────
# Main setup
# ──────────────────────────────────────────────────────────────────────────
main() {
    log_info "=========================================="
    log_info "Certbot with Hetzner Storage Box Setup"
    log_info "=========================================="
    
    check_docker
    setup_cloudflare_secret
    setup_storagebox_secret
    
    log_info "=========================================="
    log_info "Setup completed successfully!"
    log_info "=========================================="
    log_info ""
    log_info "Next steps:"
    log_info "1. Build the Docker image:"
    log_info "   cd $SCRIPT_DIR"
    log_info "   docker build -t ghcr.io/chilla55/certbot-storagebox:latest ."
    log_info ""
    log_info "2. Edit docker-compose.swarm.yml and set:"
    log_info "   - CERT_EMAIL: your email address"
    log_info "   - CERT_DOMAINS: your domain(s)"
    log_info "   - STORAGE_BOX_HOST: your Storage Box hostname"
    log_info "   - STORAGE_BOX_USER: your Storage Box username"
    log_info "   - NGINX_SERVICE_NAME: your nginx service name"
    log_info ""
    log_info "3. Deploy the stack:"
    log_info "   docker stack deploy -c docker-compose.swarm.yml certbot"
    log_info ""
    log_info "4. Check logs:"
    log_info "   docker service logs -f certbot_certbot"
}

main "$@"
