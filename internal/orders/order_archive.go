package orders

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type ObjectStore interface {
	BucketExists(context.Context, string) (bool, error)
	CreateBucket(context.Context, string) error
	PutObject(context.Context, string, string, string) error
	HeadObject(context.Context, string, string) (bool, error)
}

type Archive struct {
	store ObjectStore
	mu    sync.Mutex
	ready map[string]bool
}

type Update struct {
	TenantID string          `json:"tenant_id"`
	OrderID  string          `json:"order_id"`
	Payload  json.RawMessage `json:"payload"`
}

type Result struct {
	OrderID string `json:"order_id"`
	Stage   string `json:"stage"`
	Stored  bool   `json:"stored"`
}

type Status struct {
	OrderID    string `json:"order_id"`
	CheckedOut bool   `json:"checked_out"`
	Fulfilled  bool   `json:"fulfilled"`
	Receipt    bool   `json:"receipt_ready"`
}

func NewArchive(store ObjectStore) *Archive {
	return &Archive{store: store, ready: make(map[string]bool)}
}

func (a *Archive) Record(ctx context.Context, stage string, update Update) (Result, error) {
	if !validStage(stage) || !identifier.MatchString(update.TenantID) || !identifier.MatchString(update.OrderID) || !json.Valid(update.Payload) {
		return Result{}, fmt.Errorf("invalid order update")
	}
	bucket := BucketForTenant(update.TenantID)
	if err := a.ensureBucket(ctx, bucket); err != nil {
		return Result{}, err
	}
	document, err := json.Marshal(struct {
		OrderID    string          `json:"order_id"`
		Stage      string          `json:"stage"`
		RecordedAt string          `json:"recorded_at"`
		Payload    json.RawMessage `json:"payload"`
	}{update.OrderID, stage, time.Now().UTC().Format(time.RFC3339), update.Payload})
	if err != nil {
		return Result{}, err
	}
	key := objectKey(update.OrderID, stage)
	if err := a.store.PutObject(ctx, bucket, key, base64.StdEncoding.EncodeToString(document)); err != nil {
		return Result{}, err
	}
	return Result{OrderID: update.OrderID, Stage: stage, Stored: true}, nil
}

func (a *Archive) CustomerStatus(ctx context.Context, tenantID, orderID string) (Status, error) {
	if !identifier.MatchString(tenantID) || !identifier.MatchString(orderID) {
		return Status{}, fmt.Errorf("invalid order lookup")
	}
	bucket := BucketForTenant(tenantID)
	if err := a.ensureBucket(ctx, bucket); err != nil {
		return Status{}, err
	}
	status := Status{OrderID: orderID}
	checks := []struct {
		stage string
		set   func(bool)
	}{
		{"checkout", func(found bool) { status.CheckedOut = found }},
		{"fulfillment", func(found bool) { status.Fulfilled = found }},
		{"receipt", func(found bool) { status.Receipt = found }},
	}
	for _, check := range checks {
		found, err := a.store.HeadObject(ctx, bucket, objectKey(orderID, check.stage))
		if err != nil {
			return Status{}, err
		}
		check.set(found)
	}
	return status, nil
}

func (a *Archive) ensureBucket(ctx context.Context, bucket string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ready[bucket] {
		return nil
	}
	exists, err := a.store.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		if err := a.store.CreateBucket(ctx, bucket); err != nil {
			return err
		}
	}
	a.ready[bucket] = true
	return nil
}

func BucketForTenant(tenantID string) string {
	sum := sha256.Sum256([]byte(tenantID))
	return "shop-" + hex.EncodeToString(sum[:10])
}

func objectKey(orderID, stage string) string {
	return "orders/" + orderID + "/" + stage + ".json"
}

func validStage(stage string) bool {
	return stage == "checkout" || stage == "fulfillment" || stage == "receipt"
}
