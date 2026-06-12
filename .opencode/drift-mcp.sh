#!/bin/sh
# F4: Controlled Drift Detection Engine (CDDE)
# Reads F3-A observations, computes drift against baseline
# Output: drift state per service + system-wide trend
set -e

DRIFT_DIR="/home/asem/workspace/.opencode/drift"
mkdir -p "$DRIFT_DIR"

python3 << 'PYEOF'
import json, os, datetime, sys
from collections import defaultdict

OBS_DIR = "/home/asem/workspace/.opencode/observations"
DRIFT_DIR = "/home/asem/workspace/.opencode/drift"

# F4.1: Adaptive baseline model
# - Long-term baseline: system boot (first 10 obs) — reference only
# - Short-term baseline: rolling window (last 30 obs) — drift decisions
ROLLING_BASELINE_SIZE = 30  # observations (~30 min)
LONG_TERM_WINDOW = 10  # initial for reference
DRIFT_WINDOWS = [5, 15, 30]  # minutes for current analysis

today = datetime.datetime.utcnow().strftime('%Y-%m-%d')
obs_file = os.path.join(OBS_DIR, f"{today}.jsonl")
if not os.path.exists(obs_file):
    exit(0)

def safe_load_jsonl(path):
    results = []
    if not os.path.exists(path):
        return results
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                results.append(json.loads(line))
            except json.JSONDecodeError as e:
                print(f"F4 WARN: skipping malformed observation — {e}", file=sys.stderr)
    return results

observations = safe_load_jsonl(obs_file)

if len(observations) < LONG_TERM_WINDOW:
    print(json.dumps({"ts": datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ'),
                       "status": "baseline_mode",
                       "observations_current": len(observations),
                       "baseline_needed": LONG_TERM_WINDOW}))
    exit(0)

SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory','chromadb-mcp','github-mcp']
CORE_SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory']

# ─── 1. ESTABLISH BASELINES (dual: long-term + rolling adaptive) ───

def compute_baseline(obs_slice):
    b = {}
    for svc in SERVICES:
        lats = []
        rsss = []
        for obs in obs_slice:
            d = obs.get(svc, {})
            lat = d.get('latency')
            rss = d.get('rss_kb')
            if isinstance(lat, (int,float)):
                lats.append(lat * 1000)
            if isinstance(rss, (int,float)):
                rsss.append(rss)
        b[svc] = {
            "latency_mean_ms": round(sum(lats) / len(lats), 2) if lats else 0,
            "rss_mean_kb": round(sum(rsss) / len(rsss), 0) if rsss else 0,
        }
    return b

# Long-term baseline: first LONG_TERM_WINDOW observations (system boot reference)
long_term_baseline = compute_baseline(observations[:LONG_TERM_WINDOW])

# Short-term adaptive baseline: last ROLLING_BASELINE_SIZE observations (current normal)
n_rolling = min(ROLLING_BASELINE_SIZE, len(observations))
rolling_baseline = compute_baseline(observations[-n_rolling:])

# ─── 2. COMPUTE CURRENT VALUES (last N minutes) ───
now = datetime.datetime.utcnow()
now_ts = now.strftime('%Y-%m-%dT%H:%M:%SZ')

current = {}
for svc in SERVICES:
    current[svc] = {}
    for window_min in DRIFT_WINDOWS:
        cutoff = (now - datetime.timedelta(minutes=window_min)).strftime('%Y-%m-%dT%H:%M:%SZ')
        w_obs = [o for o in observations if o.get('ts', '') >= cutoff]
        
        lats = []
        rsss = []
        restarts_in_window = 0
        state_ok = 0
        state_total = 0
        
        for obs in w_obs:
            d = obs.get(svc, {})
            lat = d.get('latency')
            rss = d.get('rss_kb')
            st = d.get('state', '?')
            nr = d.get('nrestarts', 0)
            
            if isinstance(lat, (int,float)):
                lats.append(lat * 1000)
            if isinstance(rss, (int,float)):
                rsss.append(rss)
            if st == 'active':
                state_ok += 1
            state_total += 1
        
        if state_total > 0:
            stability_pct = round(state_ok / state_total * 100, 1)
        else:
            stability_pct = 100.0

        current[svc][f"{window_min}m"] = {
            "latency_mean_ms": round(sum(lats) / len(lats), 2) if lats else 0,
            "latency_max_ms": round(max(lats), 2) if lats else 0,
            "rss_mean_kb": round(sum(rsss) / len(rsss), 0) if rsss else 0,
            "stability_pct": stability_pct,
            "samples": len(w_obs)
        }

# ─── 3. DRIFT SCORING (adaptive: against rolling baseline) ───
DRIFT_THRESHOLD_LATENCY = 1.8  # ratio > 1.8x rolling baseline = drift (more sensitive)
CRITICAL_THRESHOLD_LATENCY = 3.0  # ratio > 3x = critical
DRIFT_THRESHOLD_STABILITY = 80.0  # stability < 80% = drift

drift_scores = {}
system_drift_count = 0
system_critical_count = 0

for svc in SERVICES:
    rb = rolling_baseline[svc]  # adaptive baseline (rolling window)
    lb = long_term_baseline[svc]  # reference baseline (boot)
    c5 = current.get(svc, {}).get("5m", {})
    
    lat_ratio_vs_rolling = round(c5.get("latency_mean_ms", 0) / max(rb.get("latency_mean_ms", 1), 0.01), 2)
    lat_ratio_vs_long = round(c5.get("latency_mean_ms", 0) / max(lb.get("latency_mean_ms", 1), 0.01), 2)
    stability = c5.get("stability_pct", 100)
    
    # Drift state determined by rolling baseline (adaptive)
    if lat_ratio_vs_rolling >= CRITICAL_THRESHOLD_LATENCY or stability < 50:
        state = "CRITICAL"
        system_critical_count += 1
    elif lat_ratio_vs_rolling >= DRIFT_THRESHOLD_LATENCY or stability < DRIFT_THRESHOLD_STABILITY:
        state = "DRIFT"
        system_drift_count += 1
    elif lat_ratio_vs_rolling > 1.3:
        state = "PRE_DRIFT"
    else:
        state = "STABLE"
    
    drift_scores[svc] = {
        "state": state,
        "latency_ratio_vs_rolling": lat_ratio_vs_rolling,
        "latency_ratio_vs_longterm": lat_ratio_vs_long,
        "latency_rolling_ms": rb.get("latency_mean_ms", 0),
        "latency_longterm_ms": lb.get("latency_mean_ms", 0),
        "latency_current_ms": c5.get("latency_mean_ms", 0),
        "stability_pct_5m": stability,
        "rss_rolling_kb": rb.get("rss_mean_kb", 0),
        "rss_current_kb": c5.get("rss_mean_kb", 0),
    }
    if svc in CORE_SERVICES:
        drift_scores[svc]["_is_core"] = True

# ─── 4. SYSTEM DRIFT ASSESSMENT ───
if system_critical_count > 0:
    system_state = "CRITICAL"
elif system_drift_count > 0:
    system_state = "DRIFT"
elif any(s.get("state") == "PRE_DRIFT" for s in drift_scores.values()):
    system_state = "PRE_DRIFT"
else:
    system_state = "STABLE"

# Core vs Integration health
core_drift = [s for s in CORE_SERVICES if drift_scores.get(s, {}).get("state") in ("DRIFT", "CRITICAL")]
integ_drift = [s for s in ['chromadb-mcp','github-mcp'] if drift_scores.get(s, {}).get("state") in ("DRIFT", "CRITICAL")]

report = {
    "ts": now_ts,
    "observations_total": len(observations),
    "baseline_model": "F4.1 ADAPTIVE (rolling + long-term reference)",
    "long_term_baseline": {
        "samples": LONG_TERM_WINDOW,
        "window": f"{observations[0].get('ts','')} → {observations[LONG_TERM_WINDOW-1].get('ts','')}"
    },
    "rolling_baseline": {
        "samples": n_rolling,
        "window": f"{observations[-n_rolling].get('ts','')} → {observations[-1].get('ts','')}"
    },
    "system_drift_state": system_state,
    "core_drift_count": len(core_drift),
    "integration_drift_count": len(integ_drift),
    "per_service": drift_scores,
    "summary": {
        "stable": sum(1 for s in drift_scores.values() if s["state"] == "STABLE"),
        "pre_drift": sum(1 for s in drift_scores.values() if s["state"] == "PRE_DRIFT"),
        "drift": sum(1 for s in drift_scores.values() if s["state"] == "DRIFT"),
        "critical": sum(1 for s in drift_scores.values() if s["state"] == "CRITICAL"),
    }
}

outfile = os.path.join(DRIFT_DIR, f"{today}.json")
with open(outfile, 'w') as f:
    json.dump(report, f, indent=2)

print(json.dumps(report, indent=2))
PYEOF
