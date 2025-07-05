#!/bin/sh

cd /var/www/pterodactyl || exit 1

# Check .env exists
if [ ! -f .env ]; then
  echo ".env not found"
  exit 1
fi

# Check APP_KEY is set
if ! grep -q '^APP_KEY=base64:' .env; then
  echo "APP_KEY not set"
  exit 1
fi

# Check DB is accessible and migrations applied
php artisan migrate:status > /dev/null 2>&1 || {
  echo "migrate:status failed"
  exit 1
}
