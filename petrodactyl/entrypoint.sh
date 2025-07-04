#!/bin/bash
set -e

APP_DIR="/var/www/pterodactyl"
cd "$APP_DIR"

### 📥 Download Pterodactyl panel if not already present
if [ ! -f artisan ]; then
    echo "📥 Downloading latest Pterodactyl Panel..."
    curl -Lo /tmp/panel.tar.gz https://github.com/pterodactyl/panel/releases/latest/download/panel.tar.gz
    tar -xzf /tmp/panel.tar.gz -C "$APP_DIR" --strip-components=1
    rm /tmp/panel.tar.gz
    echo "✅ Panel files extracted."
else
    echo "📁 Existing panel files found — skipping download."
fi

### 📝 Create .env if missing
if [ ! -f .env ]; then
    echo "📄 .env not found, copying from example..."
    cp .env.example .env
fi

### 📦 Install Composer dependencies if not already done
if [ ! -d vendor ]; then
    echo "📦 Running composer install..."
    COMPOSER_ALLOW_SUPERUSER=1 composer install --no-dev --optimize-autoloader
else
    echo "✅ Composer dependencies already installed."
fi

### 🔑 Generate app key if missing
if ! grep -q "^APP_KEY=" .env || grep -q "^APP_KEY=$" .env; then
    echo "🗝️  Generating Laravel APP_KEY..."
    php artisan key:generate --force
else
    echo "🔐 APP_KEY already set — skipping."
fi

### ✅ Run CMD passed to container
echo "🚀 Starting container with command: $@"
exec "$@"
