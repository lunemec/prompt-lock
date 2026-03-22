# demo-envs

Disposable demo files and commands for PromptLock `env_path` flows.

- `github.env`: demo `github_token` value used by `run_tests` quickstart intent examples.
- `showcase/`: tiny Go test package that proves leased env + extra demo metadata env are present.

Quick local run (host shell):

```bash
GITHUB_TOKEN=FAKE_GITHUB_TOKEN make demo-run-env-showcase-tests
```

PromptLock MCP run (agent request):

```json
{
  "intent": "run_tests",
  "command": ["make", "demo-run-env-showcase-tests"],
  "env_path": "demo-envs/github.env"
}
```
