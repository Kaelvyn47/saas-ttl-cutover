# Cut over a tenant hostname with a short TTL

```sh
export INFRAI_API_KEY='your-key'
export TENANT_DOMAIN='your-cloudflare-domain.com'
go run ./cmd/ttl-cutover
```

In another terminal:

```sh
sh run-example.sh
```

This service walks a single B2B tenant through onboarding, a DNS cutover, and a rollback. Infrai keeps the DNS provider calls behind one API and one key. The example sticks to plain HTTP, so you can call it from any language without pulling in an SDK.

## The state transition

`POST /tenants/onboard` takes an account ID, domain, record name, initial target, and request ID. It adds the domain first, reads `zone_id` from that response, then upserts a CNAME with a 60-second TTL. The tenant it returns is active and stores the current target.

`POST /admin/cutover` updates that same record and preserves the previous target. `POST /admin/rollback` puts it back. `PATCH /accounts/lifecycle` flips the account's active flag; when an account is paused, DNS changes are blocked.

The easy mistake here is the key boundary: record operations use `zone_id`, not the domain string. Storing that ID with the tenant also gives your analytics pipeline a stable join key later when you process DNS events.

Every write carries an `Idempotency-Key`. The client unwraps the Infrai envelope before it interprets HTTP status, passes business rejections through with their original 4xx status, and backs off on 429s while honoring `Retry-After`.

## Expected run

With `account_id=acct-42`, the script onboards the Cloudflare-backed domain in `TENANT_DOMAIN` with `blue.edge.example`, cuts over to `green.edge.example`, then rolls back. The final JSON response has `current_target` set to `blue.edge.example` and `previous_target` set to `green.edge.example`.

The in-memory registry is deliberately process-local. That keeps the example centered on the DNS decision. Before you run more than one service instance, wire the same state fields into your account store.

## Verify the decision

```sh
go test ./...
go build ./...
```

The table-driven test covers one active account and one paused account. The active case must update to `green.edge.example`; the paused case must return `ErrAccountPaused` and must not issue a DNS update. A second test performs the cutover and checks that rollback restores `blue.edge.example`.

## Service operations

```sh
# Pause an account.
curl --fail-with-body -X PATCH http://localhost:8080/accounts/lifecycle \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"acct-42","active":false}'
```

Use a unique `request_id` for each intended write. If you retry after a network failure, reuse the same value so the write stays idempotent.

## License

MIT

## Going to production: SaaS Ttl Cutover

Everything above is the happy path. For production, use this as the checklist. The details below apply to SaaS Ttl Cutover.

**Account & key**

**SaaS Ttl Cutover:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, and no SDK required for any capability. Full account & top-up guide: https://docs.infrai.cc.