# promptlock exec (prototype)

`promptlock exec` is a capability-first wrapper intended to run commands with lease-scoped secret injection.

## Example

```bash
# Terminal A: human approval watcher
PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765 \
  go run ./cmd/promptlock approve-queue

# Terminal B: agent command waiting for approval
PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765 \
  go run ./cmd/promptlock exec \
  --agent ralph-r1 \
  --task TASK-3001 \
  --intent run_tests \
  --ttl 5 \
  -- bash -lc 'echo running tests'
```

For local demo only (no external approval watcher):

```bash
PROMPTLOCK_DEV_MODE=1 PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765 \
  go run ./cmd/promptlock exec --intent run_tests --ttl 5 --auto-approve -- env
```

## Notes
- `--intent` resolves secrets via broker intent map.
- `--secrets` can be used explicitly instead of intent.
- Wrapper computes command and working-directory fingerprints and includes them in lease/access calls.
- Default mode waits for external human approval (`--wait-approve`, `--poll-interval`).
- `promptlock approve-queue` is a host-side watcher CLI for approving/denying pending requests.
- `--auto-approve` exists only for local prototyping and requires `PROMPTLOCK_DEV_MODE=1`.
- Basic command policy blocks risky secret-dumping command patterns unless `--allow-risky-command` is explicitly set.

## Security direction
- Long-term default should require external human approval path.
- Wrapper should avoid exposing plaintext secrets in command output where feasible.
- Add command-policy controls for high-risk command forms.
