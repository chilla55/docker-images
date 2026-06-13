#!/bin/ash -e

load_secret() {
	var_name="$1"
	secret_path="/run/secrets/${var_name}"

	if [ ! -f "$secret_path" ]; then
		return 0
	fi

	secret_value=$(cat "$secret_path")
	if [ -n "$secret_value" ]; then
		export "${var_name}=${secret_value}"
	fi
}

load_secret APP_KEY
load_secret DB_PASSWORD
load_secret MAIL_PASSWORD

exec /bin/ash /entrypoint.sh "$@"