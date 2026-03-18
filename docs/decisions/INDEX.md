# ADR Index

This index is the canonical entrypoint for repository decisions.
ADR metadata was normalized to the current status and supersession conventions on 2026-03-09.

| ADR | Status | Date | Title | Supersedes |
|---|---|---|---|---|
| 0001 | accepted | 2026-03-07 | Project governance requirements | - |
| 0002 | accepted | 2026-03-07 | PromptLock v1 requirements baseline | - |
| 0003 | accepted | 2026-03-07 | Codex token access model (v1) | - |
| 0004 | accepted | 2026-03-07 | Codex CLI integration strategy | - |
| 0005 | accepted | 2026-03-07 | Multi-CLI authentication integration model | - |
| 0006 | accepted | 2026-03-07 | Intent API and execute-with-secret injection model | - |
| 0007 | accepted | 2026-03-07 | TTL bypass prevention and secret non-disclosure defaults | - |
| 0008 | accepted | 2026-03-07 | Protocol exposure strategy: MCP first, ACP later | - |
| 0009 | accepted | 2026-03-07 | Secret delivery mechanisms: env, ephemeral files, and in-memory handles | - |
| 0010 | accepted | 2026-03-07 | Leakage containment and policy enforcement model | - |
| 0011 | accepted | 2026-03-07 | Agent auth: host pairing plus refreshable sessions | - |
| 0012 | accepted | 2026-03-07 | Broker-exec as preferred secure mode | - |
| 0013 | accepted | 2026-03-07 | MCP adapter scaffold (capability-first) | - |
| 0014 | accepted | 2026-03-08 | Host Docker access via PromptLock approval | - |
| 0015 | accepted | 2026-03-08 | Network egress control via PromptLock policy | - |
| 0016 | accepted | 2026-03-08 | Execution-surface policy boundaries | - |
| 0017 | accepted | 2026-03-09 | Dev-mode risk signaling and unauthenticated non-local TCP guard | - |
| 0018 | accepted | 2026-03-09 | Production deployment guardrails and state durability | - |
| 0019 | accepted | 2026-03-09 | Storage fsync report HMAC attestation | - |
| 0020 | accepted | 2026-03-10 | Production hardening: release gates and persistence safety | - |
| 0021 | accepted | 2026-03-10 | External request/lease state backend path | - |
| 0022 | accepted | 2026-03-10 | SOPS-managed runtime and fsync key-material loading | - |
| 0023 | accepted | 2026-03-14 | Operator watch command and minimal terminal approval UI | - |
| 0024 | accepted | 2026-03-14 | Dual unix sockets for local operator and agent separation | - |
| 0025 | superseded | 2026-03-14 | Local-only OSS v1 transport scope and TLS/mTLS quarantine | - |
| 0026 | accepted | 2026-03-14 | Exact executable identity and minimal child-process environment | - |
| 0027 | accepted | 2026-03-14 | Broker-managed executable resolution | - |
| 0028 | accepted | 2026-03-14 | Local-only transport and removal of TCP TLS/mTLS paths | 0025 |
| 0029 | accepted | 2026-03-16 | Workspace-derived setup and container-first quickstart | - |
| 0030 | accepted | 2026-03-18 | Single `promptlock` CLI surface with daemon lifecycle subcommands | - |

Clarification notes:
- `0014` was clarified on 2026-03-14: host-Docker mediation guarantees a pre-dispatch audit gate, while post-dispatch result recording is best-effort with an explicit durability warning if it fails after side effects.
