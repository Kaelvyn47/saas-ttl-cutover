package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	cutover "example.com/saas-ttl-cutover"
)

type request struct {
	AccountID string `json:"account_id"`
	Domain    string `json:"domain"`
	Name      string `json:"name"`
	Target    string `json:"target"`
	RequestID string `json:"request_id"`
	Active    *bool  `json:"active"`
}

type server struct{ manager *cutover.Manager }

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := &cutover.Client{BaseURL: cutover.DefaultBaseURL, APIKey: key, MaxRetries: 3}
	s := &server{manager: cutover.NewManager(client)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tenants/onboard", s.onboard)
	mux.HandleFunc("PATCH /accounts/lifecycle", s.lifecycle)
	mux.HandleFunc("POST /admin/cutover", s.cutover)
	mux.HandleFunc("POST /admin/rollback", s.rollback)
	log.Printf("tenant cutover service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (s *server) onboard(w http.ResponseWriter, r *http.Request) {
	var in request
	if !decode(w, r, &in) {
		return
	}
	tenant, err := s.manager.Onboard(r.Context(), in.AccountID, in.Domain, in.Name, in.Target, in.RequestID)
	respond(w, tenant, err)
}

func (s *server) lifecycle(w http.ResponseWriter, r *http.Request) {
	var in request
	if !decode(w, r, &in) {
		return
	}
	if in.Active == nil {
		http.Error(w, "active is required", http.StatusBadRequest)
		return
	}
	tenant, err := s.manager.SetActive(in.AccountID, *in.Active)
	respond(w, tenant, err)
}

func (s *server) cutover(w http.ResponseWriter, r *http.Request) {
	var in request
	if !decode(w, r, &in) {
		return
	}
	tenant, err := s.manager.Cutover(r.Context(), in.AccountID, in.Target, in.RequestID)
	respond(w, tenant, err)
}

func (s *server) rollback(w http.ResponseWriter, r *http.Request) {
	var in request
	if !decode(w, r, &in) {
		return
	}
	tenant, err := s.manager.Rollback(r.Context(), in.AccountID, in.RequestID)
	respond(w, tenant, err)
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, tenant cutover.Tenant, err error) {
	if err != nil {
		status := http.StatusConflict
		var apiErr *cutover.APIError
		if errors.As(err, &apiErr) {
			status = cutover.ClientStatus(err)
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenant)
}
