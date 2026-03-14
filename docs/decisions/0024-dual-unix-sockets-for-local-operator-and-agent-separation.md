# 0024 - Dual unix sockets for local operator and agent separation

- Status: accepted
- Date: 2026-03-14

## Context
PromptLock originally exposed one local Unix socket when running in hardened local mode. That single socket carried both agent-safe endpoints and operator-only endpoints. In practice, local container workflows needed the agent container to reach the broker socket, which meant the transport itself did not separate untrusted agent traffic from trusted operator traffic. The operator token still protected approval/bootstrap endpoints, but the local transport shape was broader than necessary and the CLI required repeated broker transport flags.

## Decision
1. Hardened local mode now defaults to two Unix sockets instead of one shared socket:
   - `/tmp/promptlock-agent.sock`
   - `/tmp/promptlock-operator.sock`
2. The agent socket exposes only agent-safe routes:
   - intent resolution
   - request/status/cancel/by-request
   - lease access/execute
   - auth pair/session mint
3. The operator socket exposes only operator routes:
   - pending queue
   - approve/deny
   - auth bootstrap/revoke
   - host Docker mediation
4. CLI commands auto-select sockets by role when no broker transport flags are provided:
   - operator commands use the operator socket
   - agent commands use the agent socket
   - `auth login` and `auth docker-run` span both sockets internally
5. The existing `unix_socket` field and `--broker-unix-socket` / `PROMPTLOCK_BROKER_UNIX_SOCKET` client compatibility path remain available for legacy single-socket deployments and explicit overrides.

## Consequences
- Local hardened usage is simpler: `promptlock watch`, `promptlock exec`, and `promptlock auth docker-run` no longer need repeated broker transport flags.
- The untrusted container path can mount only the agent socket; operator workflows stay on the host-only operator socket.
- Legacy single-socket configurations continue to work, but they are no longer the recommended local hardened shape.
- PromptLock's supported transport surface is now local-only Unix sockets; non-local transport is out of scope.

## Security implications
- Transport separation now narrows the local attack surface: an agent container with only the agent socket cannot even address operator-only endpoints.
- Operator token auth remains necessary. Socket separation reduces exposure but does not replace the human/operator authorization boundary.
- The local hardened default is stricter than the previous shared-socket model, but it does not by itself fix higher-layer authorization bugs; request and lease ownership must still be enforced correctly.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
