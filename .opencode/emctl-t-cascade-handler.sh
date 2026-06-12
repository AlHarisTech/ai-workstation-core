#!/bin/sh
# EMCL-T Cascade Handler (Domain-aware) — invoked by systemd OnFailure=%n
# Routes failure to the correct domain EMCL-T instance.
set -e

SERVICE="$1"

# Determine domain from service name
case "$SERVICE" in
    mcp-filesystem*|mcp-git*|mcp-fetch*|mcp-memory*)
        DOMAIN="core"
        ;;
    chromadb*|github*)
        DOMAIN="integration"
        ;;
    *)
        DOMAIN="core"  # default fallback
        ;;
esac

exec /home/asem/workspace/.opencode/emctl-t.sh --domain "$DOMAIN" --cascade-signal "$SERVICE"
