# Contributing to AI Workstation Core

## Architecture First

AI Workstation Core is an architecture-driven project. All changes must be preceded by architectural analysis and documented in an ADR.

## ADR Process

1. Create a new ADR in `.ai/ADR_LOG.md` following the established template
2. Each ADR must document:
   - Context (what problem)
   - Decision (what change)
   - Forbidden Alternatives (what was rejected and why)
   - Consequences (positive, negative, neutral)
   - Failure Modes Prevented
   - Governance Implications
3. ADR must be accepted before implementation begins

## Development Workflow

1. Read `.ai/GOVERNANCE.md` — understand the platform laws
2. Read `.ai/ARCHITECTURE.md` — understand the full architecture
3. Check `.ai/ADR_LOG.md` — understand prior decisions
4. Propose changes via ADR before implementing

## Standards

- **No project-specific assumptions** — the platform must remain agnostic
- **Fail-closed** — all enforcement points must deny on uncertainty
- **Evidence-first** — all operations must produce audit records
- **Minimal-diff** — changes must not break existing project configurations

## Pull Request Requirements

- All PRs must reference an ADR
- All PRs must include governance alignment verification
- All PRs must pass the governance audit before merge

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). All contributors must adhere to it.
