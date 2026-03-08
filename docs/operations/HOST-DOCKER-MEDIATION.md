# Host Docker mediation (MVP)

PromptLock can mediate selected host Docker operations without mounting `docker.sock` into agent containers.

## Endpoint

`POST /v1/host/docker/execute`

- operator-auth required
- command must pass host docker policy checks

Payload example:

```json
{
  "command": ["docker", "ps"]
}
```

Response:

```json
{
  "exit_code": 0,
  "stdout_stderr": "..."
}
```

## Policy controls

Configured via `host_ops_policy`:
- `docker_allow_subcommands`
- `docker_compose_allow_verbs`
- `docker_ps_allowed_flags`
- `docker_images_allowed_flags`
- `docker_deny_substrings`
- `docker_timeout_sec`

This is a structured allowlist/denylist policy (subcommands, verbs, flags), not just free-form substring checks.

## Security notes
- this MVP is intentionally restrictive
- avoid adding `docker run`/`docker exec` until stronger policy model is ready
- all invocations are audit logged with actor attribution
