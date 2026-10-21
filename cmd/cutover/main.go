package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	cutover "github.com/infrai-examples/fintech-dns-cutover"
)

func main() {
	client := cutover.NewClient(os.Getenv("INFRAI_API_KEY"))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /cutovers", func(w http.ResponseWriter, r *http.Request) {
		var input cutover.Request
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		decision, err := cutover.Decide(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if input.Apply {
			if decision.Status != "approved" {
				writeJSON(w, http.StatusConflict, decision)
				return
			}
			if err := client.Apply(r.Context(), input, decision); err != nil {
				var apiErr *cutover.APIError
				if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
					writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Code, "message": apiErr.Message})
					return
				}
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
		}
		log.Printf("dns_audit domain=%s action=%s status=%s events=%d volume_minor=%d", input.Domain, decision.Action, decision.Status, decision.EventCount, decision.PaymentVolume)
		writeJSON(w, http.StatusOK, decision)
	})
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("cutover service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
