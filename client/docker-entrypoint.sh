#!/bin/sh
set -e
export BACKEND_URL="${BACKEND_URL:-http://server:3000}"
envsubst '${BACKEND_URL}' < /etc/nginx/conf.d/default.conf.template > /etc/nginx/conf.d/default.conf
exec nginx -g 'daemon off;'
