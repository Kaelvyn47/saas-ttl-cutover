#!/bin/sh
set -eu
: "${TENANT_DOMAIN:?set TENANT_DOMAIN to a Cloudflare-backed domain you control}"

curl --fail-with-body -X POST http://localhost:8080/tenants/onboard \
  -H 'Content-Type: application/json' \
  -d "{\"account_id\":\"acct-42\",\"domain\":\"$TENANT_DOMAIN\",\"name\":\"app\",\"target\":\"blue.edge.example\",\"request_id\":\"onboard-acct-42\"}"

curl --fail-with-body -X POST http://localhost:8080/admin/cutover \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"acct-42","target":"green.edge.example","request_id":"cutover-acct-42-green"}'

curl --fail-with-body -X POST http://localhost:8080/admin/rollback \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"acct-42","request_id":"rollback-acct-42"}'
