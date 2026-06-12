#!/bin/sh
# Stress Injection Layer — Controlled Chaos for Level 2 Agent Validation
# Injects synthetic drift into the F4 signal path
# Does NOT touch real services — only signal-level injection
set -e

STRESS_DIR="/home/asem/workspace/.opencode/stress"
mkdir -p "$STRESS_DIR"

python3 << 'PYEOF'
import json, os, datetime, random, copy

DRIFT_DIR = "/home/asem/workspace/.opencode/drift"
STRESS_DIR = "/home/asem/workspace/.opencode/stress"

today = datetime.datetime.utcnow().strftime('%Y-%m-%d')
now_ts = datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')

# Load current drift state
current_drift_path = os.path.join(DRIFT_DIR, f"{today}.json")
if not os.path.exists(current_drift_path):
    print(json.dumps({"error": "no drift baseline available", "ts": now_ts}))
    exit(0)

with open(current_drift_path) as f:
    base_drift = json.load(f)

SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory','chromadb-mcp','github-mcp']
CORE_SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory']

# ─── STRESS SCENARIOS ───
# Each scenario produces a synthetic F4 drift report

def scenario_single_drift():
    """One core service enters DRIFT state"""
    d = copy.deepcopy(base_drift)
    d["ts"] = now_ts
    d["scenario"] = "single_drift"
    d["system_drift_state"] = "DRIFT"

    target = "mcp-git"
    if "per_service" in d and target in d["per_service"]:
        d["per_service"][target]["state"] = "DRIFT"
        d["per_service"][target]["latency_ratio_vs_rolling"] = 2.1
        d["per_service"][target]["latency_current_ms"] = round(
            d["per_service"][target].get("latency_rolling_ms", 1.0) * 2.1, 2
        )
        d["per_service"][target]["stability_pct_5m"] = 85.0

    d["core_drift_count"] = 1
    d["summary"]["drift"] = 1
    d["summary"]["stable"] = 5
    return d


def scenario_cascade():
    """Simulate BindsTo cascade: filesystem fails → git/fetch/memory stop"""
    d = copy.deepcopy(base_drift)
    d["ts"] = now_ts
    d["scenario"] = "cascade"
    d["system_drift_state"] = "CRITICAL"

    for svc in CORE_SERVICES:
        if svc in d.get("per_service", {}):
            d["per_service"][svc]["state"] = "CRITICAL"
            d["per_service"][svc]["latency_ratio_vs_rolling"] = 5.0
            d["per_service"][svc]["stability_pct_5m"] = 0.0

    d["core_drift_count"] = 4
    d["integration_drift_count"] = 0
    d["summary"]["critical"] = 4
    d["summary"]["stable"] = 2
    return d


def scenario_latency_spike():
    """All core services show sudden latency increase"""
    d = copy.deepcopy(base_drift)
    d["ts"] = now_ts
    d["scenario"] = "latency_spike"
    d["system_drift_state"] = "DRIFT"

    for svc in CORE_SERVICES:
        if svc in d.get("per_service", {}):
            d["per_service"][svc]["state"] = "DRIFT"
            d["per_service"][svc]["latency_ratio_vs_rolling"] = 2.5
            d["per_service"][svc]["stability_pct_5m"] = 75.0

    d["core_drift_count"] = 4
    d["summary"]["drift"] = 4
    d["summary"]["stable"] = 2
    return d


def scenario_integration_failure():
    """Only integration services (chromadb/github) fail — core stays stable"""
    d = copy.deepcopy(base_drift)
    d["ts"] = now_ts
    d["scenario"] = "integration_failure"
    d["system_drift_state"] = "DRIFT"

    for svc in ['chromadb-mcp', 'github-mcp']:
        if svc in d.get("per_service", {}):
            d["per_service"][svc]["state"] = "CRITICAL"
            d["per_service"][svc]["latency_ratio_vs_rolling"] = 4.0
            d["per_service"][svc]["stability_pct_5m"] = 30.0

    d["core_drift_count"] = 0
    d["integration_drift_count"] = 2
    d["summary"]["critical"] = 2
    d["summary"]["stable"] = 4
    return d


# ─── GENERATE ALL SCENARIOS ───
scenarios = {
    "single_drift": scenario_single_drift(),
    "cascade": scenario_cascade(),
    "latency_spike": scenario_latency_spike(),
    "integration_failure": scenario_integration_failure(),
}

# Save each scenario
for name, data in scenarios.items():
    path = os.path.join(STRESS_DIR, f"{name}.json")
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)

print(json.dumps({
    "ts": now_ts,
    "scenarios_generated": list(scenarios.keys()),
    "instruction": "Run './.opencode/agent-mcp.sh --stress <scenario>' to test Level 2 Agent under stress",
}, indent=2))
PYEOF
