---
name: secret-request
description: Request short-lived secret leases from PromptLock with explicit human approval.
---

# Secret Request Skill

Use this when an agent needs secrets for build/test/e2e tasks.

## Rules
- Never assume direct access to secret files.
- Request only required secrets.
- Ask for minimal practical TTL.
- Include clear reason and task id.
- If denied, continue with fallback plan or ask for narrower scope.

## Request template

The published container image ships this helper on `PATH` as `secretctl.sh`.
If you are working from a repo checkout instead of the image, `scripts/secretctl.sh` is the same helper.

```bash
secretctl.sh request \
  --agent <agent_id> \
  --task <task_id> \
  --ttl <minutes> \
  --reason "<why secrets are needed>" \
  --secret <secret_1> \
  --secret <secret_2>
```

Capture `request_id` from response and inform human:
- what was requested
- why
- TTL requested

## Access template

After approval, use lease token:

```bash
secretctl.sh access --lease <lease_token> --secret <secret_name>
```

## Recommended TTL by task type
- unit tests: 5–10 min
- integration tests: 10–20 min
- e2e setup + verify: 20–30 min

## Safety
- Do not print secret values in logs.
- Do not persist secrets to repo files.
- Clear env vars after use.
