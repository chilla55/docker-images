#!/bin/bash
# ============================================================================
# Cleanup Script - Remove Docker Swarm Deployment
# ============================================================================
# This script removes the Docker Swarm deployment and optionally the services
# Run this on the Swarm manager node (mail)
# ============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

DEPLOY_DIR="/serverdata/docker-swarm"

echo "============================================================================"
echo "Docker Swarm Deployment Cleanup"
echo "============================================================================"

if [ ! -d "$DEPLOY_DIR" ]; then
    echo -e "${YELLOW}No deployment found at $DEPLOY_DIR${NC}"
    exit 0
fi

echo ""
echo -e "${RED}WARNING: This will remove files from $DEPLOY_DIR${NC}"
echo ""
echo "What would you like to remove?"
echo "  1) Only files (keep Docker stacks/services running)"
echo "  2) Files + Docker stacks (stop all services and remove files)"
echo "  3) Everything (stacks + networks + secrets + configs + files)"
echo "  4) Cancel"
echo ""
read -p "Enter choice [1-4]: " choice

case $choice in
    1)
        echo -e "${BLUE}Removing deployment files only...${NC}"
        rm -rf "$DEPLOY_DIR"
        echo -e "${GREEN}✓ Files removed from $DEPLOY_DIR${NC}"
        echo "Docker services are still running"
        ;;
    2)
        echo -e "${BLUE}Removing Docker stacks and files...${NC}"
        
        # Remove stacks
        STACKS=$(docker stack ls --format '{{.Name}}' 2>/dev/null || true)
        if [ -n "$STACKS" ]; then
            echo "Removing Docker stacks:"
            for stack in $STACKS; do
                echo "  - $stack"
                docker stack rm "$stack"
            done
            echo -e "${GREEN}✓ Stacks removed${NC}"
            echo "Waiting for services to shut down..."
            sleep 10
        fi
        
        # Remove files
        rm -rf "$DEPLOY_DIR"
        echo -e "${GREEN}✓ Files removed from $DEPLOY_DIR${NC}"
        ;;
    3)
        echo -e "${BLUE}Performing complete cleanup...${NC}"
        
        # Remove stacks
        STACKS=$(docker stack ls --format '{{.Name}}' 2>/dev/null || true)
        if [ -n "$STACKS" ]; then
            echo "Removing Docker stacks:"
            for stack in $STACKS; do
                echo "  - $stack"
                docker stack rm "$stack"
            done
            echo -e "${GREEN}✓ Stacks removed${NC}"
            echo "Waiting for services to shut down..."
            sleep 10
        fi
        
        # Remove networks (if not in use)
        echo ""
        echo "Removing overlay networks:"
        for network in web-net mariadb-net postgres-net redis-net; do
            if docker network inspect "$network" &>/dev/null; then
                docker network rm "$network" 2>/dev/null && echo "  - $network" || echo "  - $network (in use, skipped)"
            fi
        done
        
        # List secrets (don't auto-remove as they contain passwords)
        echo ""
        echo -e "${YELLOW}Docker secrets detected (not automatically removed):${NC}"
        docker secret ls --format "  - {{.Name}}"
        echo ""
        echo "To remove secrets manually, run:"
        echo "  docker secret rm <secret-name>"
        
        # List configs
        echo ""
        echo -e "${YELLOW}Docker configs detected (not automatically removed):${NC}"
        docker config ls --format "  - {{.Name}}"
        echo ""
        echo "To remove configs manually, run:"
        echo "  docker config rm <config-name>"
        
        # Remove files
        echo ""
        rm -rf "$DEPLOY_DIR"
        echo -e "${GREEN}✓ Files removed from $DEPLOY_DIR${NC}"
        
        echo ""
        echo -e "${GREEN}Complete cleanup finished!${NC}"
        echo "Secrets and configs preserved (contain passwords)"
        ;;
    4)
        echo "Cancelled."
        exit 0
        ;;
    *)
        echo "Invalid choice. Cancelled."
        exit 1
        ;;
esac

echo ""
echo "============================================================================"
echo "Cleanup Complete"
echo "============================================================================"
