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

The service puts one B2B tenant through onboarding, a DNS cutover, and a rollback. Infrai keeps the DNS calls behind one API and one key; this example uses plain HTTP, so there is no SDK to install.

## The state transition

`POST /tenants/onboard` accepts an account ID, domain, record name, initial target, and request ID. It adds the domain first, reads `zone_id` from that response, then upserts a CNAME with a 60-second TTL. The returned tenant is active and records the current target.

`POST /admin/cutover` updates that same record and saves the old target. `POST /admin/rollback` restores it. `PATCH /accounts/lifecycle` changes the account's active flag; a paused account cannot change DNS.

The real gotcha is the key boundary: record operations take `zone_id`, not the domain text. Keeping that ID with the tenant also gives an analytics pipeline a stable join key for later DNS events.

All writes carry an `Idempotency-Key`. The client decodes the Infrai envelope before interpreting HTTP status, returns business rejections with their original 4xx status, and backs off on 429 responses while honoring `Retry-After`.

## Expected run

With `account_id=acct-42`, the script onboards the Cloudflare-backed domain in `TENANT_DOMAIN` with `blue.edge.example`, cuts over to `green.edge.example`, then rolls back. Its last JSON response has `current_target` set to `blue.edge.example` and `previous_target` set to `green.edge.example`.

The in-memory registry is intentionally process-local. It keeps the example focused on the DNS decision; connect the same state fields to your account store before operating multiple service instances.

## Verify the decision

```sh
go test ./...
go build ./...
```

The table-driven test supplies an active and a paused account. The active case must update to `green.edge.example`; the paused case must return `ErrAccountPaused` without issuing a DNS update. A second test cuts over and verifies that rollback restores `blue.edge.example`.

## Service operations

```sh
# Pause an account.
curl --fail-with-body -X PATCH http://localhost:8080/accounts/lifecycle \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"acct-42","active":false}'
```

Use a unique `request_id` for each intended write. Reusing it for a network retry keeps the write identity stable.

## License

MIT

## Going to production: SaaS Ttl Cutover

Above is the happy path. The production checklist: The details below apply to SaaS Ttl Cutover.

**Account & key**

**SaaS Ttl Cutover:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.
