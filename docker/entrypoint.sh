#!/bin/sh
set -eu

BACKEND_PID=""
NGINX_PID=""

cleanup() {
  for p in $NGINX_PID $BACKEND_PID; do
    [ -n "$p" ] || continue
    kill -TERM "$p" 2>/dev/null || true
  done
  for p in $NGINX_PID $BACKEND_PID; do
    [ -n "$p" ] || continue
    wait "$p" 2>/dev/null || true
  done
}

handle_shutdown() {
  cleanup
  exit 0
}

trap handle_shutdown INT TERM

/app/app &
BACKEND_PID=$!

i=0
while [ "$i" -lt 120 ]; do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    wait "$BACKEND_PID" || exit $?
  fi
  if curl -sf -o /dev/null --connect-timeout 1 --max-time 2 http://127.0.0.1:3000/ 2>/dev/null; then
    break
  fi
  i=$((i + 1))
  sleep 1
done

if ! curl -sf -o /dev/null --connect-timeout 1 --max-time 2 http://127.0.0.1:3000/ 2>/dev/null; then
  echo "entrypoint: backend did not become ready on :3000 within 120s." >&2
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    wait "$BACKEND_PID" || exit $?
  fi
  kill -TERM "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  exit 1
fi

nginx -g "daemon off;" &
NGINX_PID=$!

while true; do
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    ST=0
    wait "$BACKEND_PID" || ST=$?
    echo "entrypoint: backend exited with status $ST, stopping container." >&2
    cleanup
    exit "$ST"
  fi
  if ! kill -0 "$NGINX_PID" 2>/dev/null; then
    echo "entrypoint: nginx exited unexpectedly." >&2
    wait "$NGINX_PID" 2>/dev/null || true
    cleanup
    exit 1
  fi
  sleep 1
done
