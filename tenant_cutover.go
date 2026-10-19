package cutover

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const ShortTTL = 60

var (
	ErrTenantExists   = errors.New("tenant already exists")
	ErrTenantNotFound = errors.New("tenant not found")
	ErrAccountPaused  = errors.New("account is paused")
	ErrNoRollback     = errors.New("no previous target to restore")
)

type DNSWriter interface {
	AddDomain(context.Context, string, string, string) (Domain, error)
	UpsertCNAME(context.Context, string, string, string, int, string) (Record, error)
	UpdateCNAME(context.Context, string, string, string, string, int, string) error
}

type Tenant struct {
	AccountID      string `json:"account_id"`
	Domain         string `json:"domain"`
	ZoneID         string `json:"zone_id"`
	RecordID       string `json:"record_id"`
	Name           string `json:"name"`
	CurrentTarget  string `json:"current_target"`
	PreviousTarget string `json:"previous_target,omitempty"`
	Active         bool   `json:"active"`
}

type Manager struct {
	dns     DNSWriter
	mu      sync.Mutex
	tenants map[string]Tenant
}

func NewManager(dns DNSWriter) *Manager {
	return &Manager{dns: dns, tenants: make(map[string]Tenant)}
}

func (m *Manager) Onboard(ctx context.Context, accountID, domain, name, target, requestID string) (Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tenants[accountID]; exists {
		return Tenant{}, ErrTenantExists
	}
	zone, err := m.dns.AddDomain(ctx, domain, accountID, requestID+"-domain")
	if err != nil {
		return Tenant{}, err
	}
	record, err := m.dns.UpsertCNAME(ctx, zone.ZoneID, name, target, ShortTTL, requestID+"-record")
	if err != nil {
		return Tenant{}, err
	}
	tenant := Tenant{AccountID: accountID, Domain: domain, ZoneID: zone.ZoneID, RecordID: record.RecordID, Name: name, CurrentTarget: target, Active: true}
	m.tenants[accountID] = tenant
	return tenant, nil
}

func (m *Manager) SetActive(accountID string, active bool) (Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tenant, ok := m.tenants[accountID]
	if !ok {
		return Tenant{}, ErrTenantNotFound
	}
	tenant.Active = active
	m.tenants[accountID] = tenant
	return tenant, nil
}

func (m *Manager) Cutover(ctx context.Context, accountID, target, requestID string) (Tenant, error) {
	return m.move(ctx, accountID, target, requestID, false)
}

func (m *Manager) Rollback(ctx context.Context, accountID, requestID string) (Tenant, error) {
	return m.move(ctx, accountID, "", requestID, true)
}

func (m *Manager) move(ctx context.Context, accountID, target, requestID string, rollback bool) (Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tenant, ok := m.tenants[accountID]
	if !ok {
		return Tenant{}, ErrTenantNotFound
	}
	if !tenant.Active {
		return Tenant{}, ErrAccountPaused
	}
	if rollback {
		if tenant.PreviousTarget == "" {
			return Tenant{}, ErrNoRollback
		}
		target = tenant.PreviousTarget
	}
	if target == "" {
		return Tenant{}, fmt.Errorf("target is required")
	}
	if err := m.dns.UpdateCNAME(ctx, tenant.ZoneID, tenant.RecordID, tenant.Name, target, ShortTTL, requestID); err != nil {
		return Tenant{}, err
	}
	old := tenant.CurrentTarget
	tenant.CurrentTarget = target
	tenant.PreviousTarget = old
	m.tenants[accountID] = tenant
	return tenant, nil
}
