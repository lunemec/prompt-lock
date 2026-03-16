# 0029 - Workspace-derived setup and container-first quickstart

- Status: accepted
- Date: 2026-03-16

## Context
PromptLock's supported OSS story is a local-only hardened deployment where the broker runs on the host, only the agent socket is mounted into the untrusted container, and the first meaningful proof should come from a container-originated `auth docker-run` flow. The repo previously documented that shape, but the actual quickstart still required users to hand-write a config under `/tmp`, manually export multiple env vars, and translate that setup into three separate terminals.

That was workable, but it created avoidable friction and invited unsafe shortcuts such as storing supported config or state under the repo workspace simply because that path was convenient.

## Decision
1. Add a dedicated `promptlock setup` CLI workflow for the local hardened Docker quickstart.
2. The setup command derives a deterministic host-side instance directory from the current workspace root instead of writing supported config/state under the repo tree.
3. The generated quickstart instance includes:
   - `config.json`
   - `instance.env`
   - durable auth/request state paths
   - host audit path
   - per-workspace agent/operator Unix sockets
4. `instance.env` is sourceable and exports the env needed to run the documented local hardened flow without hand-editing:
   - `PROMPTLOCK_CONFIG`
   - `PROMPTLOCK_AGENT_UNIX_SOCKET`
   - `PROMPTLOCK_OPERATOR_UNIX_SOCKET`
   - `PROMPTLOCK_OPERATOR_TOKEN`
   - `PROMPTLOCK_AUTH_STORE_KEY`
   - a demo secret env value for the first local quickstart run
5. The README quickstart should lead with that setup flow and should assume the first real agent request comes from inside Docker via `promptlock auth docker-run`.
6. Expose the setup workflow via `make setup-local-docker` in addition to the raw CLI command.

## Consequences
- First-run UX is substantially simpler for repo-based evaluation and prerelease OSS testing.
- Supported config/state stay outside the repo workspace by default, which is more honest to the trust boundary than a repo-local quickstart.
- The generated quickstart remains explicitly lab-oriented: it favors a working container-first demo and visibility over a fully production-shaped operator bootstrap.

## Security implications
- The setup path reduces pressure to store supported config, audit, and durable state inside the agent-controlled workspace.
- `instance.env` contains sensitive local quickstart material (operator token, auth-store key, demo secret). It must remain host-side and private to the operator account.
- The generated quickstart config uses `execution_policy.output_security_mode=raw` for the first broker-exec demo. That is a convenience tradeoff for the quickstart, not the recommended hardened steady state.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
