#!/bin/sh
# Global Autonomy Governor (GAG) v1 — Hardening Layer
# Monitors intent streams, detects runaway patterns, provides circuit breaker.
# This is the LAST architectural layer before Level 3.
#
# Usage:
#   ./gag-mcp.sh --status     # hardening health + metrics
#   ./gag-mcp.sh --check      # run hardening check, optionally force-reset

set -e

GAG_DIR="/home/asem/workspace/.opencode/gag"
EMCTL_T_DIR="/home/asem/workspace/.opencode/emctl-t"
mkdir -p "$GAG_DIR"

STATUS_ONLY=""
CHECK=""

while [ $# -gt 0 ]; do
    case "$1" in
        --status) STATUS_ONLY="1"; shift ;;
        --check)  CHECK="1"; shift ;;
        *) echo "Usage: $0 [--status|--check]"; exit 1 ;;
    esac
done

STATUS_ONLY="$STATUS_ONLY" CHECK="$CHECK" python3 << 'PYEOF'
import json, os, sys, datetime

GAG_DIR      = "/home/asem/workspace/.opencode/gag"
EMCTL_T_DIR  = "/home/asem/workspace/.opencode/emctl-t"
STICKY_PATH  = os.path.join(GAG_DIR, "governor.json")
HISTORY_PATH = os.path.join(GAG_DIR, "history.json")

STATUS_ONLY = os.environ.get('STATUS_ONLY', '')
CHECK       = os.environ.get('CHECK', '')

now_ts = datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')
today  = datetime.datetime.utcnow().strftime('%Y-%m-%d')

def load_json(path):
    if not os.path.exists(path):
        return {}
    with open(path) as f:
        return json.load(f)

def save_json(path, data):
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)

# ─── Load governor state ───
governor = load_json(STICKY_PATH)
if not governor:
    governor = {
        "ts": now_ts,
        "circuit_breaker": "CLOSED",
        "mode_override": None,
        "last_forced_reset": None,
        "consecutive_anomalies": 0,
        "total_overrides": 0,
    }

history = load_json(HISTORY_PATH)
if not history:
    history = {"ts": now_ts, "snapshots": []}

# ─── Load intent + failure data ───
def load_jsonl_latest(dirname, suffix=".jsonl"):
    path = os.path.join(EMCTL_T_DIR, dirname, f"{today}{suffix}")
    entries = []
    if os.path.exists(path):
        with open(path) as f:
            for line in f:
                try:
                    entries.append(json.loads(line))
                except:
                    pass
    return entries

auth_entries = load_jsonl_latest("authorizations")
intents      = load_jsonl_latest("intents")

def load_domain_degraded(domain):
    path = os.path.join(EMCTL_T_DIR, domain, "degraded.json")
    return load_json(path)

core_state = load_domain_degraded("core")
int_state  = load_domain_degraded("integration")

# ─── If --status only ───
if STATUS_ONLY:
    result = {
        "ts": now_ts,
        "circuit_breaker": governor.get("circuit_breaker", "CLOSED"),
        "mode_override": governor.get("mode_override"),
        "consecutive_anomalies": governor.get("consecutive_anomalies", 0),
        "total_overrides": governor.get("total_overrides", 0),
        "last_forced_reset": governor.get("last_forced_reset"),
        "intents_24h": len(intents),
        "authorizations_24h": len(auth_entries),
        "core_state": core_state.get("state", "UNKNOWN"),
        "int_state": int_state.get("state", "UNKNOWN"),
        "hardening_status": "ACTIVE",
    }
    print(json.dumps(result, indent=2))
    sys.exit(0)

# ─── Hardening check ───
anomalies = []
triggers = []

# H1: Intent frequency anomaly (too many in window)
RECENT_CUTOFF = (datetime.datetime.utcnow() - datetime.timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%SZ')
recent_intents = [i for i in intents if i.get('ts', '') >= RECENT_CUTOFF]
if len(recent_intents) >= 5:
    anomalies.append(f"{len(recent_intents)} intents in 5min — possible runaway")
    triggers.append("INTENT_FLOOD")

# H2: Cyclic restart pattern (same service restarted >3 times in 15min)
restart_counts = {}
for i in intents:
    if i.get('intent_type') == 'RESTART':
        svc = i.get('target_service', 'unknown')
        restart_counts[svc] = restart_counts.get(svc, 0) + 1
for svc, count in restart_counts.items():
    if count >= 3:
        anomalies.append(f"{svc} restarted {count}x in 24h — cycling")
        triggers.append(f"CYCLE_{svc}")

# H3: Degraded oscillation (high transition count)
core_transitions = core_state.get('transitions', [])
RECENT_TRANS_CUTOFF = (datetime.datetime.utcnow() - datetime.timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M:%SZ')
recent_trans = [t for t in core_transitions if t.get('ts', '') >= RECENT_TRANS_CUTOFF]
if len(recent_trans) >= 3:
    anomalies.append(f"{len(recent_trans)} core state transitions in 1h — oscillation")
    triggers.append("DEGRADED_OSCILLATION")

# H4: Authorization denial spike
recent_auth_denials = [
    a for a in auth_entries[-20:]
    if a.get('decision') == 'DENIED' and a.get('ts', '') >= RECENT_CUTOFF
]
if len(recent_auth_denials) >= 4:
    anomalies.append(f"{len(recent_auth_denials)} DENIED in 5min — authorization storm")
    triggers.append("AUTH_STORM")

# H5: Confidence decay (intents getting less confident)
confidences = [
    i.get('confidence', 1.0) for i in intents[-10:]
    if i.get('intent_type') == 'RESTART'
]
if len(confidences) >= 3:
    trend = confidences[-1] - confidences[0]
    if trend < -0.3:
        anomalies.append(f"confidence decay {confidences[0]}→{confidences[-1]} — degrading reliability")
        triggers.append("CONFIDENCE_DECAY")

# ─── Act on anomalies ───
if anomalies:
    governor["consecutive_anomalies"] += 1
    governor["ts"] = now_ts

    if governor["consecutive_anomalies"] >= 3 and governor.get("circuit_breaker") == "CLOSED":
        # TRIP circuit breaker → force OBSERVE_ONLY
        governor["circuit_breaker"] = "TRIPPED"
        governor["mode_override"] = "OBSERVE_ONLY"
        governor["last_forced_reset"] = now_ts
        governor["total_overrides"] += 1
        circuit_action = "TRIPPED — forced OBSERVE_ONLY mode"
    else:
        # Warning only
        circuit_action = "WARN — monitoring"
elif governor.get("consecutive_anomalies", 0) > 0:
    # Recovery: no anomalies this cycle
    governor["consecutive_anomalies"] = max(0, governor["consecutive_anomalies"] - 1)
    if governor["consecutive_anomalies"] == 0 and governor.get("circuit_breaker") == "TRIPPED":
        governor["circuit_breaker"] = "CLOSED"
        governor["mode_override"] = None
    circuit_action = "HEALTHY"
else:
    circuit_action = "HEALTHY"

save_json(STICKY_PATH, governor)

# ─── Log snapshot ───
snapshot = {
    "ts": now_ts,
    "circuit_breaker": governor["circuit_breaker"],
    "anomalies": anomalies,
    "triggers": triggers,
    "action": circuit_action,
    "intent_count_5min": len(recent_intents),
    "auth_denials_5min": len(recent_auth_denials),
    "core_transitions_1h": len(recent_trans),
    "restart_cycles": {svc: c for svc, c in restart_counts.items() if c >= 2},
}
history["snapshots"] = (history.get("snapshots", []) + [snapshot])[-100:]
save_json(HISTORY_PATH, history)

# ─── Report ───
report = {
    "ts": now_ts,
    "circuit_breaker": governor["circuit_breaker"],
    "mode_override": governor.get("mode_override"),
    "consecutive_anomalies": governor.get("consecutive_anomalies", 0),
    "anomalies_found": len(anomalies),
    "anomalies": anomalies,
    "triggers": triggers,
    "action": circuit_action,
    "intents_checked": len(intents),
    "authorizations_checked": len(auth_entries),
    "hardening_active": True,
    "_version": "GAG v1 — Autonomy Hardening Layer",
}

print(json.dumps(report, indent=2))
PYEOF