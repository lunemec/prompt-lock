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

If the Docker command already ran but the final result audit cannot be persisted, the response also includes:

```json
{
  "audit_warning": "durability persistence unavailable; broker closed for safety"
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
- the broker must pass the durability gate and write `host_docker_execute_started` before dispatching the host command
- result audit is attempted immediately after the command returns; if that write fails after side effects, the broker returns `audit_warning` and closes the durability gate
- all invocations keep actor attribution in audit events, but post-exec result recording is best-effort because host-side side effects cannot be rolled back by the broker
