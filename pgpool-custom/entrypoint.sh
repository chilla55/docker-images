#!/bin/bash
set -euo pipefail

# Default values
PGPOOL_LISTEN_ADDR="0.0.0.0"
PGPOOL_PORT="5432"
PGPOOL_SR_CHECK_USER="${PGPOOL_SR_CHECK_USER:-${REPLICATION_USER:-replicator}}"
PGPOOL_SR_CHECK_PASSWORD="${PGPOOL_SR_CHECK_PASSWORD:-${REPLICATION_PASSWORD:-}}"
PGPOOL_POSTGRES_USERNAME="${PGPOOL_POSTGRES_USERNAME:-${POSTGRES_USER:-postgres}}"
PGPOOL_POSTGRES_PASSWORD="${PGPOOL_POSTGRES_PASSWORD:-${POSTGRES_PASSWORD:-}}"
PGPOOL_ADMIN_USERNAME="${PGPOOL_ADMIN_USERNAME:-${PGPOOL_ADMIN_USER:-admin}}"
PGPOOL_ADMIN_PASSWORD="${PGPOOL_ADMIN_PASSWORD:-${PGPOOL_ADMIN_PASSWORD:-}}"
PGPOOL_ENABLE_LOAD_BALANCING="${PGPOOL_ENABLE_LOAD_BALANCING:-yes}"
PGPOOL_AUTO_FAILBACK="${PGPOOL_AUTO_FAILBACK:-yes}"
PGPOOL_FAILOVER_ON_BACKEND_ERROR="${PGPOOL_FAILOVER_ON_BACKEND_ERROR:-yes}"
PGPOOL_NUM_INIT_CHILDREN="${PGPOOL_NUM_INIT_CHILDREN:-32}"
PGPOOL_MAX_POOL="${PGPOOL_MAX_POOL:-4}"
PGPOOL_BACKEND_NODES="${PGPOOL_BACKEND_NODES:-}"

# Load secrets from *_FILE paths if provided
if [ -n "${PGPOOL_SR_CHECK_PASSWORD_FILE:-}" ] && [ -f "$PGPOOL_SR_CHECK_PASSWORD_FILE" ]; then
    PGPOOL_SR_CHECK_PASSWORD="$(cat "$PGPOOL_SR_CHECK_PASSWORD_FILE")"
fi

if [ -n "${PGPOOL_POSTGRES_PASSWORD_FILE:-}" ] && [ -f "$PGPOOL_POSTGRES_PASSWORD_FILE" ]; then
    PGPOOL_POSTGRES_PASSWORD="$(cat "$PGPOOL_POSTGRES_PASSWORD_FILE")"
fi

if [ -n "${PGPOOL_ADMIN_PASSWORD_FILE:-}" ] && [ -f "$PGPOOL_ADMIN_PASSWORD_FILE" ]; then
    PGPOOL_ADMIN_PASSWORD="$(cat "$PGPOOL_ADMIN_PASSWORD_FILE")"
fi

mkdir -p /etc/pgpool2 /var/log/pgpool /var/run/pgpool /var/lib/pgpool

# Render pool_passwd if password provided
if [[ -n "${PGPOOL_POSTGRES_PASSWORD}" ]]; then
  echo "Rendering pool_passwd for user ${PGPOOL_POSTGRES_USERNAME}"
  (echo "${PGPOOL_POSTGRES_PASSWORD}" | pg_md5 -m) >/tmp/.pgpwd
  HASH=$(cat /tmp/.pgpwd)
  echo "${PGPOOL_POSTGRES_USERNAME}:${HASH}" > /etc/pgpool2/pool_passwd
  rm -f /tmp/.pgpwd
fi

# Parse PGPOOL_BACKEND_NODES like "0:postgresql-primary:5432,1:postgresql-secondary:5432"
BACKEND_LINES=""
IFS=',' read -ra NODES <<< "${PGPOOL_BACKEND_NODES}"
for NODE in "${NODES[@]}"; do
  [[ -z "$NODE" ]] && continue
  IFS=':' read -ra PARTS <<< "$NODE"
  IDX="${PARTS[0]}"; HOST="${PARTS[1]}"; PORT="${PARTS[2]:-5432}"
  BACKEND_LINES+=$'backend_hostname'"${IDX} = '${HOST}'\n"
  BACKEND_LINES+=$'backend_port'"${IDX} = ${PORT}\n"
  BACKEND_LINES+=$'backend_weight'"${IDX} = 1\n"
  BACKEND_LINES+=$'backend_flag'"${IDX} = 'ALLOW_TO_FAILOVER'\n"
  BACKEND_LINES+=$'hostname'"${IDX} = '${HOST}'\n" # compatibility
  BACKEND_LINES+=$'port'"${IDX} = ${PORT}\n" # compatibility
  BACKEND_LINES+=$'weight'"${IDX} = 1\n" # compatibility
  BACKEND_LINES+=$'flag'"${IDX} = 'ALLOW_TO_FAILOVER'\n" # compatibility
done

# Render pgpool.conf from template
sed -e "s|@@LISTEN_ADDR@@|${PGPOOL_LISTEN_ADDR}|g" \
    -e "s|@@PORT@@|${PGPOOL_PORT}|g" \
    -e "s|@@SR_CHECK_USER@@|${PGPOOL_SR_CHECK_USER}|g" \
    -e "s|@@SR_CHECK_PASSWORD@@|${PGPOOL_SR_CHECK_PASSWORD}|g" \
    -e "s|@@LOAD_BALANCE_MODE@@|${PGPOOL_ENABLE_LOAD_BALANCING}|g" \
    -e "s|@@AUTO_FAILBACK@@|${PGPOOL_AUTO_FAILBACK}|g" \
    -e "s|@@FAILOVER_ON_BACKEND_ERROR@@|${PGPOOL_FAILOVER_ON_BACKEND_ERROR}|g" \
    -e "s|@@NUM_INIT_CHILDREN@@|${PGPOOL_NUM_INIT_CHILDREN}|g" \
    -e "s|@@MAX_POOL@@|${PGPOOL_MAX_POOL}|g" \
    /etc/pgpool2/pgpool.conf.tpl > /etc/pgpool2/pgpool.conf

# Append backend lines at the end
printf "%b" "${BACKEND_LINES}" >> /etc/pgpool2/pgpool.conf

# Render pool_hba.conf
sed -e "s|@@SR_CHECK_USER@@|${PGPOOL_SR_CHECK_USER}|g" \
    -e "s|@@PG_USER@@|${PGPOOL_POSTGRES_USERNAME}|g" \
    /etc/pgpool2/pool_hba.conf.tpl > /etc/pgpool2/pool_hba.conf

# Ensure permissions
chown -R postgres:postgres /etc/pgpool2 /var/log/pgpool /var/run/pgpool /var/lib/pgpool || true

# Start pgpool in foreground
exec pgpool -n -f /etc/pgpool2/pgpool.conf
