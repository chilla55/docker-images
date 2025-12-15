#!/bin/bash
# Post-start script to move RSA keys to Storage Box after Vaultwarden generates them

set -e

echo "Waiting for Vaultwarden to generate RSA keys..."

# Wait for RSA key generation (max 60 seconds)
for i in {1..60}; do
    if [ -f "/data/rsa_key.pem" ] && [ -f "/data/rsa_key.pub.pem" ]; then
        echo "RSA keys found!"
        break
    fi
    sleep 1
done

# Move keys to Storage Box if not already there
if [ -d "/data/keys" ] && [ -f "/data/rsa_key.pem" ]; then
    if [ ! -f "/data/keys/rsa_key.pem" ]; then
        echo "Moving RSA keys to Storage Box..."
        
        # Copy keys to Storage Box
        cp /data/rsa_key.pem /data/keys/rsa_key.pem
        cp /data/rsa_key.pub.pem /data/keys/rsa_key.pub.pem
        
        # Set permissions
        chmod 600 /data/keys/rsa_key.pem
        chmod 644 /data/keys/rsa_key.pub.pem
        
        # Remove originals and create symlinks
        rm -f /data/rsa_key.pem /data/rsa_key.pub.pem
        ln -s /data/keys/rsa_key.pem /data/rsa_key.pem
        ln -s /data/keys/rsa_key.pub.pem /data/rsa_key.pub.pem
        
        echo "RSA keys moved to Storage Box and symlinked"
    else
        echo "RSA keys already exist on Storage Box"
    fi
fi

echo "RSA key management complete"
