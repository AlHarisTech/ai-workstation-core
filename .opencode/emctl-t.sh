#!/bin/sh
# EMCL-T v2.3 — Distributed Control Surface (Domain-aware)
# Usage:
#   ./emctl-t.sh --domain core                       # Core domain cycle
#   ./emctl-t.sh --domain integration                # Integration domain cycle
#   ./emctl-t.sh --domain core --cascade-signal <svc> # Domain-specific cascade
#   ./emctl-t.sh --domain core --status              # Domain health
#   ./emctl-t.sh --domain all --coordinator          # Cross-domain coordinator

set -e

EMCTL_DIR="/home/asem/workspace/.opencode"
EMCTL_T_DIR="/home/asem/workspace/.opencode/emctl-t"
mkdir -p "$EMCTL_T_DIR"

STRESS_SCENARIO=""
STATUS_ONLY=""
CASCADE_SIGNAL=""
DOMAIN=""
COORDINATOR=""
AUTHORIZE_ACTION=""
AUTHORIZE_SERVICE=""
GENERATE_INTENTS=""

while [ $# -gt 0 ]; do
    case "$1" in
        --domain)          DOMAIN="$2"; shift 2 ;;
        --coordinator)     COORDINATOR="1"; shift ;;
        --generate-intents) GENERATE_INTENTS="1"; shift ;;
        --stress)          STRESS_SCENARIO="$2"; shift 2 ;;
        --status)          STATUS_ONLY="1"; shift ;;
        --cascade-signal)  CASCADE_SIGNAL="$2"; shift 2 ;;
        --authorize)       AUTHORIZE_ACTION="$2"; AUTHORIZE_SERVICE="$3"; shift 3 ;;
        *) echo "Usage: $0 --domain <core|integration|all> [--status|--cascade-signal <svc>|--coordinator|--authorize <action> <service>|--generate-intents]"; exit 1 ;;
    esac
done

if [ -z "$DOMAIN" ] && [ -z "$COORDINATOR" ] && [ -z "$AUTHORIZE_ACTION" ] && [ -z "$GENERATE_INTENTS" ]; then
    echo "Error: --domain <core|integration|all> required (use --domain all --coordinator for cross-domain)"
    exit 1
fi

STRESS_SCENARIO="$STRESS_SCENARIO" STATUS_ONLY="$STATUS_ONLY" CASCADE_SIGNAL="$CASCADE_SIGNAL" DOMAIN="$DOMAIN" COORDINATOR="$COORDINATOR" AUTHORIZE_ACTION="$AUTHORIZE_ACTION" AUTHORIZE_SERVICE="$AUTHORIZE_SERVICE" GENERATE_INTENTS="$GENERATE_INTENTS" python3 << 'PYEOF'
import json, os, sys, datetime, subprocess

EMCTL_DIR   = "/home/asem/workspace/.opencode"
EMCTL_T_DIR = "/home/asem/workspace/.opencode/emctl-t"

STRESS_SCENARIO = os.environ.get('STRESS_SCENARIO', '')
STATUS_ONLY     = os.environ.get('STATUS_ONLY', '')
CASCADE_SIGNAL  = os.environ.get('CASCADE_SIGNAL', '')
DOMAIN          = os.environ.get('DOMAIN', '')
COORDINATOR     = os.environ.get('COORDINATOR', '')
AUTHORIZE_ACTION  = os.environ.get('AUTHORIZE_ACTION', '')
AUTHORIZE_SERVICE = os.environ.get('AUTHORIZE_SERVICE', '')
GENERATE_INTENTS  = os.environ.get('GENERATE_INTENTS', '')

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

# ═══════════════════════════════════════════════════════════════════
# TRACK 2.3: DOMAIN CONFIGURATION
# ═══════════════════════════════════════════════════════════════════

DOMAINS = {
    'core': {
        'services': ['mcp-filesystem', 'mcp-git', 'mcp-fetch', 'mcp-memory'],
        'dependency_map': {
            'mcp-filesystem': [],
            'mcp-git':        ['mcp-filesystem'],
            'mcp-fetch':      ['mcp-filesystem', 'mcp-git'],
            'mcp-memory':     ['mcp-filesystem', 'mcp-git', 'mcp-fetch'],
        },
        'description': 'Core MCP runtime services',
    },
    'integration': {
        'services': ['chromadb-mcp', 'github-mcp'],
        'dependency_map': {
            'chromadb-mcp': [],
            'github-mcp':   [],
        },
        'description': 'External integration MCP services',
    },
}

ALL_SERVICES = []
for d in DOMAINS.values():
    ALL_SERVICES.extend(d['services'])

# ═══════════════════════════════════════════════════════════════════
# COORDINATOR MODE
# ═══════════════════════════════════════════════════════════════════

if COORDINATOR:
    coord_dir = os.path.join(EMCTL_T_DIR, "coordinator")
    os.makedirs(coord_dir, exist_ok=True)
    coord_path = os.path.join(coord_dir, "state.json")
    hist_path  = os.path.join(EMCTL_T_DIR, "coordinator", "history.json")

    # ─── Track 2.4: Intelligence Layer ───
    # Load domain states + failure histories
    domain_states = {}
    domain_details = {}
    for name in DOMAINS:
        degraded_path = os.path.join(EMCTL_T_DIR, name, "degraded.json")
        failures_path = os.path.join(EMCTL_T_DIR, name, "failures.json")
        ds = load_json(degraded_path)
        fs = load_json(failures_path)
        domain_states[name] = {
            "state": ds.get("state", "UNKNOWN"),
            "failure_count": ds.get("failure_count", 0),
            "last_policy": ds.get("last_policy_decision", {}),
        }
        # Collect recent failures for propagation analysis
        recent = [f for f in fs.get("failures", [])[-10:]]
        domain_details[name] = {
            "state": ds.get("state", "UNKNOWN"),
            "failure_count": ds.get("failure_count", 0),
            "recent_failures": recent,
            "transitions": ds.get("transitions", [])[-3:],
        }

    # Load history for trend analysis
    history = load_json(hist_path)
    if not history:
        history = {"ts": now_ts, "snapshots": []}

    core_state = domain_states.get('core', {}).get('state', 'UNKNOWN')
    int_state  = domain_states.get('integration', {}).get('state', 'UNKNOWN')
    core_fails = domain_states.get('core', {}).get('failure_count', 0)
    int_fails  = domain_states.get('integration', {}).get('failure_count', 0)

    # ─── 2.4.1: Risk Scoring ───
    STATE_RISK_MAP = {
        'FULL_OPERATIONAL':  0,
        'DEGRADED_ISOLATED': 2,
        'RECOVERING':        1,
        'STABILIZING':       4,
        'EMERGENCY':         5,
        'UNKNOWN':           3,
    }

    def domain_risk_score(state, failure_count):
        base = STATE_RISK_MAP.get(state, 3)
        failure_penalty = min(failure_count, 5) * 0.5
        return round(base + failure_penalty, 2)

    core_risk  = domain_risk_score(core_state, core_fails)
    int_risk   = domain_risk_score(int_state, int_fails)

    # Propagation risk: Core → Integration (filesystem dependency)
    PROPAGATION_MATRIX = {
        ('core', 'integration'): {
            'risk_factor': 0.3,  # core failure has 30% chance of affecting integration
            'rationale': 'core services provide filesystem/git layer that integration may depend on',
        },
        ('integration', 'core'): {
            'risk_factor': 0.05,
            'rationale': 'integration services are external-facing — minimal core impact',
        },
    }

    def propagation_risk(from_domain, to_domain):
        prop = PROPAGATION_MATRIX.get((from_domain, to_domain), {'risk_factor': 0.1})
        from_risk = domain_risk_score(
            domain_states.get(from_domain, {}).get('state', 'UNKNOWN'),
            domain_states.get(from_domain, {}).get('failure_count', 0)
        )
        # Propagation = source_risk * propagation_factor
        return round(from_risk * prop['risk_factor'], 2), prop['rationale']

    prop_core_to_int, prop_core_int_reason = propagation_risk('core', 'integration')
    prop_int_to_core, prop_int_core_reason = propagation_risk('integration', 'core')

    # Mesh risk = weighted combination
    mesh_risk = round(core_risk * 1.0 + int_risk * 0.5 + prop_core_to_int, 2)

    # ─── 2.4.2: Conflict Detection ───
    conflicts = []
    if core_state == 'FULL_OPERATIONAL' and int_fails >= 2:
        conflicts.append({
            'type': 'STATE_MISMATCH',
            'domains': ['core', 'integration'],
            'detail': f'core=FULL_OPERATIONAL but integration has {int_fails} failures',
            'resolution': 'integration failures likely external — no core cascade risk',
        })
    if core_state in ('STABILIZING', 'EMERGENCY') and int_state == 'FULL_OPERATIONAL':
        conflicts.append({
            'type': 'ASYMMETRIC_DEGRADATION',
            'domains': ['core', 'integration'],
            'detail': f'core={core_state} but integration=FULL_OPERATIONAL',
            'resolution': 'core failure contained — integration unaffected',
        })

    # ─── 2.4.3: Propagation Forecast ───
    forecast = []
    if core_risk >= 3:
        forecast.append({
            'source': 'core',
            'target': 'integration',
            'probability': prop_core_to_int,
            'impact': 'integration services may experience latency or filesystem access failures',
            'confidence': 'medium' if core_fails > 0 else 'low',
        })
    if int_risk >= 4:
        forecast.append({
            'source': 'integration',
            'target': 'core',
            'probability': prop_int_to_core,
            'impact': 'minimal — core does not depend on integration services',
            'confidence': 'high',
        })

    # ─── 2.4.4: Cross-domain Policy (intelligent) ───
    cross_policy = None
    if mesh_risk >= 8:
        cross_policy = {
            'directive': 'GLOBAL_EMERGENCY',
            'description': f'mesh risk {mesh_risk}: both domains degraded — full system escalation',
            'severity': 5,
            'mesh_risk': mesh_risk,
        }
    elif core_state == 'EMERGENCY':
        cross_policy = {
            'directive': 'CORE_EMERGENCY',
            'description': f'core EMERGENCY (risk={core_risk}) — isolate integration, escalate to EMCL',
            'severity': 4,
            'mesh_risk': mesh_risk,
        }
    elif core_state == 'STABILIZING':
        cross_policy = {
            'directive': 'CORE_STABILIZING',
            'description': f'core stabilizing (risk={core_risk}) — monitor, prepare for cascade',
            'severity': 3,
            'mesh_risk': mesh_risk,
        }
    elif core_state == 'DEGRADED_ISOLATED' and int_state == 'DEGRADED_ISOLATED':
        cross_policy = {
            'directive': 'DUAL_ISOLATED',
            'description': f'both domains isolated (core_risk={core_risk}, int_risk={int_risk}) — monitor recovery',
            'severity': 2,
            'mesh_risk': mesh_risk,
        }
    elif core_state == 'DEGRADED_ISOLATED' or int_state == 'DEGRADED_ISOLATED':
        cross_policy = {
            'directive': 'SINGLE_DEGRADED',
            'description': f'single domain degraded — mesh risk {mesh_risk}, cross-domain propagation low',
            'severity': 1,
            'mesh_risk': mesh_risk,
        }
    else:
        cross_policy = {
            'directive': 'NORMAL',
            'description': f'all domains operational — mesh risk {mesh_risk}',
            'severity': 0,
            'mesh_risk': mesh_risk,
        }

    # Mesh health
    if mesh_risk >= 6:
        mesh_health = "CRITICAL"
    elif mesh_risk >= 3:
        mesh_health = "WARN"
    else:
        mesh_health = "HEALTHY"

    # ─── 2.4.5: Save snapshot to history (for trend analysis) ───
    snapshot = {
        "ts": now_ts,
        "core_risk": core_risk,
        "int_risk": int_risk,
        "mesh_risk": mesh_risk,
        "mesh_health": mesh_health,
        "core_state": core_state,
        "int_state": int_state,
    }
    history["snapshots"] = (history.get("snapshots", []) + [snapshot])[-50:]
    history["ts"] = now_ts
    save_json(hist_path, history)

    # ─── 2.4.6: Build intelligence report ───
    coord_state = {
        "ts": now_ts,
        "domain_states": domain_states,
        "intelligence": {
            "risk_scores": {
                "core": core_risk,
                "integration": int_risk,
                "mesh": mesh_risk,
            },
            "propagation_risk": {
                "core_to_integration": prop_core_to_int,
                "core_to_int_rationale": prop_core_int_reason,
                "integration_to_core": prop_int_to_core,
                "int_to_core_rationale": prop_int_core_reason,
            },
            "conflicts": conflicts,
            "forecast": forecast,
        },
        "cross_policy": cross_policy,
        "mesh_health": mesh_health,
        "_version": "Track 2.4 — Coordinator Intelligence Layer",
    }
    save_json(coord_path, coord_state)
    print(json.dumps(coord_state, indent=2))
    sys.exit(0)

# ═══════════════════════════════════════════════════════════════════
# TRACK 2.5: ADAL — Autonomous Decision Authorization Layer
# ═══════════════════════════════════════════════════════════════════

if AUTHORIZE_ACTION:
    action_type = AUTHORIZE_ACTION
    target_svc  = AUTHORIZE_SERVICE

    # 1. Run coordinator to get fresh intelligence
    coord_args = [sys.argv[0], '--domain', 'all', '--coordinator']
    import subprocess as sp
    try:
        cp = sp.run(
            ['/home/asem/workspace/.opencode/emctl-t.sh', '--domain', 'all', '--coordinator'],
            capture_output=True, text=True, timeout=30
        )
        if cp.returncode != 0 or not cp.stdout:
            result = {"ts": now_ts, "decision": "DENIED", "reason": "coordinator unavailable", "action": action_type, "service": target_svc}
            print(json.dumps(result, indent=2))
            sys.exit(0)
        intelligence = json.loads(cp.stdout)
    except Exception as e:
        result = {"ts": now_ts, "decision": "DENIED", "reason": f"coordinator error: {e}", "action": action_type, "service": target_svc}
        print(json.dumps(result, indent=2))
        sys.exit(0)

    # 2. Extract intelligence signals
    risk_scores = intelligence.get('intelligence', {}).get('risk_scores', {})
    propagation = intelligence.get('intelligence', {}).get('propagation_risk', {})
    conflicts   = intelligence.get('intelligence', {}).get('conflicts', [])
    forecast    = intelligence.get('intelligence', {}).get('forecast', [])
    mesh_health = intelligence.get('mesh_health', 'UNKNOWN')
    cross_policy = intelligence.get('cross_policy', {})
    domain_states = intelligence.get('domain_states', {})

    mesh_risk = risk_scores.get('mesh', 0)
    core_risk = risk_scores.get('core', 0)
    int_risk  = risk_scores.get('integration', 0)

    # Determine target domain
    target_domain = 'integration' if target_svc.startswith('chromadb') or target_svc.startswith('github') else 'core'
    target_state = domain_states.get(target_domain, {}).get('state', 'UNKNOWN')
    target_risk  = risk_scores.get(target_domain, 0)

    # 3. Authorization Rules
    deny_reasons = []

    # Rule A1: Mesh overload — no action allowed during global emergency
    if mesh_health == 'CRITICAL' and cross_policy.get('severity', 0) >= 5:
        deny_reasons.append(f"mesh CRITICAL (risk={mesh_risk}) — global emergency, all actions denied")

    # Rule A2: Target domain in EMERGENCY — no hard actions
    if target_state == 'EMERGENCY':
        deny_reasons.append(f"target domain {target_domain} in EMERGENCY — actions denied")

    # Rule A3: Restart in stable domain is safe
    if action_type == 'SIMULATED_RESTART' and target_risk <= 2.0:
        pass  # allowed

    # Rule A4: Isolation in any degraded domain requires escalation
    elif action_type == 'SIMULATED_ISOLATION' and target_state != 'FULL_OPERATIONAL':
        deny_reasons.append(f"isolation during {target_state} — escalate to EMCL instead")

    # Rule A5: Cross-domain action during propagation risk
    prop_key = f"{'core' if target_domain == 'integration' else 'integration'}_to_{target_domain}"
    prop_risk = propagation.get(f"{'core' if target_domain == 'integration' else 'integration'}_to_{target_domain}", 0)
    # Fix: the actual keys use "core_to_integration" and "integration_to_core"
    if target_domain == 'core':
        prop_val = propagation.get('integration_to_core', 0)
    else:
        prop_val = propagation.get('core_to_integration', 0)
    if prop_val >= 1.0 and action_type in ('SIMULATED_RESTART', 'SIMULATED_ISOLATION'):
        deny_reasons.append(f"propagation risk {prop_val} from other domain — action denied")

    # Rule A6: Conflict active — escalate
    if conflicts:
        deny_reasons.append(f"active domain conflict — escalate to EMCL")

    # 4. Decision
    decision = "AUTHORIZED" if not deny_reasons else "DENIED"
    if cross_policy.get('severity', 0) >= 4 and decision == "AUTHORIZED":
        decision = "ESCALATED"
        deny_reasons.append("high severity cross-policy — escalated to EMCL for confirmation")

    result = {
        "ts": now_ts,
        "decision": decision,
        "action": action_type,
        "service": target_svc,
        "target_domain": target_domain,
        "mesh_risk": mesh_risk,
        "target_risk": target_risk,
        "mesh_health": mesh_health,
        "deny_reasons": deny_reasons,
        "intelligence_snapshot": {
            "risk_scores": risk_scores,
            "propagation_risk": propagation,
            "conflicts": conflicts,
        },
        "_version": "Track 2.5 — ADAL v1",
    }

    # Log authorization
    auth_dir = os.path.join(EMCTL_DIR, "emctl-t", "authorizations")
    os.makedirs(auth_dir, exist_ok=True)
    auth_path = os.path.join(auth_dir, f"{today}.jsonl")
    with open(auth_path, 'a') as af:
        af.write(json.dumps(result) + "\n")

    print(json.dumps(result, indent=2))
    sys.exit(0)

# ═══════════════════════════════════════════════════════════════════
# TRACK 2.6: AIIL — Autonomous Intent Initiation Layer
# ═══════════════════════════════════════════════════════════════════

if GENERATE_INTENTS:
    # 1. Get fresh coordinator intelligence
    try:
        cp = subprocess.run(
            ['/home/asem/workspace/.opencode/emctl-t.sh', '--domain', 'all', '--coordinator'],
            capture_output=True, text=True, timeout=30
        )
        if cp.returncode != 0 or not cp.stdout:
            result = {"ts": now_ts, "intents": [], "error": "coordinator unavailable"}
            print(json.dumps(result, indent=2))
            sys.exit(0)
        intel = json.loads(cp.stdout)
    except Exception as e:
        result = {"ts": now_ts, "intents": [], "error": str(e)}
        print(json.dumps(result, indent=2))
        sys.exit(0)

    # 2. Extract signals
    risk_scores = intel.get('intelligence', {}).get('risk_scores', {})
    propagation = intel.get('intelligence', {}).get('propagation_risk', {})
    conflicts   = intel.get('intelligence', {}).get('conflicts', [])
    forecast    = intel.get('intelligence', {}).get('forecast', [])
    mesh_health = intel.get('mesh_health', 'UNKNOWN')
    cross_policy = intel.get('cross_policy', {})
    domain_states = intel.get('domain_states', {})

    core_state = domain_states.get('core', {}).get('state', 'UNKNOWN')
    int_state  = domain_states.get('integration', {}).get('state', 'UNKNOWN')
    core_risk  = risk_scores.get('core', 0)
    int_risk   = risk_scores.get('integration', 0)
    mesh_risk  = risk_scores.get('mesh', 0)

    # 3. Load recent intents for rate limiting
    intents_dir = os.path.join(EMCTL_DIR, "emctl-t", "intents")
    os.makedirs(intents_dir, exist_ok=True)
    intents_log = os.path.join(intents_dir, f"{today}.jsonl")
    recent_intents = []
    if os.path.exists(intents_log):
        with open(intents_log) as f:
            for line in f:
                try:
                    recent_intents.append(json.loads(line))
                except:
                    pass
        # Keep last 50
        recent_intents = recent_intents[-50:]

    # Helper: rate limit check (same intent_type+service within 300s)
    def is_rate_limited(intent_type, service):
        cutoff = (datetime.datetime.utcnow() - datetime.timedelta(seconds=300)).strftime('%Y-%m-%dT%H:%M:%SZ')
        for r in recent_intents:
            if r.get('intent_type') == intent_type and r.get('target_service') == service:
                if r.get('ts', '') >= cutoff:
                    return True
        return False

    # 4. Intent generation rules
    generated_intents = []

    # Rule I1: Core degraded → generate RESTART intent on failed service
    if core_state == 'DEGRADED_ISOLATED':
        # Find the failed service from recent failures
        core_failures = domain_states.get('core', {}).get('last_policy', {})
        failed_svc = core_failures.get('service')
        if failed_svc and not is_rate_limited('RESTART', failed_svc):
            generated_intents.append({
                'intent_type': 'RESTART',
                'target_service': failed_svc,
                'target_domain': 'core',
                'confidence': round(max(0.5, 1.0 - core_risk/10), 2),
                'reasoning': f'core domain isolated — {failed_svc} failure detected',
                'mesh_context': {'mesh_risk': mesh_risk, 'mesh_health': mesh_health},
            })

    # Rule I2: Integration degraded → generate RESTART intent
    if int_state == 'DEGRADED_ISOLATED':
        int_failures = domain_states.get('integration', {}).get('last_policy', {})
        failed_svc = int_failures.get('service')
        if failed_svc and not is_rate_limited('RESTART', failed_svc):
            generated_intents.append({
                'intent_type': 'RESTART',
                'target_service': failed_svc,
                'target_domain': 'integration',
                'confidence': round(max(0.6, 1.0 - int_risk/8), 2),
                'reasoning': f'integration domain isolated — {failed_svc} failure',
                'mesh_context': {'mesh_risk': mesh_risk, 'mesh_health': mesh_health},
            })

    # Rule I3: Propagation risk → generate MONITOR intent
    prop_core_to_int = propagation.get('core_to_integration', 0)
    if prop_core_to_int >= 0.8 and not is_rate_limited('MONITOR', 'integration'):
        generated_intents.append({
            'intent_type': 'MONITOR',
            'target_service': 'integration_domain',
            'target_domain': 'integration',
            'confidence': round(min(0.9, prop_core_to_int/2), 2),
            'reasoning': f'propagation risk {prop_core_to_int} from core to integration',
            'mesh_context': {'propagation_risk': prop_core_to_int},
        })

    # Rule I4: Asymmetric conflict → generate ESCALATE intent
    for c in conflicts:
        if not is_rate_limited('ESCALATE', '_'.join(c.get('domains', []))):
            generated_intents.append({
                'intent_type': 'ESCALATE',
                'target_service': 'mesh_coordinator',
                'target_domain': 'all',
                'confidence': 0.85,
                'reasoning': f'conflict: {c.get("detail","")}',
                'resolution': c.get('resolution', ''),
                'mesh_context': {'conflict_type': c.get('type')},
            })

    # Rule I5: Degraded + stable domain → generate ISOLATE intent
    if core_state == 'DEGRADED_ISOLATED' and int_state == 'FULL_OPERATIONAL':
        if not is_rate_limited('ISOLATE', 'core_domain'):
            generated_intents.append({
                'intent_type': 'ISOLATE',
                'target_service': 'core_domain',
                'target_domain': 'core',
                'confidence': 0.7,
                'reasoning': 'core isolated, integration healthy — contain core only',
                'mesh_context': {'core_state': core_state, 'int_state': int_state},
            })
    elif int_state == 'DEGRADED_ISOLATED' and core_state == 'FULL_OPERATIONAL':
        if not is_rate_limited('ISOLATE', 'integration_domain'):
            generated_intents.append({
                'intent_type': 'ISOLATE',
                'target_service': 'integration_domain',
                'target_domain': 'integration',
                'confidence': 0.7,
                'reasoning': 'integration isolated, core healthy — contain integration',
                'mesh_context': {'core_state': core_state, 'int_state': int_state},
            })

    # Rule I6: STABILIZING without recovery → generate FULL_STATE_CAPTURE
    if core_state == 'STABILIZING' and int_state != 'FULL_OPERATIONAL':
        if not is_rate_limited('FULL_STATE_CAPTURE', 'mesh'):
            generated_intents.append({
                'intent_type': 'FULL_STATE_CAPTURE',
                'target_service': 'mesh',
                'target_domain': 'all',
                'confidence': 0.9,
                'reasoning': 'both domains non-operational — capture full state',
                'mesh_context': {'core_state': core_state, 'int_state': int_state},
            })

    # 5. Log and return
    result = {
        "ts": now_ts,
        "intents": generated_intents,
        "intent_count": len(generated_intents),
        "mesh_context": {
            "mesh_health": mesh_health,
            "mesh_risk": mesh_risk,
            "core_state": core_state,
            "int_state": int_state,
        },
        "_version": "Track 2.6 — AIIL v1",
    }

    # Append to intent log
    for intent in generated_intents:
        with open(intents_log, 'a') as f:
            f.write(json.dumps({"ts": now_ts, **intent}) + "\n")

    print(json.dumps(result, indent=2))
    sys.exit(0)

# ═══════════════════════════════════════════════════════════════════
# DOMAIN-SPECIFIC OPERATIONS
# ═══════════════════════════════════════════════════════════════════

domain_config = DOMAINS.get(DOMAIN)
if not domain_config:
    # Try with "all" — for coordinator already handled above
    result = {"error": f"unknown domain '{DOMAIN}'. Valid: {list(DOMAINS.keys())} + all"}
    print(json.dumps(result, indent=2))
    sys.exit(1)

SERVICES         = domain_config['services']
DEPENDENCY_MAP   = domain_config['dependency_map']

# Domain-specific state directories
DOMAIN_DIR = os.path.join(EMCTL_T_DIR, DOMAIN)
os.makedirs(DOMAIN_DIR, exist_ok=True)

state_path    = os.path.join(DOMAIN_DIR, "state.json")
degraded_path = os.path.join(DOMAIN_DIR, "degraded.json")
failures_path = os.path.join(DOMAIN_DIR, "failures.json")

# ─── Policy Engine (from Track 2.2) ───
FAILURE_TYPES = {
    'NODE_CRITICAL':    'service crashed unexpectedly',
    'TRANSIENT':        'brief failure, auto-recovered',
    'CASCADE_RISK':     'failure in dependency chain during degraded state',
    'LOCALIZED':        'integration failure — no core impact',
    'EMERGENCY':        'multiple failures — system unstable',
}

def classify_failure(service, current_degraded_state, recent_failures):
    if service not in SERVICES:
        return 'LOCALIZED', 'service outside domain — cross-domain event'
    if current_degraded_state in ('STABILIZING', 'EMERGENCY'):
        return 'CASCADE_RISK', f'failure during {current_degraded_state}'
    deps = DEPENDENCY_MAP.get(service, [])
    failed_deps = [d for d in deps if d in recent_failures]
    if failed_deps:
        return 'CASCADE_RISK', f'dependency failed: {", ".join(failed_deps)}'
    recent_count = len(recent_failures)
    if recent_count >= 2:
        return 'EMERGENCY', f'{recent_count} failures in window'
    return 'NODE_CRITICAL', f'{service} failed — isolated'

DEGRADED_STATES = ['FULL_OPERATIONAL', 'DEGRADED_ISOLATED', 'RECOVERING', 'STABILIZING', 'EMERGENCY']

def transition_degraded_state(current, failure_count, service_is_recovering):
    if failure_count >= 3:
        return 'EMERGENCY'
    if failure_count >= 2:
        return 'STABILIZING'
    if service_is_recovering:
        return 'RECOVERING'
    if failure_count >= 1:
        return 'DEGRADED_ISOLATED'
    return 'FULL_OPERATIONAL'

POLICY_RULES = [
    {'id': 'P-CORE-1', 'condition': lambda cls, svc: cls == 'NODE_CRITICAL',
     'directive': 'RESTART_ISOLATED', 'description': 'node failure — restart with verification'},
    {'id': 'P-CORE-2', 'condition': lambda cls, svc: cls == 'CASCADE_RISK',
     'directive': 'ESCALATE_TO_EMCL', 'description': 'cascade risk — hand off to EMCL'},
    {'id': 'P-INT-1', 'condition': lambda cls, svc: cls == 'LOCALIZED',
     'directive': 'MONITOR_ONLY', 'description': 'out-of-domain or integration — observe'},
    {'id': 'P-SYS-1', 'condition': lambda cls, svc: cls == 'EMERGENCY',
     'directive': 'FULL_STATE_CAPTURE', 'description': 'multiple failures — capture state'},
    {'id': 'P-DEFAULT', 'condition': lambda cls, svc: True,
     'directive': 'MONITOR_ONLY', 'description': 'default fallback'},
]

def apply_policy(classification, service):
    for rule in POLICY_RULES:
        if rule['condition'](classification, service):
            return rule
    return POLICY_RULES[-1]

def get_service_states():
    states = {}
    for svc in ALL_SERVICES:
        try:
            active = subprocess.run(
                ['systemctl', '--user', 'is-active', svc],
                capture_output=True, text=True, timeout=5
            ).stdout.strip()
            pid = subprocess.run(
                ['systemctl', '--user', 'show', '-p', 'MainPID', svc],
                capture_output=True, text=True, timeout=5
            ).stdout.strip().split('=')[-1]
            states[svc] = {'active': active, 'pid': pid}
        except Exception:
            states[svc] = {'active': 'unknown', 'pid': '?'}
    return states

# ─── Load domain state ───
temporal_state = load_json(state_path)
if not temporal_state:
    temporal_state = {
        "ts": now_ts, "cycle_count": 0,
        "last_cycle_ts": None, "last_cycle_duration_ms": None,
        "avg_duration_ms": None, "missed_cycles": 0, "total_missed": 0,
        "timing_drift_ms": 0, "health": "UNKNOWN",
        "domain": DOMAIN,
    }
    save_json(state_path, temporal_state)

degraded_state = load_json(degraded_path)
if not degraded_state:
    degraded_state = {
        "ts": now_ts, "state": "FULL_OPERATIONAL",
        "domain": DOMAIN,
        "previous_state": None, "transitions": [],
        "failure_count": 0, "recent_failures": [],
        "last_policy_decision": None,
    }
    save_json(degraded_path, degraded_state)

failures = load_json(failures_path)
if not failures:
    failures = {"ts": now_ts, "domain": DOMAIN, "failures": []}
    save_json(failures_path, failures)

# ═══════════════════════════════════════════════════════════════════
# MODE DISPATCH
# ═══════════════════════════════════════════════════════════════════

if STATUS_ONLY:
    temporal_state["ts"] = now_ts
    temporal_state["domain"] = DOMAIN
    temporal_state["degraded_state"] = degraded_state.get("state")
    temporal_state["failure_count"] = degraded_state.get("failure_count", 0)
    temporal_state["last_policy_directive"] = degraded_state.get("last_policy_decision", {}).get("directive")
    print(json.dumps(temporal_state, indent=2))
    sys.exit(0)

# ──────────────────
# --cascade-signal: OnFailure handler (domain-aware)
# ──────────────────
if CASCADE_SIGNAL:
    svc = CASCADE_SIGNAL.replace('.service', '')
    service_states = get_service_states()
    current_degraded = degraded_state.get("state", "FULL_OPERATIONAL")

    cutoff = (datetime.datetime.utcnow() - datetime.timedelta(seconds=300)).strftime('%Y-%m-%dT%H:%M:%SZ')
    recent = [f for f in failures.get("failures", []) if f.get("ts", "") >= cutoff]
    recent_services = [f.get("service") for f in recent]

    classification, class_reason = classify_failure(svc, current_degraded, recent_services)
    policy = apply_policy(classification, svc)

    failure_event = {
        "ts": now_ts, "service": svc, "domain": DOMAIN,
        "classification": classification,
        "classification_reason": class_reason,
        "policy_rule": policy["id"],
        "policy_directive": policy["directive"],
    }
    failures["failures"].append(failure_event)
    failures["failures"] = failures["failures"][-100:]
    save_json(failures_path, failures)

    failure_count = len(recent) + 1
    recovering = any(s.get("active") == "active" for s in recent) if recent else False
    new_state = transition_degraded_state(current_degraded, failure_count, recovering)

    if new_state != current_degraded:
        transition = {"ts": now_ts, "from": current_degraded, "to": new_state, "trigger": f"failure: {svc}"}
        degraded_state["previous_state"] = current_degraded
        degraded_state["state"] = new_state
        degraded_state["transitions"] = degraded_state.get("transitions", []) + [transition]
    degraded_state["failure_count"] = failure_count
    degraded_state["ts"] = now_ts
    degraded_state["last_policy_decision"] = {
        "ts": now_ts, "service": svc, "domain": DOMAIN,
        "classification": classification,
        "policy_rule": policy["id"],
        "directive": policy["directive"],
    }
    save_json(degraded_path, degraded_state)

    result = {
        "ts": now_ts, "domain": DOMAIN, "mode": "CASCADE_POLICY",
        "signal": svc, "classification": classification,
        "classification_reason": class_reason,
        "degraded_state": {
            "current": new_state, "previous": current_degraded,
            "failure_count": failure_count,
        },
        "policy": policy,
        "service_states": service_states,
    }
    print(json.dumps(result, indent=2))
    sys.exit(0)

# ──────────────────
# Normal temporal cycle (domain-scoped)
# ──────────────────
EXPECTED_INTERVAL_S = 300
cycle_start = datetime.datetime.utcnow()
drift_s = 0
if temporal_state.get("last_cycle_ts"):
    last_ts = datetime.datetime.strptime(temporal_state["last_cycle_ts"], '%Y-%m-%dT%H:%M:%SZ')
    actual_interval = (cycle_start - last_ts).total_seconds()
    drift_s = actual_interval - EXPECTED_INTERVAL_S
    temporal_state["timing_drift_ms"] = round(drift_s * 1000, 1)

if abs(drift_s) > EXPECTED_INTERVAL_S * 2:
    temporal_state["missed_cycles"] += 1
    temporal_state["total_missed"] += 1

duration_ms = round((datetime.datetime.utcnow() - cycle_start).total_seconds() * 1000, 1)
temporal_state["ts"] = now_ts
temporal_state["cycle_count"] += 1
temporal_state["last_cycle_ts"] = now_ts
temporal_state["last_cycle_duration_ms"] = duration_ms
prev_avg = temporal_state.get("avg_duration_ms")
temporal_state["avg_duration_ms"] = round(0.9 * (prev_avg or duration_ms) + 0.1 * duration_ms, 1)

temporal_state["health"] = "HEALTHY"
save_json(state_path, temporal_state)

report = {
    "ts": now_ts, "domain": DOMAIN,
    "cycle": temporal_state["cycle_count"],
    "services": SERVICES,
    "degraded_state": degraded_state.get("state"),
    "failure_count": degraded_state.get("failure_count", 0),
    "health": temporal_state["health"],
    "domain_description": domain_config['description'],
    "_note": f"EMCL-T v2.3 domain: {DOMAIN}",
}
print(json.dumps(report, indent=2))
PYEOF