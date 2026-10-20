#!/bin/sh
# Starts the node given as arguments and, every 10 seconds, writes its
# resident memory (kB) to $MEM_FILE for the app page. The node is no longer
# PID 1, so SIGTERM/SIGINT are passed on and its clean shutdown is awaited.
"$@" &
pid=$!
trap 'kill -TERM "$pid" 2>/dev/null' TERM INT
if [ -n "${MEM_FILE:-}" ]; then
  (
    while kill -0 "$pid" 2>/dev/null; do
      while read -r k v _; do
        if [ "$k" = "VmRSS:" ]; then
          echo "$v" > "$MEM_FILE.tmp" && mv "$MEM_FILE.tmp" "$MEM_FILE"
        fi
      done < "/proc/$pid/status"
      sleep 10
    done
  ) 2>/dev/null &
fi
wait "$pid"
rc=$?
while kill -0 "$pid" 2>/dev/null; do
  wait "$pid"
  rc=$?
done
exit "$rc"
