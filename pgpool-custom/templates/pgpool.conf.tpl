# Minimal pgpool.conf template rendered from env
listen_addresses = '@@LISTEN_ADDR@@'
port = @@PORT@@

sr_check_user = '@@SR_CHECK_USER@@'
sr_check_password = '@@SR_CHECK_PASSWORD@@'

load_balance_mode = @@LOAD_BALANCE_MODE@@
auto_failback = @@AUTO_FAILBACK@@
failover_on_backend_error = @@FAILOVER_ON_BACKEND_ERROR@@
num_init_children = @@NUM_INIT_CHILDREN@@
max_pool = @@MAX_POOL@@

# Authentication
enable_pool_hba = on
pool_passwd = '/etc/pgpool2/pool_passwd'

# Logging
log_destination = 'stderr'
log_line_prefix = '%m: pid %p: '
log_error_verbosity = verbose

# Backend definitions appended by entrypoint
