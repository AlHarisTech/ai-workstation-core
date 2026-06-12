#!/bin/sh
# F3-B: Temporal Failure Attribution Engine (TFAE)
# Reads F3-A JSONL stream, performs temporal diffing,
# detects restart sequences, cascade patterns, latency drift.
# Output: classification events JSONL

set -e

OBS_DIR="/home/asem/workspace/.opencode/observations"
CLS_DIR="/home/asem/workspace/.opencode/classifications"
mkdir -p "$CLS_DIR"

STREAM="${OBS_DIR}/$(date -u +%Y-%m-%d).jsonl"
OUTPUT="${CLS_DIR}/$(date -u +%Y-%m-%d).jsonl"
STATE="${CLS_DIR}/.state.json"

[ ! -f "$STREAM" ] && exit 0

python3 << 'PYEOF'
import json, sys, os

OBS_DIR = "/home/asem/workspace/.opencode/observations"
CLS_DIR = "/home/asem/workspace/.opencode/classifications"
STATE_FILE = os.path.join(CLS_DIR, ".state.json")

today = __import__('datetime').datetime.utcnow().strftime('%Y-%m-%d')
stream_file = os.path.join(OBS_DIR, f"{today}.jsonl")
output_file = os.path.join(CLS_DIR, f"{today}.jsonl")

if not os.path.exists(stream_file):
    sys.exit(0)

with open(stream_file) as f:
    lines = []
    for l in f:
        l = l.strip()
        if not l:
            continue
        try:
            lines.append(json.loads(l))
        except json.JSONDecodeError as e:
            print(f"F3-B WARN: skipping malformed observation — {e}", file=sys.stderr)

if len(lines) < 2:
    sys.exit(0)

# Load previous state
prev_state = {}
if os.path.exists(STATE_FILE):
    with open(STATE_FILE) as f:
        try:
            prev_state = json.load(f)
        except:
            prev_state = {}

latest = lines[-1]
prev = lines[-2] if len(lines) >= 2 else None

SERVICES = ['mcp-filesystem', 'mcp-git', 'mcp-fetch', 'mcp-memory', 'chromadb-mcp', 'github-mcp']
DEPENDENCY_CHAIN = ['mcp-filesystem', 'mcp-git', 'mcp-fetch', 'mcp-memory']

events = []

def classify_restart(svc, cur, prev_data):
    cv = cur.get(svc, {})
    pv = prev_data.get(svc, {})
    nr_cur = cv.get('nrestarts', 0)
    nr_prev = pv.get('nrestarts', 0)
    if isinstance(nr_cur, (int,float)) and isinstance(nr_prev, (int,float)):
        delta = nr_cur - nr_prev
        if delta > 0:
            state_change = cv.get('state', '?')
            return {
                "type": "restart",
                "service": svc,
                "delta": delta,
                "state": state_change,
                "severity": "warning" if svc in ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory'] else "info"
            }
    return None

def classify_latency_drift(svc, cur, prev_data):
    cv = cur.get(svc, {})
    pv = prev_data.get(svc, {})
    cl = cv.get('latency')
    pl = pv.get('latency')
    if isinstance(cl, (int,float)) and isinstance(pl, (int,float)) and pl > 0:
        ratio = cl / pl
        if ratio > 2.0:
            return {
                "type": "latency_drift",
                "service": svc,
                "ratio": round(ratio, 2),
                "from_ms": round(pl * 1000, 2),
                "to_ms": round(cl * 1000, 2),
                "severity": "warning" if ratio > 3.0 else "info"
            }
    return None

def classify_state_change(svc, cur, prev_data):
    cv = cur.get(svc, {})
    pv = prev_data.get(svc, {})
    cs = cv.get('state', '?')
    ps = pv.get('state', '?')
    if cs != ps:
        return {
            "type": "state_change",
            "service": svc,
            "from_state": ps,
            "to_state": cs,
            "severity": "critical" if cs in ['failed','inactive'] else "warning" if cs != 'active' else "info"
        }
    return None

def classify_cascade(cur, prev_data):
    # Detect BindsTo cascade: filesystem restart/fail → git stop → fetch stop → memory stop
    states = {}
    for svc in DEPENDENCY_CHAIN:
        s = cur.get(svc, {}).get('state', '?')
        states[svc] = s

    prev_states = {}
    for svc in DEPENDENCY_CHAIN:
        s = prev_data.get(svc, {}).get('state', '?')
        prev_states[svc] = s

    # Check for cascade pattern
    inactive_chain = []
    for svc in DEPENDENCY_CHAIN:
        if states.get(svc) != 'active':
            inactive_chain.append(svc)

    if len(inactive_chain) >= 2:
        # Check if it's a sequential chain (filesystem → git → fetch → memory)
        order_ok = True
        for i in range(len(inactive_chain) - 1):
            a = DEPENDENCY_CHAIN.index(inactive_chain[i])
            b = DEPENDENCY_CHAIN.index(inactive_chain[i+1])
            if a >= b:
                order_ok = False
                break
        if order_ok:
            return {
                "type": "dependency_cascade",
                "services": inactive_chain,
                "root_cause": inactive_chain[0],
                "severity": "critical",
                "description": f"BindsTo cascade detected: {inactive_chain[0]} → {' → '.join(inactive_chain[1:])}"
            }
    return None

def classify_missing_service(cur, prev_data):
    cur_keys = set(cur.keys()) - {'ts'}
    prev_keys = set(prev_data.keys()) - {'ts'}
    dropped = prev_keys - cur_keys
    added = cur_keys - prev_keys
    events = []
    for svc in dropped:
        events.append({"type": "service_drop", "service": svc, "severity": "critical"})
    for svc in added:
        events.append({"type": "service_added", "service": svc, "severity": "info"})
    return events

if prev:
    # Cascade check first (highest priority)
    cascade = classify_cascade(latest, prev)
    if cascade:
        events.append(cascade)

    # Per-service checks
    for svc in SERVICES:
        r = classify_restart(svc, latest, prev)
        if r:
            events.append(r)
        s = classify_state_change(svc, latest, prev)
        if s:
            events.append(s)
        d = classify_latency_drift(svc, latest, prev)
        if d:
            events.append(d)

    # Missing service checks
    missing = classify_missing_service(latest, prev)
    events.extend(missing)

if events:
    with open(output_file, 'a') as f:
        for ev in events:
            ev['ts'] = latest.get('ts', '')
            f.write(json.dumps(ev) + '\n')
            print(json.dumps(ev))
else:
    print(json.dumps({"ts": latest.get('ts', ''), "type": "no_event", "severity": "info"}))

# Save state for next run
with open(STATE_FILE, 'w') as f:
    json.dump({"last_ts": latest.get('ts', ''), "last_observation": latest}, f)
PYEOF
