# Hetzner Storage Box - fstab Setup

## Quick Setup (on srv2)

### 1. Create Credential File
```bash
sudo cat > /root/.storagebox-creds << 'EOF'
username=u515899
password=YOUR_PASSWORD_HERE
EOF

sudo chmod 600 /root/.storagebox-creds
```

### 2. Create Mount Point
```bash
sudo mkdir -p /mnt/storagebox
```

### 3. Add to /etc/fstab
```bash
sudo bash -c 'cat >> /etc/fstab << '\''EOF'\''
# Hetzner Storage Box
//u515899.your-storagebox.de/backup /mnt/storagebox smb3 credentials=/root/.storagebox-creds,vers=3.0,seal,nodfs,noserverino,nounix,uid=0,gid=0,file_mode=0755,dir_mode=0755,x-systemd.automount 0 0
EOF'
```

### 4. Mount
```bash
sudo mount -a
```

### 5. Verify
```bash
mount | grep storagebox
ls -la /mnt/storagebox/certs/
```

## Troubleshooting

### Test mount manually
```bash
sudo mount -t smb3 //u515899.your-storagebox.de/backup /mnt/storagebox \
  -o credentials=/root/.storagebox-creds,vers=3.0,seal,nodfs,noserverino,nounix
```

### Check fstab errors
```bash
sudo findmnt --verify
```

### View current mounts
```bash
mount | grep -E "(storagebox|smb3)"
```

### Unmount
```bash
sudo umount /mnt/storagebox
```

## fstab Entry Explanation
- `smb3` - Protocol (SMB3 is better than cifs)
- `credentials=/root/.storagebox-creds` - Credentials file
- `vers=3.0` - SMB version
- `seal` - Encrypt traffic
- `nodfs,noserverino,nounix` - Hetzner compatibility options
- `uid=0,gid=0` - Owner (root)
- `file_mode=0755,dir_mode=0755` - Permissions
- `x-systemd.automount` - Auto-mount at boot via systemd
- `0 0` - No dump/fsck (last two columns)
