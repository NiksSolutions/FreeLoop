#!/bin/sh
# NFR-PERF-004: idle memory of the app with ten idle sessions, external
# harness processes left out. Starts the built app on a scratch data folder,
# creates ten sessions through the command line (EXT-004), runs one demo turn
# in each, waits for the app to settle and measures the physical footprint
# (what Activity Monitor shows as Memory) of the app's own process and of the
# WebKit processes that appeared with it, then counts the processes ten sessions
# on the reference agent hold before and after the idle time (OQ-053). Exits 1
# above the budget, and when a process could not be measured.
#
#   scripts/memcheck.sh [path to the app binary] [budget in MB]
#
# The app's window opens while it runs and closes when it ends.
set -eu

APP=${1:-bin/Freeloop.app/Contents/MacOS/Freeloop}
BUDGET_MB=${2:-300}
SETTLE=${SETTLE:-20}
[ -x "$APP" ] || { echo "no app at $APP; run make package first" >&2; exit 2; }

DATA=$(mktemp -d "${TMPDIR:-/tmp}/freeloop-memcheck.XXXXXX")
export FREELOOP_HOME="$DATA/home" FREELOOP_DEV=1
mkdir -p "$FREELOOP_HOME"
# The demo's one command runs without asking; its file edit is allowed by
# accept_edits.
cat > "$FREELOOP_HOME/rules.yaml" <<'EOF'
version: 1
rules:
  - {effect: allow, tool: shell, command: "ls -la freeloop-demo.txt && cat freeloop-demo.txt"}
EOF
cat > "$FREELOOP_HOME/config.yaml" <<'EOF'
ui:
  onboarded: true
EOF

webkit() { pgrep -f 'com.apple.WebKit' | sort; }
before=$(webkit || true)

"$APP" >"$DATA/app.log" 2>&1 &
pid=$!
cleanup() { kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -rf "$DATA"; }
trap cleanup EXIT INT TERM

# The command line answers once the app's socket is up.
i=0
until "$APP" sessions >/dev/null 2>&1; do
  i=$((i + 1))
  [ $i -lt 100 ] || { echo "the app did not start; see $DATA/app.log" >&2; exit 2; }
  sleep 0.2
done

n=1
while [ $n -le 10 ]; do
  mkdir -p "$DATA/projects/p$n"
  sid=$("$APP" new "$DATA/projects/p$n" -title "Idle $n" -provider demo -model scripted -mode accept_edits)
  "$APP" prompt "$sid" "go" >/dev/null
  n=$((n + 1))
done

sleep "$SETTLE"

# footprint prints "Footprint: <n> KB|MB|GB" per process, and skips one it
# cannot find with a message and status 0. mb prints the total in MB only when
# every process given was measured: an empty or failed measurement once read as
# 0 MB, and the check passed on it.
mb() { # process ids
  want=$#
  for p; do
    set -- "$@" -p "$p"
    shift
  done
  footprint "$@" 2>/dev/null | awk -v want="$want" '/Footprint:/ && !/^Summary/ {
    for (i = 1; i <= NF; i++) if ($i == "Footprint:") { v = $(i + 1); u = $(i + 2) }
    if (u == "KB") v /= 1024; else if (u == "GB") v *= 1024; else if (u == "B") v /= 1048576
    t += v; got++ } END { if (want > 0 && got == want) printf "%.0f", t }'
}
# A measurement is a whole number of MB above zero; anything else is a failure,
# never a pass.
measured() { # what was measured, the measurement
  case "$2" in
  '' | *[!0-9]* | 0)
    echo "could not measure $1 (footprint gave \"$2\"), so there is nothing to hold against the budget" >&2
    exit 1
    ;;
  esac
}
app_mb=$(mb "$pid")
measured "the app process" "$app_mb"
if [ -n "$before" ]; then
  helpers=$(webkit | grep -vxF "$before" || true)
else
  helpers=$(webkit || true)
fi
# The window always starts WebKit processes of its own; finding none means their
# share would be left out of the total.
[ -n "$helpers" ] || { echo "found no WebKit process started with the app, so the window's memory cannot be measured" >&2; exit 1; }
# shellcheck disable=SC2086 # one process id per word
helper_mb=$(mb $helpers)
measured "the WebKit processes" "$helper_mb"
total=$((app_mb + helper_mb))

echo "app process:        ${app_mb} MB"
echo "WebKit processes:   ${helper_mb} MB ($(printf '%s\n' $helpers | grep -c . || true) started with the app)"
echo "total, 10 idle sessions: ${total} MB (budget ${BUDGET_MB} MB)"

# OQ-053: ten sessions on the reference agent hold ten processes until the idle time
# passes (the shortest a setting allows, a minute), then none. Runs in the engine's
# test binary, which serves the reference agent; set NO_IDLE_CHECK=1 to skip it.
if [ -z "${NO_IDLE_CHECK:-}" ]; then
  idle=$(FREELOOP_MEMCHECK=1 go test ./internal/engine -run '^TestMemcheckIdleStop$' -count=1 -v 2>&1) || { echo "$idle" >&2; echo "the idle stop check failed" >&2; exit 1; }
  echo "$idle" | grep "reference agent processes"
fi
[ "$total" -le "$BUDGET_MB" ] || { echo "over the budget" >&2; exit 1; }
