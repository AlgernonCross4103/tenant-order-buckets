package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/infrai-examples/tenant-order-buckets/internal/infrai"
	"github.com/infrai-examples/tenant-order-buckets/internal/orders"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	archive := orders.NewArchive(infrai.New(apiKey))
	mux := http.NewServeMux()
	for _, stage := range []string{"checkout", "fulfillment", "receipt"} {
		stage := stage
		mux.HandleFunc("POST /"+stage, func(w http.ResponseWriter, r *http.Request) {
			var update orders.Update
			if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
			result, err := archive.Record(r.Context(), stage, update)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, result)
		})
	}
	mux.HandleFunc("GET /orders/{order_id}", func(w http.ResponseWriter, r *http.Request) {
		status, err := archive.CustomerStatus(r.Context(), r.URL.Query().Get("tenant_id"), r.PathValue("order_id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	})

	addr := ":8080"
	log.Printf("tenant order storage listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var apiErr *infrai.Error
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		status = apiErr.HTTPStatus
	} else if strings.HasPrefix(err.Error(), "invalid ") {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
