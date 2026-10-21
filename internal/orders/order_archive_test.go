package orders

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
)

type putCall struct {
	bucket, key, data string
}

type memoryStore struct {
	buckets map[string]bool
	objects map[string]bool
	puts    []putCall
}

func newMemoryStore() *memoryStore {
	return &memoryStore{buckets: make(map[string]bool), objects: make(map[string]bool)}
}

func (m *memoryStore) BucketExists(_ context.Context, bucket string) (bool, error) {
	return m.buckets[bucket], nil
}

func (m *memoryStore) CreateBucket(_ context.Context, bucket string) error {
	m.buckets[bucket] = true
	return nil
}

func (m *memoryStore) PutObject(_ context.Context, bucket, key, data string) error {
	m.puts = append(m.puts, putCall{bucket, key, data})
	m.objects[bucket+"/"+key] = true
	return nil
}

func (m *memoryStore) HeadObject(_ context.Context, bucket, key string) (bool, error) {
	return m.objects[bucket+"/"+key], nil
}

func TestRecordRoutesEachTenantToItsOwnBucket(t *testing.T) {
	tests := []struct {
		name     string
		updates  []Update
		wantSame bool
	}{
		{
			name: "same tenant shares its bucket",
			updates: []Update{
				{TenantID: "north-shop", OrderID: "ord-100", Payload: json.RawMessage(`{"total":4200}`)},
				{TenantID: "north-shop", OrderID: "ord-101", Payload: json.RawMessage(`{"total":1800}`)},
			},
			wantSame: true,
		},
		{
			name: "different tenants use different buckets",
			updates: []Update{
				{TenantID: "north-shop", OrderID: "ord-100", Payload: json.RawMessage(`{"total":4200}`)},
				{TenantID: "south-shop", OrderID: "ord-100", Payload: json.RawMessage(`{"total":4200}`)},
			},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemoryStore()
			archive := NewArchive(store)
			for _, update := range tt.updates {
				if _, err := archive.Record(context.Background(), "checkout", update); err != nil {
					t.Fatal(err)
				}
			}
			gotSame := store.puts[0].bucket == store.puts[1].bucket
			if gotSame != tt.wantSame {
				t.Fatalf("same bucket = %v, want %v", gotSame, tt.wantSame)
			}
			if store.puts[0].key != "orders/ord-100/checkout.json" {
				t.Fatalf("unexpected object key %q", store.puts[0].key)
			}
			if _, err := base64.StdEncoding.DecodeString(store.puts[0].data); err != nil {
				t.Fatalf("stored document is not base64: %v", err)
			}
		})
	}
}

func TestCustomerStatusReflectsRecordedStages(t *testing.T) {
	store := newMemoryStore()
	archive := NewArchive(store)
	update := Update{TenantID: "north-shop", OrderID: "ord-100", Payload: json.RawMessage(`{"carrier":"parcel-post"}`)}
	if _, err := archive.Record(context.Background(), "checkout", update); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Record(context.Background(), "fulfillment", update); err != nil {
		t.Fatal(err)
	}
	status, err := archive.CustomerStatus(context.Background(), update.TenantID, update.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.CheckedOut || !status.Fulfilled || status.Receipt {
		t.Fatalf("unexpected customer status: %+v", status)
	}
}
