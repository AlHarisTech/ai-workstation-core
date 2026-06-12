#!/bin/sh
# F3-A: Service Health Collector
# Polls systemd state + HTTP latency for all MCP services
# Output: JSON to stdout, appended to .opencode/observations/YYYY-MM-DD.jsonl

set -e

TS=$(date -u +%Y-%m-%dT%H:%M:%SZ)
OBS_DIR="/home/asem/workspace/.opencode/observations"
mkdir -p "$OBS_DIR"

SERVICES="mcp-filesystem:4110 mcp-git:4111 mcp-fetch:4112 mcp-memory:4113 chromadb-mcp:4114 github-mcp:4115"

JSON="{\"ts\":\"$TS\""

for entry in $SERVICES; do
  name="${entry%%:*}"
  port="${entry##*:}"

  state=$(systemctl --user show -p ActiveState "$name.service" 2>/dev/null | cut -d= -f2 || echo "unknown")
  substate=$(systemctl --user show -p SubState "$name.service" 2>/dev/null | cut -d= -f2 || echo "unknown")
  pid=$(systemctl --user show -p MainPID "$name.service" 2>/dev/null | cut -d= -f2 || echo "0")
  nrestarts=$(systemctl --user show -p NRestarts "$name.service" 2>/dev/null | cut -d= -f2 || echo "0")

  if [ "$pid" ] && [ "$pid" -gt 1 ] 2>/dev/null; then
    rss=$(ps -o rss= -p "$pid" 2>/dev/null | tr -d ' ' || echo "0")
  else
    rss="0"
    pid="0"
  fi

  lat=$(curl -s -o /dev/null -w "%{time_total}" --max-time 3 -X POST "http://localhost:$port" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' 2>/dev/null || echo "null")

  tools=$(curl -s --max-time 3 -X POST "http://localhost:$port" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' 2>/dev/null | \
    python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('result',{}).get('tools',[])))" 2>/dev/null || echo "null")

  JSON="${JSON},\"${name}\":{\"state\":\"${state}\",\"substate\":\"${substate}\",\"pid\":${pid},\"rss_kb\":${rss},\"nrestarts\":${nrestarts},\"latency\":${lat},\"tools\":${tools}}"
done

JSON="${JSON}}"

# Validate JSON before writing (producer gate)
echo "$JSON" | python3 -c "
import json,sys
try:
    json.loads(sys.stdin.read())
    sys.exit(0)
except json.JSONDecodeError as e:
    print(f'F3-A ERROR: invalid observation JSON — {e}', file=sys.stderr)
    sys.exit(1)
" 2>/dev/null && echo "$JSON" >> "${OBS_DIR}/$(date -u +%Y-%m-%d).jsonl"
echo "$JSON"
