#!/bin/sh
set -e

# Copy base config to temporary location
cp /usr/local/etc/redis/redis.conf /tmp/redis.conf

# Add password if provided
if [ -n "$REDIS_PASSWORD" ]; then
    echo "requirepass $REDIS_PASSWORD" >> /tmp/redis.conf
fi

# Start redis with the generated config
exec redis-server /tmp/redis.conf
