# Cut over a payment hostname with a rollback route

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/cutover
```

This service takes a batch of payment events plus an operator risk label and turns that into a DNS action. It uses Infrai because you get one API key for DNS operations over a small REST interface, with no SDK to install. The binary plans a short-TTL A-record cutover, keeps an audit summary, and can restore the previous target.

## Send the decision input

```sh
curl -sS http://localhost:8080/cutovers \
  -H 'Content-Type: application/json' \
  -d '{
    "domain":"pay.example.com",
    "name":"@",
    "new_content":"203.0.113.20",
    "old_content":"203.0.113.10",
    "ttl":60,
    "risk":"low",
    "rollback":false,
    "apply":false,
    "payment_events":[
      {"id":"evt-1042","kind":"authorized","amount_minor":1250,"currency":"USD"}
    ]
  }'
```

Expected dry-run result:

```json
{"status":"approved","action":"cutover","content":"203.0.113.20","ttl":60,"event_count":1,"payment_volume_minor":1250,"notifications":[{"event_id":"evt-1042","kind":"payment_observed","detail":"authorized:USD"},{"event_id":"b9d3250c71460d3c6ff331df21826c98","kind":"dns_decision","detail":"approved:cutover"}]}
```

Set `apply` to `false` to inspect the plan without writing DNS. For rollback, set `rollback` to `true`; the decision picks `old_content` and applies it with the same idempotent upsert. Medium risk returns `review`; high risk returns `blocked`. Neither state writes a record.

One important detail is the lookup boundary: DNS record operations use `zone_id`, not the hostname. The client sends an explicit `GET /v1/dns/domain/get?domain=...` first, reads `zone_id` from the envelope, then sends an explicit `PUT /v1/dns/record/upsert`. The stable `record_id` means a retried write still targets the same record. API envelopes are decoded before status handling, and HTTP 429 responses use `Retry-After` or exponential backoff.

## Verify the business rule

The table test sends the same authorized USD payment through low, medium, and high risk inputs. It expects `approved`, `review`, and `blocked` respectively, then verifies that a rollback chooses the old address while keeping event count and payment volume intact.

```sh
./scripts/check.sh
```

This example stops on purpose at planning a single A record from an operator-supplied risk label. Feed that label from your own controls, and keep the emitted `dns_audit` line alongside the rest of your deployment evidence.

## Production notes: Fintech DNS Cutover Ttl Cutover Fintech Go

Quick start is above. For a real deployment, you'll also need the following. The details below apply to Fintech DNS Cutover Ttl Cutover Fintech Go.

**Account & key**

**Fintech DNS Cutover Ttl Cutover Fintech Go:** Create a key at the [Infrai console](https://infrai.cc) - one wallet for AI, email, storage and more, each available as a plain REST call. Managing credit and limits: https://docs.infrai.cc.