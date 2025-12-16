# Deploy NGINX Site Configurations

## Step 1: Copy sites to storagebox on srv0/srv1

Run these commands on **srv0** or **srv1**:

```bash
# Create sites directory if it doesn't exist
sudo mkdir -p /mnt/storagebox/sites

# Upload the site configs (from your local machine)
scp nginx/sites-available/pterodactyl.conf srv0:/tmp/
scp nginx/sites-available/vaultwarden.conf srv0:/tmp/

# On srv0, move to storagebox
sudo mv /tmp/pterodactyl.conf /mnt/storagebox/sites/
sudo mv /tmp/vaultwarden.conf /mnt/storagebox/sites/
sudo chmod 644 /mnt/storagebox/sites/*.conf

# Verify files are there
ls -la /mnt/storagebox/sites/
```

## Step 2: Check nginx logs

The watch-sites-reload.sh script checks every 30 seconds for new sites and will automatically:
1. Check if upstreams are resolvable
2. Create symlinks in /etc/nginx/sites-enabled/
3. Test nginx config
4. Reload nginx

Monitor the process:
```bash
docker service logs -f nginx_nginx
```

## Step 3: Verify site is enabled

After ~30 seconds, check if the site was enabled:
```bash
docker exec $(docker ps -q -f name=nginx_nginx) ls -la /etc/nginx/sites-enabled/
```

## Troubleshooting

If sites aren't auto-enabling, check:

1. **Files are on storagebox:**
   ```bash
   ls -la /mnt/storagebox/sites/
   ```

2. **Upstream is resolvable from nginx container:**
   ```bash
   docker exec $(docker ps -q -f name=nginx_nginx) getent hosts pterodactyl_panel
   ```

3. **Watch nginx logs for errors:**
   ```bash
   docker service logs nginx_nginx --tail 100
   ```
