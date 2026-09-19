#!/bin/bash
# Maintenance is healthy only while intentionally building. In normal operation
# require both supervised processes and an HTTP response from the application.
set -eu
APP_PORT="${PORT:-3000}"
MAINTENANCE_PORT="${MAINTENANCE_PORT:-3001}"
MODE=$(node -e '
try {
  const state = JSON.parse(require("fs").readFileSync("/tmp/orbat-runtime.json", "utf8"));
  if (state.mode === "building") { console.log("building"); }
  else if (state.mode === "running") {
    if (!state.processes.web) process.exit(1);
    if (process.env.SCHEDULER_ENABLED !== "false" && !state.processes.scheduler) process.exit(1);
    for (const pid of Object.values(state.processes)) {
      if (!pid) process.exit(1);
      process.kill(pid, 0);
    }
    console.log("running");
  } else { process.exit(1); }
} catch (_) { process.exit(1); }
') || exit 1

if [ "$MODE" = "building" ]; then
  curl --max-time 3 -fsS "http://localhost:${MAINTENANCE_PORT}/healthcheck" >/dev/null 2>&1
else
  curl --max-time 3 -fsS "http://localhost:${APP_PORT}/api/health" >/dev/null 2>&1 || \
  curl --max-time 3 -fsS "http://localhost:${APP_PORT}/" >/dev/null 2>&1
fi
