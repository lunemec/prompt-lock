# promptlock exec (prototype)

`promptlock exec` is a capability-first wrapper intended to run commands with lease-scoped secret injection.

## Example

```bash
# demo mode (auto-approve) - do not use in production
PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765 \
  go run ./cmd/promptlock exec \
  --agent ralph-r1 \
  --task TASK-3001 \
  --intent run_tests \
  --ttl 5 \
  --auto-approve \
  -- env | grep GITHUB_TOKEN
```

## Notes
- `--intent` resolves secrets via broker intent map.
- `--secrets` can be used explicitly instead of intent.
- `--auto-approve` exists only for local prototyping and should be disabled in production paths.

## Security direction
- Long-term default should require external human approval path.
- Wrapper should avoid exposing plaintext secrets in command output where feasible.
- Add command-policy controls for high-risk command forms.
