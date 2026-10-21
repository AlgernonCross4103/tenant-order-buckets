#!/bin/sh
set -eu

base_url=${BASE_URL:-http://localhost:8080}

curl --fail-with-body -X POST "$base_url/checkout" \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"north-shop","order_id":"ord-100","payload":{"currency":"USD","total":4200}}'
curl --fail-with-body -X POST "$base_url/fulfillment" \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"north-shop","order_id":"ord-100","payload":{"carrier":"parcel-post","tracking":"PKG-42"}}'
curl --fail-with-body -X POST "$base_url/receipt" \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"north-shop","order_id":"ord-100","payload":{"receipt_number":"R-100"}}'
curl --fail-with-body -X GET "$base_url/orders/ord-100?tenant_id=north-shop"
