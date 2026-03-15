# Example workflow

This is a mock-broker localhost-TCP demo only. It is not the supported hardened deployment path.

## 1) Start broker
```bash
go run ./cmd/promptlock-mock-broker
```

## 2) Agent requests lease
```bash
scripts/secretctl.sh request \
  --agent ralph-r1 \
  --task TASK-2001 \
  --ttl 15 \
  --reason "Need GitHub + npm credentials for integration test run" \
  --secret github_token \
  --secret npm_token
```

## 3) Human approves request
```bash
APPROVE_ENDPOINT_STYLE=path scripts/human-approve.sh <request_id> 15
```

## 4) Agent accesses approved secret
```bash
scripts/secretctl.sh access --lease <lease_token> --secret github_token
```
