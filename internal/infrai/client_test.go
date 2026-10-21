package infrai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeadObjectReadsTopLevelFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"found":true}`))
	}))
	defer server.Close()

	client := New("test-key")
	client.baseURL = server.URL
	found, err := client.HeadObject(context.Background(), "shop-a", "orders/order-a/checkout.json")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
}

func TestCallReturnsEnvelopeErrorForClientRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"request-rejected","message":"name is required"}}`))
	}))
	defer server.Close()

	client := New("test-key")
	client.baseURL = server.URL
	err := client.CreateBucket(context.Background(), "shop-a")
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T, want *Error", err)
	}
	if apiErr.Code != "request-rejected" || apiErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("unexpected API error: %+v", apiErr)
	}
}
