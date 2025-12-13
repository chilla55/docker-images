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

# Connection settings
connection_cache = on
reset_query_list = 'ABORT; DISCARD ALL'

# Health check settings
health_check_period = 10
health_check_timeout = 5
health_check_user = '@@SR_CHECK_USER@@'
health_check_password = '@@SR_CHECK_PASSWORD@@'
health_check_max_retries = 3
health_check_retry_delay = 1

# Failover and failback
failover_command = ''
follow_primary_command = ''
failback_command = ''

# Connection retry
search_primary_node_timeout = 300

# SSL Configuration for backend connections
ssl = on
ssl_key = '/var/lib/postgresql/server.key'
ssl_cert = '/var/lib/postgresql/server.crt'
ssl_ca_cert = '/var/lib/postgresql/rootca/ca-cert.pem'
ssl_prefer_server_ciphers = on

# Backend SSL mode
backend_use_ssl = on

# Authentication
enable_pool_hba = on
pool_passwd = '/etc/pgpool2/pool_passwd'

# Logging
log_destination = 'stderr'
log_line_prefix = '%m: pid %p: '
log_error_verbosity = verbose

# Backend definitions appended by entrypoint
