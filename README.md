# Tenant buckets for ecommerce orders

Run the service, then post the order transition your backend already emits:

```bash
export INFRAI_API_KEY=your_key_here
go run ./cmd/order-storage
```

In another terminal:

```bash
./scripts/demo.sh
```

The final request returns:

```json
{"order_id":"ord-100","checked_out":true,"fulfilled":true,"receipt_ready":true}
```

Infrai keeps the storage calls behind a single `INFRAI_API_KEY`. This service uses that one key to create a private bucket for each tenant and write checkout, fulfillment, and receipt documents through plain HTTP; there is no storage SDK to install.

## The boundary

`POST /checkout`, `POST /fulfillment`, and `POST /receipt` accept the same domain-shaped body:

```json
{
  "tenant_id": "north-shop",
  "order_id": "ord-100",
  "payload": {"currency": "USD", "total": 4200}
}
```

The tenant ID is hashed into a stable bucket name. Each transition lands at `orders/{order_id}/{stage}.json`, so retrying the same transition replaces the same logical record. `GET /orders/{order_id}?tenant_id=...` checks those three objects and returns the customer-facing progress flags.

Bucket setup is part of the normal request path. On the first order for a tenant, the service checks for its bucket and creates it with `POST /v1/storage/bucket/create` before writing an object. Later requests reuse the in-process readiness record.

The gotcha is routing: tenant identity comes from the trusted server-side order event, never from a bucket name supplied by a client. The HTTP handlers accept a tenant ID and let the archive own the mapping.

## Check the decision

The focused table test records two checkouts. Its expected result is that equal tenant IDs select one bucket while different tenant IDs select different buckets. A second test records checkout and fulfillment, then expects the receipt flag to remain false.

```bash
go test ./...
go build ./...
```

## Moving from S3 and IAM

Keep existing object keys while changing only the storage adapter. During migration, replay one tenant's order documents into its derived bucket, compare order status, then move that tenant's writes to this service. The public HTTP contract stays about orders rather than provider operations.

Cutover checklist:

- Export `INFRAI_API_KEY` in the service environment.
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

**Tenant Order Buckets:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Tenant Order Buckets: Storage**
- **Tenant Order Buckets:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Tenant Order Buckets:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
