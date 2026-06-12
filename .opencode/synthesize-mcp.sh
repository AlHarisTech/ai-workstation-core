#!/bin/sh
# F3-C: Incident Abstraction Layer
# Reads F3-A (observations) + F3-B (classifications)
# Produces: drift curves, incident clusters, health state
set -e

mkdir -p /home/asem/workspace/.opencode/reports

python3 << 'PYEOF'
import json, os, datetime, sys
from collections import defaultdict

OBS_DIR = "/home/asem/workspace/.opencode/observations"
CLS_DIR = "/home/asem/workspace/.opencode/classifications"
RPT_DIR = "/home/asem/workspace/.opencode/reports"

today = datetime.datetime.utcnow().strftime('%Y-%m-%d')
obs_file = os.path.join(OBS_DIR, f"{today}.jsonl")
cls_file = os.path.join(CLS_DIR, f"{today}.jsonl")

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
                print(f"F3-C WARN: skipping malformed record — {e}", file=sys.stderr)
    return results

observations = safe_load_jsonl(obs_file)
events = safe_load_jsonl(cls_file)

SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory','chromadb-mcp','github-mcp']
CORE_SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory']

now = datetime.datetime.utcnow()

# ─── 1. WINDOW ANALYSIS ───
# Windows: 5-min, 15-min, full-session

def window_observations(obs_list, minutes):
    cutoff = now - datetime.timedelta(minutes=minutes)
    cutoff_str = cutoff.strftime('%Y-%m-%dT%H:%M:%SZ')
    return [o for o in obs_list if o.get('ts', '') >= cutoff_str]

def window_events(ev_list, minutes):
    cutoff = now - datetime.timedelta(minutes=minutes)
    cutoff_str = cutoff.strftime('%Y-%m-%dT%H:%M:%SZ')
    return [e for e in ev_list if e.get('ts', '') >= cutoff_str]

w5_obs = window_observations(observations, 5)
w15_obs = window_observations(observations, 15)
w5_ev = window_events(events, 5)
w15_ev = window_events(events, 15)

# ─── 2. DRIFT CURVES (latency, RSS, restart rate) ───

drift = {}
for svc in SERVICES:
    drift[svc] = {
        "latency_ms": [],
        "rss_kb": [],
        "nrestarts": [],
        "state": []
    }

for obs in observations:
    ts = obs.get('ts', '')
    for svc in SERVICES:
        d = obs.get(svc, {})
        lat = d.get('latency')
        rss = d.get('rss_kb')
        nr = d.get('nrestarts', 0)
        st = d.get('state', '?')
        if isinstance(lat, (int,float)):
            drift[svc]["latency_ms"].append({"t": ts, "v": round(lat * 1000, 2)})
        if isinstance(rss, (int,float)):
            drift[svc]["rss_kb"].append({"t": ts, "v": rss})
        if isinstance(nr, (int,float)):
            drift[svc]["nrestarts"].append({"t": ts, "v": nr})
        if st:
            drift[svc]["state"].append({"t": ts, "v": st})

# Compute drift trends
drift_trends = {}
for svc in SERVICES:
    lats = [p["v"] for p in drift[svc]["latency_ms"]]
    nr_values = [p["v"] for p in drift[svc]["nrestarts"]]
    trend = {}
    if len(lats) >= 3:
        trend["latency_avg"] = round(sum(lats) / len(lats), 2)
        trend["latency_min"] = round(min(lats), 2)
        trend["latency_max"] = round(max(lats), 2)
        # Drift: ratio of max to min (excluding zeros)
        if trend["latency_min"] > 0 and trend["latency_max"] > 0:
            trend["latency_drift_ratio"] = round(trend["latency_max"] / trend["latency_min"], 2)
    if nr_values:
        trend["restarts_total"] = nr_values[-1] - nr_values[0] if len(nr_values) >= 2 else 0
        trend["restarts_per_hour"] = round(trend["restarts_total"] / max(len(observations) / 60, 0.01), 2)
    drift_trends[svc] = trend

# ─── 3. INCIDENT CLUSTERING ───
# Group events within 90s windows into incidents

incidents = []
if len(events) >= 2:
    # Sort by timestamp
    sorted_ev = sorted(events, key=lambda e: e.get('ts', ''))
    current_incident = None
    
    for ev in sorted_ev:
        if ev.get('type') == 'no_event':
            continue
        et = ev.get('ts', '')
        if current_incident is None:
            current_incident = {
                "start_ts": et,
                "end_ts": et,
                "events": [ev],
                "services": {ev.get('service', ev.get('root_cause', '?')) for ev in [ev]},
                "types": {ev.get('type', '?')},
                "max_severity": ev.get('severity', 'info'),
            }
        else:
            # If within 90s, merge
            last_ts = current_incident["end_ts"]
            try:
                t1 = datetime.datetime.fromisoformat(et.replace('Z','+00:00'))
                t2 = datetime.datetime.fromisoformat(last_ts.replace('Z','+00:00'))
                gap = (t1 - t2).total_seconds()
                if gap <= 90:
                    current_incident["end_ts"] = et
                    current_incident["events"].append(ev)
                    svc = ev.get('service', ev.get('root_cause', '?'))
                    if svc:
                        current_incident["services"].add(svc)
                    current_incident["types"].add(ev.get('type', '?'))
                    sev = ev.get('severity', 'info')
                    severity_order = {'info': 0, 'warning': 1, 'critical': 2}
                    if severity_order.get(sev, 0) > severity_order.get(current_incident["max_severity"], 0):
                        current_incident["max_severity"] = sev
                else:
                    incidents.append(current_incident)
                    current_incident = {
                        "start_ts": et, "end_ts": et,
                        "events": [ev],
                        "services": {ev.get('service', ev.get('root_cause', '?')), ev.get('service', '?')},
                        "types": {ev.get('type', '?')},
                        "max_severity": ev.get('severity', 'info'),
                    }
            except:
                incidents.append(current_incident)
                current_incident = None
    
    if current_incident:
        incidents.append(current_incident)

# ─── 4. HEALTH STATE MACHINE ───

def compute_health_state(trends_5min, trend_15min, recent_events_5min):
    # Check for critical signals
    critical_types = {e.get('type') for e in recent_events_5min}
    
    has_cascade = 'dependency_cascade' in critical_types
    has_critical = any(e.get('severity') == 'critical' for e in recent_events_5min)
    
    # Check for restart activity
    total_restarts_5min = sum(trends_5min.get(s, {}).get('restarts_total', 0) for s in CORE_SERVICES)
    
    # Check latency drift
    high_drift = any(
        trends_5min.get(s, {}).get('latency_drift_ratio', 1) > 3.0
        for s in CORE_SERVICES
    )
    
    # State transitions
    if has_cascade or has_critical:
        return "RED"
    elif total_restarts_5min > 0 or high_drift:
        return "YELLOW"
    else:
        return "GREEN"

w5_trends = {svc: {} for svc in SERVICES}
for svc in SERVICES:
    lats = [p["v"] for p in drift[svc]["latency_ms"] if p["t"] >= (now - datetime.timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%SZ')]
    nr_vals = [p["v"] for p in drift[svc]["nrestarts"] if p["t"] >= (now - datetime.timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%SZ')]
    if lats:
        mn, mx = min(lats), max(lats)
        w5_trends[svc]["latency_drift_ratio"] = round(mx / mn, 2) if mn > 0 else 1
    if nr_vals:
        w5_trends[svc]["restarts_total"] = nr_vals[-1] - nr_vals[0] if len(nr_vals) >= 2 else 0

health = compute_health_state(w5_trends, {}, w5_ev)

# ─── 5. OUTPUT ───

report = {
    "ts": now.strftime('%Y-%m-%dT%H:%M:%SZ'),
    "health": health,
    "window_5min": {
        "observations": len(w5_obs),
        "events": len(w5_ev)
    },
    "window_15min": {
        "observations": len(w15_obs),
        "events": len(w15_ev)
    },
    "incidents": [
        {
            "start": inc["start_ts"],
            "end": inc["end_ts"],
            "severity": inc["max_severity"],
            "types": sorted(inc["types"]),
            "services": sorted(inc["services"]),
            "event_count": len(inc["events"])
        }
        for inc in incidents
    ],
    "drift_trends": {
        svc: t for svc, t in drift_trends.items()
        if t
    },
    "system_wide": {
        "total_observations": len(observations),
        "total_events": len([e for e in events if e.get('type') != 'no_event']),
        "uptime_minutes": round(len(observations)),  # ~1 per minute
    }
}

with open(os.path.join(RPT_DIR, f"{today}.json"), 'w') as f:
    json.dump(report, f, indent=2)

print(json.dumps(report, indent=2))
PYEOF
