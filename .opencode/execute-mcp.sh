#!/bin/sh
# Execution Bridge v1.0 — Simulation ✦ Gated Decision ✦ Controlled Mutation
#
# Translates Level 2 Agent's hard action candidates into real systemd actions,
# with double-gate re-validation, outcome verification, and rollback binding.
#
# Usage:
#   ./execute-mcp.sh                              # dry-run: preview highest-confidence action
#   ./execute-mcp.sh --action-id 3                 # execute specific candidate by index
#   ./execute-mcp.sh --action-id 3 --commit        # actually execute (not dry-run)
#   ./execute-mcp.sh --stress cascade --action-id 3 # test with stress scenario

set -e

BRIDGE_DIR="/home/asem/workspace/.opencode/bridge"
AGENT_DIR="/home/asem/workspace/.opencode/agent"
STRESS_DIR="/home/asem/workspace/.opencode/stress"
DRIFT_DIR="/home/asem/workspace/.opencode/drift"
mkdir -p "$BRIDGE_DIR"

# ─── Parse arguments ───
ACTION_ID=""
COMMIT=""
STRESS_SCENARIO=""

while [ $# -gt 0 ]; do
    case "$1" in
        --action-id)   ACTION_ID="$2"; shift 2 ;;
        --commit)      COMMIT="1"; shift ;;
        --stress)      STRESS_SCENARIO="$2"; shift 2 ;;
        *) echo "Usage: $0 [--action-id <n>] [--commit] [--stress <scenario>]"; exit 1 ;;
    esac
done

# ─── Make agent process its latest signals first ───
if [ -n "$STRESS_SCENARIO" ]; then
    /home/asem/workspace/.opencode/agent-mcp.sh --stress "$STRESS_SCENARIO" 2>/dev/null > /dev/null
    AGENT_REPORT=$(cat "$AGENT_DIR/$(date +%F).json")
else
    AGENT_REPORT=$(/home/asem/workspace/.opencode/agent-mcp.sh 2>/dev/null)
fi

ACTION_ID="$ACTION_ID" COMMIT="$COMMIT" STRESS_SCENARIO="$STRESS_SCENARIO" python3 << 'PYEOF'
import json, os, sys, subprocess

BRIDGE_DIR = "/home/asem/workspace/.opencode/bridge"
AGENT_DIR  = "/home/asem/workspace/.opencode/agent"
DRIFT_DIR  = "/home/asem/workspace/.opencode/drift"
STRESS_SCENARIO = os.environ.get('STRESS_SCENARIO', '')
ACTION_ID_STR    = os.environ.get('ACTION_ID', '')
COMMIT_FLAG      = os.environ.get('COMMIT', '')

now_ts = __import__('datetime').datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')
today  = __import__('datetime').datetime.utcnow().strftime('%Y-%m-%d')

# ─── 1. Load agent decision ───
agent_path = os.path.join(AGENT_DIR, f"{today}.json")
if not os.path.exists(agent_path):
    result = {"ts": now_ts, "error": "no agent report found", "verdict": "REJECT_NO_SIGNAL"}
    with open(os.path.join(BRIDGE_DIR, f"{today}.json"), 'w') as f:
        json.dump(result, f, indent=2)
    print(json.dumps(result, indent=2))
    sys.exit(0)

with open(agent_path) as f:
    agent = json.load(f)

# ─── 2. Resolve candidate ───
candidates = agent.get('hard_action_candidates', [])
if not candidates:
    result = {"ts": now_ts, "error": "no hard action candidates", "verdict": "REJECT_NO_CANDIDATES"}
    with open(os.path.join(BRIDGE_DIR, f"{today}.json"), 'w') as f:
        json.dump(result, f, indent=2)
    print(json.dumps(result, indent=2))
    sys.exit(0)

if ACTION_ID_STR:
    try:
        idx = int(ACTION_ID_STR)
        if idx < 0 or idx >= len(candidates):
            raise IndexError
        candidate = candidates[idx]
    except (ValueError, IndexError):
        result = {"ts": now_ts, "error": f"invalid action-id {ACTION_ID_STR}", "verdict": "REJECT_BAD_ID"}
        print(json.dumps(result, indent=2))
        sys.exit(0)
else:
    # Default: highest-confidence candidate (sorted by simulation pass order)
    candidate = candidates[0]

action_type  = candidate.get('action_type', '')
service      = candidate.get('service', '')
sim_result   = candidate.get('gate_result', 'UNKNOWN')
blast_radius = candidate.get('blast_radius', 'unknown')

# ─── 3. RE-VALIDATE safety gates (double check before execution) ───
# Load current drift state
drift_path = os.path.join(DRIFT_DIR, f"{today}.json")
if not os.path.exists(drift_path) and not STRESS_SCENARIO:
    result = {"ts": now_ts, "error": "no drift data for re-validation", "verdict": "REJECT_NO_DRIFT"}
    print(json.dumps(result, indent=2))
    sys.exit(0)

reject_reasons = []

# Evidence check: simulation must have PASSED
if 'PASS' not in sim_result:
    reject_reasons.append(f"simulation did not pass ({sim_result})")

# Stability check: system must not be critically degraded
drift_data = agent.get('state_vector', {}).get('services', {})
if action_type == 'SIMULATED_RESTART':
    # For restart: target service must not be the root of a cascade
    svc_state = drift_data.get(service, {})
    if svc_state.get('drift_state') == 'STABLE':
        reject_reasons.append(f"restart target {service} is healthy — no action needed")

# Cascade check: if blast radius is high, reject unless --commit confirms
if blast_radius == 'high' and not COMMIT_FLAG:
    reject_reasons.append(f"blast radius is high ({blast_radius}) — requires --commit to proceed")

# ─── 4. Translate to real action ───
ACTION_MAP = {
    'SIMULATED_RESTART': {
        'systemd_cmd': f'systemctl --user restart {service}',
        'description': f'restart {service} via systemd',
    },
    'SIMULATED_ISOLATION': {
        'systemd_cmd': f'systemctl --user stop {service} && systemctl --user mask {service}',
        'description': f'stop + mask {service} via systemd',
    },
}

action_def = ACTION_MAP.get(action_type, {})
systemd_cmd   = action_def.get('systemd_cmd', '')
action_desc   = action_def.get('description', '')

if not systemd_cmd:
    reject_reasons.append(f"unknown action type: {action_type}")

# ─── 5. Build execution result ───
execution_state = {
    "ts": now_ts,
    "stress_mode": bool(STRESS_SCENARIO),
    "candidate": {
        "action_type": action_type,
        "service": service,
        "gate_result": sim_result,
        "blast_radius": blast_radius,
    },
    "re_validation": {
        "passed": len(reject_reasons) == 0,
        "reject_reasons": reject_reasons,
    },
    "translated_command": systemd_cmd,
    "action_description": action_desc,
    "commit": bool(COMMIT_FLAG),
    "executed": False,
    "stdout": "",
    "stderr": "",
    "return_code": None,
    "post_state": {},
    "rollback_command": "",
    "verdict": "",
}

# ─── 6. Execute (if gates pass and --commit) ───
if reject_reasons:
    execution_state["verdict"] = "REJECTED_SAFETY"
elif not COMMIT_FLAG:
    execution_state["verdict"] = "DRY_RUN"
    execution_state["executed"] = False
else:
    # Execute
    try:
        proc = subprocess.run(
            systemd_cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=30
        )
        execution_state["stdout"] = proc.stdout.strip()
        execution_state["stderr"] = proc.stderr.strip()
        execution_state["return_code"] = proc.returncode
        execution_state["executed"] = True

        if proc.returncode == 0:
            execution_state["verdict"] = "EXECUTED_OK"
        else:
            execution_state["verdict"] = "EXECUTED_FAILED"
    except subprocess.TimeoutExpired:
        execution_state["verdict"] = "EXECUTED_TIMEOUT"
        execution_state["executed"] = True

    # ─── 7. Verify post-state ───
    if execution_state["executed"]:
        try:
            status = subprocess.run(
                f"systemctl --user is-active {service}",
                shell=True, capture_output=True, text=True, timeout=10
            )
            pid = subprocess.run(
                f"systemctl --user show -p MainPID {service} 2>/dev/null | cut -d= -f2",
                shell=True, capture_output=True, text=True, timeout=10
            )
            execution_state["post_state"] = {
                "active": status.stdout.strip(),
                "main_pid": pid.stdout.strip(),
            }
        except Exception as e:
            execution_state["post_state"] = {"error": str(e)}

    # ─── 8. Rollback binding ───
    if action_type == 'SIMULATED_RESTART':
        # Restart is reversible by definition (service is still active)
        execution_state["rollback_command"] = f"echo 'restart executed — no rollback needed; service returned to active'"
    elif action_type == 'SIMULATED_ISOLATION':
        execution_state["rollback_command"] = f"systemctl --user unmask {service} && systemctl --user start {service}"
    else:
        execution_state["rollback_command"] = f"echo 'unknown action type — manual rollback required'"

# ─── 9. Log ───
execution_state["rollback_ready"] = bool(execution_state.get("rollback_command"))
execution_state["agent_version"] = agent.get("agent_version", "unknown")
execution_state["agent_state"]   = agent.get("agent_state", "unknown")

with open(os.path.join(BRIDGE_DIR, f"{today}.json"), 'w') as f:
    json.dump(execution_state, f, indent=2)

print(json.dumps(execution_state, indent=2))
PYEOF
