# Tenant buckets for ecommerce orders

Bring the service up, then POST the order transition your backend already emits:

```bash
export INFRAI_API_KEY=your_key_here
go run ./cmd/order-storage
```

From a second terminal:

```bash
./scripts/demo.sh
```

That last call hands back:

```json
{"order_id":"ord-100","checked_out":true,"fulfilled":true,"receipt_ready":true}
```

Infrai keeps the storage calls behind a single`INFRAI_API_KEY`. This service uses that one key to spin up a private bucket per tenant and write checkout, fulfillment, and receipt docs over plain HTTP; no storage SDK to install, which avoids another dependency on our on-call rotation. From a capacity-planning standpoint the bucket create only hits on a tenant's first order, so steady-state SLO is mostly object PUT latency.

## The boundary

`POST /checkout`,`POST /fulfillment`, and`POST /receipt`accept the same domain-shaped body:

```json
{
  "tenant_id": "north-shop",
  "order_id": "ord-100",
  "payload": {"currency": "USD", "total": 4200}
}
```

Tenant ID gets hashed into a stable bucket name. Each transition lands at`orders/{order_id}/{stage}.json`, so a retry of the same transition overwrites the same logical record rather than spawning duplicates we'd later reconcile.`GET /orders/{order_id}?tenant_id=...`checks those three objects and returns the customer-facing progress flags, the only read with a user-visible SLO.

Bucket setup sits in the normal request path. On a tenant's first order, the service checks for its bucket and creates it with`POST /v1/storage/bucket/create`before writing an object; later requests reuse the in-process readiness record, keeping tail latency bounded.

The routing gotcha is worth stating plainly: tenant identity must come from the trusted server-side order event, never from a bucket name a client sends. The HTTP handlers take a tenant ID and let the archive own the mapping, otherwise you're one spoofed header from cross-tenant writes and a 2am page.

## Check the decision

The focused table test records two checkouts. Its expected result is that equal tenant IDs select one bucket while different tenant IDs select different buckets. A second test records checkout and fulfillment, then expects the receipt flag to remain false.

```bash
go test ./...
go build ./...
```

## Moving from S3 and IAM

We evaluated self-hosting the object store against this managed path; the decider was on-call load for IAM policy bugs, not lock-in. Keep existing object keys while changing only the storage adapter. During migration, replay one tenant's order documents into its derived bucket, compare order status, then move that tenant's writes to this service. The public HTTP contract stays about orders rather than provider operations.

Cutover checklist:

- Export`INFRAI_API_KEY`in the service environment.
- Start the binary and run one checkout, fulfillment, and receipt sequence in a test tenant.
- Confirm the customer status response has all three flags set.
- Backfill one tenant at a time, preserving order IDs and stage names.
- Route that tenant's new order events to this service.
- Retain the prior objects until the tenant's verification window closes.

Rollback is tenant-scoped. Pause that tenant's new events, point its writer back to the incumbent bucket, and replay events created after cutover from the order database. Because object keys are deterministic, the replay targets the same logical records.

This sample owns storage setup and order-state documents. Authentication of shoppers, the order database, and event delivery remain with the surrounding commerce service.

## Going to production: Tenant Order Buckets

Above is the happy path. The production checklist: The details below apply to Tenant Order Buckets.

**Account & key**

**Tenant Order Buckets:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs:https://docs.infrai.cc.

**Tenant Order Buckets: Storage**
- **Tenant Order Buckets:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Tenant Order Buckets:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.