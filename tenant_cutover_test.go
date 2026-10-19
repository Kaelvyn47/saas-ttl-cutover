package cutover

import (
	"context"
	"errors"
	"testing"
)

type fakeDNS struct {
	upsertZone string
	upsertTTL  int
	updates    []string
}

func (f *fakeDNS) AddDomain(context.Context, string, string, string) (Domain, error) {
	return Domain{ZoneID: "zone-17"}, nil
}
func (f *fakeDNS) UpsertCNAME(_ context.Context, zoneID, _, _ string, ttl int, _ string) (Record, error) {
	f.upsertZone = zoneID
	f.upsertTTL = ttl
	return Record{RecordID: "record-9"}, nil
}
func (f *fakeDNS) UpdateCNAME(_ context.Context, _, _, _, content string, ttl int, _ string) error {
	if ttl != ShortTTL {
		return errors.New("unexpected ttl")
	}
	f.updates = append(f.updates, content)
	return nil
}

func TestCutoverDecisions(t *testing.T) {
	tests := []struct {
		name       string
		paused     bool
		wantErr    error
		wantTarget string
	}{
		{name: "active account moves to new origin", wantTarget: "green.edge.example"},
		{name: "paused account cannot change DNS", paused: true, wantErr: ErrAccountPaused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dns := &fakeDNS{}
			m := NewManager(dns)
			_, err := m.Onboard(context.Background(), "acct-42", "tenant.example", "app", "blue.edge.example", "req-1")
			if err != nil {
				t.Fatal(err)
			}
			if dns.upsertZone != "zone-17" || dns.upsertTTL != ShortTTL {
				t.Fatalf("upsert boundary = zone %q, ttl %d", dns.upsertZone, dns.upsertTTL)
			}
			if tt.paused {
				_, _ = m.SetActive("acct-42", false)
			}
			got, err := m.Cutover(context.Background(), "acct-42", "green.edge.example", "req-2")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got.CurrentTarget != tt.wantTarget {
				t.Fatalf("target = %q", got.CurrentTarget)
			}
			if tt.paused && len(dns.updates) != 0 {
				t.Fatal("paused account wrote DNS")
			}
		})
	}
}

func TestRollbackRestoresPreviousTarget(t *testing.T) {
	dns := &fakeDNS{}
	m := NewManager(dns)
	_, _ = m.Onboard(context.Background(), "acct-42", "tenant.example", "app", "blue.edge.example", "req-1")
	_, _ = m.Cutover(context.Background(), "acct-42", "green.edge.example", "req-2")
	got, err := m.Rollback(context.Background(), "acct-42", "req-3")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentTarget != "blue.edge.example" {
		t.Fatalf("target = %q", got.CurrentTarget)
	}
	if len(dns.updates) != 2 || dns.updates[1] != "blue.edge.example" {
		t.Fatalf("updates = %v", dns.updates)
	}
}
