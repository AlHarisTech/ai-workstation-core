#!/bin/sh
# EMCL v1.0 — Execution Mode Controller Layer
# Sits between Level 2 Agent and Execution Bridge.
# Governs WHEN the system transitions from simulation to real execution.
#
# Modes:
#   OBSERVE_ONLY           — simulate only, no execution (current)
#   CONTROLLED_AUTOMATION  — auto-execute under bounded conditions
#   FULL_AUTONOMY          — policy-driven self-activation (future)
#
# Usage:
#   ./emctl-mcp.sh                          # evaluate current mode + decisions
#   ./emctl-mcp.sh --status                 # show current mode and counters
#   ./emctl-mcp.sh --set-mode controlled    # transition mode (requires override)
#   ./emctl-mcp.sh --stress cascade         # evaluate under stress scenario

set -e

EMCTL_DIR="/home/asem/workspace/.opencode/emctl"
AGENT_DIR="/home/asem/workspace/.opencode/agent"
BRIDGE_DIR="/home/asem/workspace/.opencode/bridge"
mkdir -p "$EMCTL_DIR"

# ─── Parse ───
SET_MODE=""
STRESS_SCENARIO=""
STATUS_ONLY=""

while [ $# -gt 0 ]; do
    case "$1" in
        --set-mode)    SET_MODE="$2"; shift 2 ;;
        --stress)      STRESS_SCENARIO="$2"; shift 2 ;;
        --status)      STATUS_ONLY="1"; shift ;;
        *) echo "Usage: $0 [--set-mode <mode>] [--stress <scenario>] [--status]"; exit 1 ;;
    esac
done

SET_MODE="$SET_MODE" STRESS_SCENARIO="$STRESS_SCENARIO" STATUS_ONLY="$STATUS_ONLY" python3 << 'PYEOF'
import json, os, sys, datetime

EMCTL_DIR   = "/home/asem/workspace/.opencode/emctl"
AGENT_DIR   = "/home/asem/workspace/.opencode/agent"
BRIDGE_DIR  = "/home/asem/workspace/.opencode/bridge"

SET_MODE       = os.environ.get('SET_MODE', '')
STRESS_SCENARIO = os.environ.get('STRESS_SCENARIO', '')
STATUS_ONLY    = os.environ.get('STATUS_ONLY', '')

# If stress scenario provided, run agent first to generate fresh report
if STRESS_SCENARIO:
    import subprocess
    agent_proc = subprocess.run(
        ["/home/asem/workspace/.opencode/agent-mcp.sh", "--stress", STRESS_SCENARIO],
        capture_output=True, text=True, timeout=30
    )

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

# ─── 1. Load persistent mode state ───
mode_path = os.path.join(EMCTL_DIR, "mode.json")
if not os.path.exists(mode_path):
    mode_state = {
        "mode": "OBSERVE_ONLY",
        "previous_mode": None,
        "transitions": [],
        "last_changed": now_ts,
        "activation_count": 0,
        "blocked_count": 0,
        "bypass_count": 0,
        "total_decisions_evaluated": 0,
    }
    save_json(mode_path, mode_state)
else:
    mode_state = load_json(mode_path)

# If --status only, print and exit
if STATUS_ONLY:
    mode_state["ts"] = now_ts
    print(json.dumps(mode_state, indent=2))
    sys.exit(0)

# ─── 2. Handle mode transition (--set-mode) ───
VALID_MODES = ["OBSERVE_ONLY", "CONTROLLED_AUTOMATION", "FULL_AUTONOMY"]

if SET_MODE:
    if SET_MODE.upper() not in VALID_MODES:
        result = {"ts": now_ts, "error": f"invalid mode '{SET_MODE}'. Valid: {VALID_MODES}", "mode": mode_state["mode"]}
        print(json.dumps(result, indent=2))
        sys.exit(0)

    new_mode = SET_MODE.upper()
    if new_mode == mode_state["mode"]:
        result = {"ts": now_ts, "info": f"already in {new_mode}", "mode": mode_state["mode"]}
        print(json.dumps(result, indent=2))
        sys.exit(0)

    # Record transition
    transition = {
        "ts": now_ts,
        "from": mode_state["mode"],
        "to": new_mode,
        "reason": "manual_override",
    }
    mode_state["previous_mode"] = mode_state["mode"]
    mode_state["mode"] = new_mode
    mode_state["last_changed"] = now_ts
    mode_state["transitions"].append(transition)
    save_json(mode_path, mode_state)

    result = {"ts": now_ts, "mode": new_mode, "transition": transition}
    print(json.dumps(result, indent=2))
    sys.exit(0)

# ─── 3. Normal evaluation cycle ───
# Load latest agent report
agent_report = load_json(os.path.join(AGENT_DIR, f"{today}.json"))
if not agent_report:
    result = {"ts": now_ts, "mode": mode_state["mode"], "error": "no agent report", "decision": "NOOP_NO_SIGNAL"}
    print(json.dumps(result, indent=2))
    sys.exit(0)

# ─── 4. Extract telemetry from agent ───
drift_state   = agent_report.get("system_drift", agent_report.get("drift", "STABLE"))
agent_state   = agent_report.get("agent_state", "OBSERVE_ONLY")
gate_results  = agent_report.get("gate_results", [])
candidates    = agent_report.get("hard_action_candidates", [])
simulations   = agent_report.get("simulations", [])
soft_actions  = agent_report.get("soft_action_count", agent_report.get("soft_actions", 0))

all_gates_pass = all(g.get("passed", False) for g in gate_results) if gate_results else False
gate_summary = {g["gate"]: "PASS" if g["passed"] else "BLOCK" for g in gate_results}

# Count per-state
state_vector = agent_report.get("state_vector", {}).get("services", {})
critical_count = sum(1 for s in state_vector.values() if s.get("drift_state") == "CRITICAL")
drift_count    = sum(1 for s in state_vector.values() if s.get("drift_state") == "DRIFT")

# ─── 5. ENFORCE MODE-SPECIFIC ACTIVATION RULES ───
current_mode = mode_state["mode"]
mode_state["total_decisions_evaluated"] += 1

activation_reasons = []
block_reasons = []

# 5a. Threshold check
has_drift = drift_state != "STABLE"
if not has_drift:
    block_reasons.append("system_drift_is_STABLE — no action needed")

# 5b. Gate check
if not all_gates_pass:
    failing = [name for name, status in gate_summary.items() if status == "BLOCK"]
    block_reasons.append(f"gates blocking: {', '.join(failing)}")

# 5c. Candidate availability
if not candidates:
    block_reasons.append("no hard action candidates")

# ─── 6. DECISION MATRIX ───
decision = "NOOP"
action = None
to_execute = None

if current_mode == "OBSERVE_ONLY":
    # Simulate only. Never execute.
    if has_drift and not block_reasons:
        decision = "WOULD_EXECUTE"
        activation_reasons.append("OBSERVE_ONLY mode blocks execution — would execute if mode were CONTROLLED_AUTOMATION")
    else:
        decision = "NOOP"
    if block_reasons:
        decision = "NOOP"

elif current_mode == "CONTROLLED_AUTOMATION":
    # Execute under bounded conditions
    if has_drift and all_gates_pass and candidates:
        # Pick safest candidate (lowest blast radius)
        safe = [c for c in candidates if c.get("blast_radius", "high") in ("low", "medium")]
        target = safe[0] if safe else candidates[0]
        action = {
            "action_type": target.get("action_type"),
            "service": target.get("service"),
            "gate_result": target.get("gate_result"),
            "blast_radius": target.get("blast_radius"),
        }
        decision = "EXECUTE"
        activation_reasons.append(f"drift={drift_state} gates=PASS → executing {action['action_type']} on {action['service']}")
    else:
        if not has_drift:
            block_reasons.append("CONTROLLED_AUTOMATION: no drift detected")
        if not all_gates_pass:
            block_reasons.append("CONTROLLED_AUTOMATION: gates not all PASS")
        if not candidates:
            block_reasons.append("CONTROLLED_AUTOMATION: no action candidates")
        decision = "BLOCKED"
        mode_state["blocked_count"] += 1

elif current_mode == "FULL_AUTONOMY":
    # Future — policy-driven self-activation
    if has_drift and all_gates_pass and candidates:
        target = candidates[0]
        action = {
            "action_type": target.get("action_type"),
            "service": target.get("service"),
            "gate_result": target.get("gate_result"),
        }
        decision = "EXECUTE"
    else:
        decision = "BLOCKED"
        mode_state["blocked_count"] += 1

# ─── 7. Execute if decision is EXECUTE ───
bridge_result = None
if decision == "EXECUTE" and action:
    mode_state["activation_count"] += 1
    # Call execution bridge with this candidate
    import subprocess
    try:
        # Find candidate index
        target_idx = next(
            (i for i, c in enumerate(candidates)
             if c.get("action_type") == action["action_type"]
             and c.get("service") == action["service"]),
            0
        )
        bridge_cmd = ["/home/asem/workspace/.opencode/execute-mcp.sh",
                      "--action-id", str(target_idx), "--commit"]
        if STRESS_SCENARIO:
            bridge_cmd.extend(["--stress", STRESS_SCENARIO])

        proc = subprocess.run(bridge_cmd, capture_output=True, text=True, timeout=35)
        if proc.stdout:
            try:
                bridge_result = json.loads(proc.stdout)
            except json.JSONDecodeError:
                bridge_result = {"raw_output": proc.stdout}
        bridge_result = bridge_result or {}
        bridge_result["_bridge_rc"] = proc.returncode
    except Exception as e:
        bridge_result = {"error": str(e), "_bridge_rc": -1}
elif decision == "WOULD_EXECUTE":
    pass

# ─── 8. Save EMCL state ───
save_json(mode_path, mode_state)

# ─── 9. Build report ───
report = {
    "ts": now_ts,
    "mode": current_mode,
    "decision": decision,
    "drift": drift_state,
    "agent_state": agent_state,
    "gates": gate_summary,
    "all_gates_pass": all_gates_pass,
    "service_states": {s: v.get("drift_state") for s, v in state_vector.items()},
    "critical_count": critical_count,
    "drift_count": drift_count,
    "action": action,
    "activation_reasons": activation_reasons,
    "block_reasons": block_reasons,
    "bridge_result": bridge_result,
    "mode_state_summary": {
        "mode": mode_state["mode"],
        "previous_mode": mode_state.get("previous_mode"),
        "last_changed": mode_state.get("last_changed"),
        "transitions_count": len(mode_state.get("transitions", [])),
        "activation_count": mode_state.get("activation_count", 0),
        "blocked_count": mode_state.get("blocked_count", 0),
        "total_decisions": mode_state.get("total_decisions_evaluated", 0),
    },
    "candidate_count": len(candidates),
    "simulation_count": len(simulations),
    "soft_action_count": soft_actions,
}

# Save daily report
report_path = os.path.join(EMCTL_DIR, f"{today}.json")
save_json(report_path, report)

print(json.dumps(report, indent=2))
PYEOF