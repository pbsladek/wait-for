# waitfor workflow recipes

This document groups common `waitfor` usage by workflow instead of by backend.

## Kubernetes deployment plus HTTP health

```bash
waitfor --timeout 10m \
  k8s deployment/api --for rollout --namespace prod \
  -- http https://api.example.com/health --status 200 --jsonpath '.ready == true'
```

Use a guard to fail fast when the application logs a terminal error:

```bash
waitfor --timeout 10m \
  http https://api.example.com/health --status 200 \
  -- guard log /var/log/api.log --matches 'FATAL|panic|bind: address already in use'
```

## Artifact signature plus checksum

```bash
waitfor cosign ghcr.io/org/app:v1.2.3 \
  --certificate-identity "$IDENTITY" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  -- checksum dist/app.tar.gz --equals "sha256:$SHA256"
```

## Log readiness without leaking matched content

```bash
waitfor log /var/log/app.log --contains "server ready" --tail 200 --min-matches 1
```

Matched log line content is not copied into JSON details; use `--verbose` only
when local troubleshooting requires poll-attempt context.

## Repeatable recipe file

```yaml
timeout: 10m
mode: all
conditions:
  - name: api
    http:
      url: https://api.example.com/health
      status: 200
      jsonpath: ".ready == true"
  - name: rollout
    k8s:
      resource: deployment/api
      for: rollout
      namespace: prod
guards:
  - log:
      path: /var/log/api.log
      matches: "FATAL|panic"
```

```bash
waitfor --config waitfor.yaml
waitfor --config waitfor.yaml --explain
```

