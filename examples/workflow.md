# Example workflow

## 1) Start broker
```bash
python3 scripts/mock-broker.py
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
scripts/human-approve.sh <request_id> 15
```

## 4) Agent accesses approved secret
```bash
scripts/secretctl.sh access --lease <lease_token> --secret github_token
```
