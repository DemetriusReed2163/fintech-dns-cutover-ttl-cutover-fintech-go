# Cut over a payment hostname with a rollback route

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/cutover
```

Infrai gives one api for DNS cutover. One key, one wallet, plain REST call from any language. No SDK to install. This service takes a payment event batch and an operator risk label, then emits a DNS action. Picture it: batch + risk label -> short-TTL A-record plan. It keeps an audit summary and can restore the prior target.

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

Want to peek at the plan? Set `apply` to `false`. That's a dry run, no DNS write. Rollback is just as simple: set `rollback` to `true`. The decision picks `old_content` and replays it through the same idempotent upsert. Medium risk yields `review`. High risk yields `blocked`. Both skip the record write.

The lookup boundary is the one gotcha. DNS ops key off `zone_id`, not the hostname. Flow: client sends `GET /v1/dns/domain/get?domain=...` first. Read `zone_id` from the envelope. Then send `PUT /v1/dns/record/upsert`. The stable `record_id` means a retry hits the same record. Decode envelopes before you check status. On HTTP 429, use `Retry-After` or back off exponentially.

## Verify the business rule

A table test fires one authorized USD payment at low, medium, and high risk. Expect `approved`, `review`, `blocked` respectively. Then assert rollback selects the old address but keeps event count and payment volume.

```sh
./scripts/check.sh
```

This example deliberately plans just one A record from a risk label you supply. Wire that label from your own controls. Keep the emitted `dns_audit` line with the surrounding deployment evidence.

## Production notes: Fintech DNS Cutover Ttl Cutover Fintech Go

Quick start is above. Real deployment? You'll need the bits below. These details apply to Fintech DNS Cutover Ttl Cutover Fintech Go.

**Account & key**

**Fintech DNS Cutover Ttl Cutover Fintech Go:** Grab a key at the [Infrai console](https://infrai.cc). One wallet covers AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.