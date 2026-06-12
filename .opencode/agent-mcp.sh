#!/bin/sh
# Level 2 Agent v0.4 — Unified Control Runtime
# Deterministic Control Loop over F3/F4 Runtime Intelligence Signals
# Supports --stress <scenario> for controlled stress testing

set -e

STRESS_MODE=""
STRESS_SCENARIO=""

while [ $# -gt 0 ]; do
    case "$1" in
        --stress)
            STRESS_MODE="1"
            STRESS_SCENARIO="$2"
            shift 2
            ;;
        *)
            echo "Usage: $0 [--stress <scenario>]"
            exit 1
            ;;
    esac
done

AGENT_DIR="/home/asem/workspace/.opencode/agent"
STRESS_DIR="/home/asem/workspace/.opencode/stress"
mkdir -p "$AGENT_DIR"

STRESS_MODE="$STRESS_MODE" STRESS_SCENARIO="$STRESS_SCENARIO" python3 << 'PYEOF'
import json, os, datetime, sys
from collections import defaultdict

STRESS_MODE = os.environ.get('STRESS_MODE', '')
STRESS_SCENARIO = os.environ.get('STRESS_SCENARIO', '')

OBS_DIR = "/home/asem/workspace/.opencode/observations"
CLS_DIR = "/home/asem/workspace/.opencode/classifications"
RPT_DIR = "/home/asem/workspace/.opencode/reports"
DRIFT_DIR = "/home/asem/workspace/.opencode/drift"
AGENT_DIR = "/home/asem/workspace/.opencode/agent"
STRESS_DIR = "/home/asem/workspace/.opencode/stress"

today = datetime.datetime.utcnow().strftime('%Y-%m-%d')
now_ts = datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')

# ─── 1. INGEST SIGNALS (F3 + F4) ───
def load_jsonl(path):
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
                print(f"L2 WARN: skipping malformed record — {e}", file=sys.stderr)
    return results

def load_json(path):
    if not os.path.exists(path):
        return {}
    with open(path) as f:
        try:
            return json.load(f)
        except json.JSONDecodeError as e:
            print(f"L2 WARN: skipping malformed JSON file {path} — {e}", file=sys.stderr)
            return {}

observations = load_jsonl(os.path.join(OBS_DIR, f"{today}.jsonl"))
events = load_jsonl(os.path.join(CLS_DIR, f"{today}.jsonl"))
reports = load_json(os.path.join(RPT_DIR, f"{today}.json"))

# Load drift: real F4 or stress scenario
if STRESS_MODE:
    stress_path = os.path.join(STRESS_DIR, f"{STRESS_SCENARIO}.json")
    drift = load_json(stress_path)
    if not drift:
        print(json.dumps({"error": f"stress scenario '{STRESS_SCENARIO}' not found", "ts": now_ts}))
        sys.exit(0)
    drift["_stress_mode"] = True
    drift["_scenario"] = STRESS_SCENARIO
else:
    drift = load_json(os.path.join(DRIFT_DIR, f"{today}.json"))

SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory','chromadb-mcp','github-mcp']
CORE_SERVICES = ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory']

# ─── 2. NORMALIZE STATE VECTOR ───
state_vector = {"ts": now_ts, "services": {}}

for svc in SERVICES:
    last_obs = observations[-1].get(svc, {}) if observations else {}
    svc_drift = drift.get("per_service", {}).get(svc, {})
    
    status = last_obs.get("state", "unknown")
    latency_ms = last_obs.get("latency")
    if isinstance(latency_ms, (int,float)):
        latency_ms = round(latency_ms * 1000, 2)
    
    state_vector["services"][svc] = {
        "status": status,
        "latency_ms": latency_ms,
        "drift_state": svc_drift.get("state", "unknown"),
        "drift_ratio": svc_drift.get("latency_ratio_vs_rolling", 1.0),
        "stability_pct": svc_drift.get("stability_pct_5m", 100),
        "is_core": svc in CORE_SERVICES,
    }

# System-wide metrics
system_drift_state = drift.get("system_drift_state", "unknown")
recent_events = [e for e in events if e.get('type') != 'no_event']

# ─── 3. POLICY ENGINE EVALUATION ───
class PolicyEvaluator:
    def __init__(self, state, drift, events):
        self.state = state
        self.drift = drift
        self.events = events
        self.decisions = []

    def log_decision(self, policy, condition, verdict, reason):
        self.decisions.append({
            "ts": now_ts,
            "policy": policy,
            "condition": condition,
            "verdict": verdict,
            "reason": reason
        })

    def evaluate(self):
        # P0 — Evidence First
        has_f3 = len(self.state.get("services", {})) > 0
        has_f4 = self.drift.get("system_drift_state") is not None
        evidence_ok = has_f3 and has_f4
        self.log_decision(
            "P0 — Evidence First",
            "F3 state + F4 drift available",
            "PASS" if evidence_ok else "FAIL",
            f"F3={'ok' if has_f3 else 'missing'} F4={'ok' if has_f4 else 'missing'}"
        )
        has_evidence = evidence_ok

        # P3 — Drift Sensitivity Gate
        system_stable = (self.drift.get("system_drift_state") == "STABLE")
        self.log_decision(
            "P3 — Drift Sensitivity Gate",
            "system_drift_state == STABLE",
            "PASS" if system_stable else "WARN",
            f"drift_state={self.drift.get('system_drift_state')}"
        )

        # P4 — Cascade Protection
        cascade_events = [e for e in self.events if e.get('type') == 'dependency_cascade']
        cascade_risk = len(cascade_events) > 0
        self.log_decision(
            "P4 — Cascade Protection",
            "recent cascade events detected",
            "ACTIVE" if cascade_risk else "INACTIVE",
            f"{len(cascade_events)} cascade events in recent history"
        )

        # Check individual service drift
        for svc_name, svc_state in self.state["services"].items():
            if svc_state["drift_state"] in ("DRIFT", "CRITICAL"):
                self.log_decision(
                    f"Service Drift — {svc_name}",
                    f"drift_state == {svc_state['drift_state']}",
                    "WARN",
                    f"ratio={svc_state['drift_ratio']} stability={svc_state['stability_pct']}%"
                )

        return self.decisions

pe = PolicyEvaluator(state_vector, drift, recent_events)
decisions = pe.evaluate()

# ─── 4. DECISION GATES ───
class DecisionGates:
    def __init__(self, decisions, state):
        self.decisions = decisions
        self.state = state
        self.gate_results = []

    def check_gate(self, gate_name, condition, passed):
        self.gate_results.append({
            "ts": now_ts,
            "gate": gate_name,
            "condition": condition,
            "passed": passed
        })

    def evaluate(self):
        svcs = self.state.get("services", {})
        
        # Gate 1: Evidence Gate
        evidence_passed = all(
            d["verdict"] in ("PASS", "INACTIVE", "WARN")
            for d in self.decisions if d["policy"] == "P0 — Evidence First"
        )
        self.check_gate("Gate 1 — Evidence", "≥2 independent signals", evidence_passed)

        # Gate 4: Stability Gate
        stable_count = sum(1 for s in svcs.values() if s["drift_state"] in ("STABLE", "PRE_DRIFT"))
        all_stable = stable_count == len(svcs)
        self.check_gate("Gate 4 — Stability", "all services stable/drift=0", all_stable)

        # Gate 5: Cascade Risk Gate
        cascade_active = any(
            d["verdict"] == "ACTIVE" and "Cascade" in d.get("policy", "")
            for d in self.decisions
        )
        self.check_gate("Gate 5 — Cascade", "dependency propagation detected", cascade_active)

        return self.gate_results

dg = DecisionGates(decisions, state_vector)
gate_results = dg.evaluate()

# ─── 5. DECISION SUMMARY ───
# Gate 4: stability_gate_passed = True when all services stable (blocks hard actions)
stability_gate_passed = any((g.get("gate", "").startswith("Gate 4")) and g.get("passed") for g in gate_results)
# Gate 5: cascade_gate_active = True when cascade IS detected (protection engaged)
cascade_gate_active = any((g.get("gate", "").startswith("Gate 5")) and g.get("passed") for g in gate_results)
# Evidence gate
evidence_passed = any((g.get("gate", "").startswith("Gate 1")) and g.get("passed") for g in gate_results)

# State machine (spec §4 — P3)
if cascade_gate_active:
    agent_state = "BLOCKED_HARD"
elif not evidence_passed:
    agent_state = "BLOCKED_HARD"
elif system_drift_state == "STABLE" and stability_gate_passed:
    agent_state = "OBSERVE_ONLY"
elif stability_gate_passed:
    agent_state = "SOFT_ONLY"
else:
    agent_state = "FULL"

# ─── 6. SOFT ACTION SELECTOR (v0.2) ───
class SoftActionSelector:
    def __init__(self, state, drift, events, agent_state):
        self.state = state
        self.drift = drift
        self.events = events
        self.agent_state = agent_state
        self.actions = []

    def emit(self, action_type, service, reason, confidence, context):
        self.actions.append({
            "ts": now_ts,
            "action_type": action_type,
            "service": service,
            "confidence": confidence,
            "reason": reason,
            "context": context
        })

    def evaluate(self):
        svcs = self.state.get("services", {})
        drift_state = self.drift.get("system_drift_state", "STABLE")

        # Stability gate: when drift=0, only FLAG allowed (anti-noise)
        only_flag = (drift_state == "STABLE")

        for svc_name, svc_data in svcs.items():
            dstate = svc_data.get("drift_state", "STABLE")
            dratio = svc_data.get("drift_ratio", 1.0)
            lat = svc_data.get("latency_ms", 0)
            stable = svc_data.get("stability_pct", 100)

            # FLAG: early drift signature or ambiguous pattern
            if dstate == "PRE_DRIFT":
                self.emit("FLAG", svc_name, "early_drift_signature", 0.4,
                    {"drift_ratio": dratio, "latency_ms": lat})

            # FLAG: latency persistently above baseline (ratio > 1.3 without reaching DRIFT)
            if "STABLE" in dstate and dratio > 1.3:
                self.emit("FLAG", svc_name, "elevated_latency_ratio", 0.3,
                    {"drift_ratio": dratio, "latency_ms": lat})

            # ALERT: drift detected (only if not in stability gate)
            if dstate == "DRIFT" and not only_flag:
                self.emit("ALERT", svc_name, "drift_state_detected", 0.6,
                    {"drift_ratio": dratio, "stability_pct": stable})

            # ALERT: critical drift
            if dstate == "CRITICAL" and not only_flag:
                self.emit("ALERT", svc_name, "critical_drift", 0.9,
                    {"drift_ratio": dratio, "stability_pct": stable})

            # ESCALATION: sustained drift with multi-service correlation
            if dstate == "DRIFT" and not only_flag:
                # Check if other services also in drift
                peers_in_drift = sum(1 for n, d in svcs.items()
                    if n != svc_name and d.get("drift_state") in ("DRIFT", "CRITICAL"))
                if peers_in_drift >= 1:
                    self.emit("ESCALATION", svc_name, "multi_service_drift", 0.7,
                        {"drift_ratio": dratio, "peers_affected": peers_in_drift})

            # ESCALATION: cascade events correlated
            cascade_count = sum(1 for e in self.events
                if e.get('type') == 'dependency_cascade')
            if cascade_count > 0 and svc_name in [s for s in ['mcp-filesystem','mcp-git','mcp-fetch','mcp-memory']]:
                self.emit("ESCALATION", svc_name, "cascade_correlation", 0.8,
                    {"cascade_events": cascade_count})

        # Suppress if no meaningful signals
        if not self.actions:
            self.emit("NOOP", "system", "no_soft_action_triggered", 0.0, {})

        return self.actions

sas = SoftActionSelector(state_vector, drift, recent_events, agent_state)
soft_actions = sas.evaluate()

# ─── 7. PRE-FLIGHT SIMULATION ENGINE (v0.4 embedded) ───
# Simulates hard actions without execution. Mandatory pre-flight gate.

DEPENDENCY_CHAIN = ['mcp-filesystem', 'mcp-git', 'mcp-fetch', 'mcp-memory']

class SimulationEngine:
    def __init__(self, state, drift):
        self.state = state
        self.drift = drift
        self.simulations = []

    def simulate_restart(self, service_name):
        """Simulate restart of a service and compute cascading effects."""
        svcs = self.state.get("services", {})
        is_core = svcs.get(service_name, {}).get("is_core", False)

        if service_name not in svcs:
            return None

        cascade_affected = []
        if is_core:
            # Simulate BindsTo cascade
            idx = DEPENDENCY_CHAIN.index(service_name)
            for dep in DEPENDENCY_CHAIN[idx + 1:]:
                cascade_affected.append(dep)

        recovery_estimate_s = 2 + (len(cascade_affected) * 2)  # 2s per service
        cascade_risk = len(cascade_affected) / max(len(DEPENDENCY_CHAIN) - 1, 1)

        return {
            "action_type": "SIMULATED_RESTART",
            "service": service_name,
            "recovery_time_estimate_s": recovery_estimate_s,
            "cascade_affected": cascade_affected,
            "cascade_risk": round(cascade_risk, 2),
            "blast_radius": "high" if cascade_risk > 0.5 else "medium" if cascade_risk > 0 else "low",
            "reversibility_score": 1.0,  # restart is fully reversible
            "stability_delta": "negative" if cascade_risk > 0 else "neutral",
        }

    def simulate_isolation(self, service_name):
        """Simulate removing a service from the dependency graph."""
        svcs = self.state.get("services", {})
        is_core = svcs.get(service_name, {}).get("is_core", False)

        downstream_loss = []
        if is_core:
            idx = DEPENDENCY_CHAIN.index(service_name)
            for dep in DEPENDENCY_CHAIN[idx + 1:]:
                downstream_loss.append(dep)

        fragmentation = len(downstream_loss) / len(DEPENDENCY_CHAIN) if is_core else 0.1

        return {
            "action_type": "SIMULATED_ISOLATION",
            "service": service_name,
            "downstream_services_lost": downstream_loss,
            "fragmentation_score": round(fragmentation, 2),
            "blast_radius": "high" if fragmentation > 0.5 else "medium" if fragmentation > 0 else "low",
            "reversibility_score": 0.7,  # isolation requires manual rejoin
            "stability_delta": "negative",
        }

    def simulate_rollback(self):
        """Simulate rolling back to previous observation state."""
        prev_state = {}
        for svc, data in self.state.get("services", {}).items():
            prev_state[svc] = {
                "latency_ms": data.get("latency_ms", 0),
                "drift_state": data.get("drift_state", "STABLE"),
            }

        return {
            "action_type": "SIMULATED_ROLLBACK",
            "service": "system",
            "recovery_time_estimate_s": 5,
            "blast_radius": "medium",
            "reversibility_score": 0.9,
            "stability_delta": "positive",
            "data_loss_risk": 0.1,
        }

    def evaluate(self):
        svcs = self.state.get("services", {})
        drift_state = self.drift.get("system_drift_state", "STABLE")

        for svc_name, svc_data in svcs.items():
            dstate = svc_data.get("drift_state", "STABLE")
            dratio = svc_data.get("drift_ratio", 1.0)

            # Only simulate when drift indicates instability
            if dstate in ("DRIFT", "CRITICAL") or drift_state in ("DRIFT", "CRITICAL"):
                sim = self.simulate_restart(svc_name)
                if sim:
                    self.simulations.append(sim)

                sim_iso = self.simulate_isolation(svc_name)
                if sim_iso:
                    self.simulations.append(sim_iso)

        return self.simulations

sim_engine = SimulationEngine(state_vector, drift)
simulations = sim_engine.evaluate()

# ─── 8. CONTROLLED EXECUTION ENGINE (v0.4) ───
# Hard actions are ONLY allowed through:
#   pre-flight simulation → gate check → execution → verification → commit/rollback

hard_action_candidates = []
for sim in simulations:
    # Gate 3: Cascade Sensitivity Gate
    if sim.get("cascade_risk", 0) > 0.7:
        sim["gate_result"] = "BLOCKED_HIGH_CASCADE_RISK"
    # Gate 4: Stability Delta Gate
    elif sim.get("stability_delta") == "negative" and sim.get("blast_radius") == "high":
        sim["gate_result"] = "BLOCKED_UNSTABLE_IMPACT"
    else:
        sim["gate_result"] = "PASSED_SIMULATION"
        hard_action_candidates.append(sim)

# Rollback engine: prepared but not engaged (no real execution yet)
rollback_readiness = {
    "state_before": {svc: data for svc, data in state_vector.get("services", {}).items()},
    "rollback_available": True,
    "estimated_recovery_s": 5,
}

# ─── 9. OUTPUT ───
agent_report = {
    "ts": now_ts,
    "agent_version": "v0.4-simulation-verified-execution",
    "agent_state": agent_state,
    "system_drift": system_drift_state,
    "state_vector": state_vector,
    "policy_decisions": decisions,
    "gate_results": gate_results,
    "soft_actions": soft_actions,
    "stress_mode": STRESS_MODE == "1",
    "stress_scenario": STRESS_SCENARIO if STRESS_MODE else None,
    "simulations": simulations,
    "hard_action_candidates": hard_action_candidates,
    "rollback_readiness": rollback_readiness,
    "summary": {
        "policies_evaluated": len(decisions),
        "gates_checked": len(gate_results),
        "soft_actions_emitted": len([a for a in soft_actions if a["action_type"] != "NOOP"]),
        "simulations_run": len(simulations),
        "hard_action_candidates": len(hard_action_candidates),
        "hard_actions_allowed": agent_state not in ("BLOCKED_HARD", "OBSERVE_ONLY"),
        "recommended_mode": agent_state,
        "rollback_ready": rollback_readiness["rollback_available"],
    }
}

outfile = os.path.join(AGENT_DIR, f"{today}.json")
with open(outfile, 'w') as f:
    json.dump(agent_report, f, indent=2)

# Append soft actions to action log
actions_log = os.path.join(AGENT_DIR, f"actions-{today}.jsonl")
with open(actions_log, 'a') as f:
    for a in soft_actions:
        if a["action_type"] != "NOOP":
            f.write(json.dumps(a) + '\n')

# Append simulations to simulation log
sim_log = os.path.join(AGENT_DIR, f"simulations-{today}.jsonl")
with open(sim_log, 'a') as f:
    for s in simulations:
        f.write(json.dumps(s) + '\n')

# Print summary only (full report in file)
print(json.dumps({
    "ts": now_ts,
    "agent_version": "v0.4-simulation-verified-execution",
    "agent_state": agent_state,
    "drift": system_drift_state,
    "policies": len(decisions),
    "gates": len(gate_results),
    "soft_actions": len([a for a in soft_actions if a["action_type"] != "NOOP"]),
    "simulations": len(simulations),
    "hard_candidates": len(hard_action_candidates),
    "rollback": rollback_readiness["rollback_available"],
    "mode": agent_state,
}, indent=2))
PYEOF
