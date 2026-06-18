package inventory

import (
	"context"
	"fmt"
	"log"

	"github.com/cuihairu/cockpit/internal/storage"
)

// Syncer syncs inventory to database
type Syncer struct {
	db *storage.DB
}

// NewSyncer creates a syncer
func NewSyncer(db *storage.DB) *Syncer {
	return &Syncer{db: db}
}

// Sync syncs inventory to database
func (s *Syncer) Sync(ctx context.Context, inv *Inventory) (*SyncResult, error) {
	result := &SyncResult{}

	// Sync agents
	agents, err := s.syncAgents(inv)
	if err != nil {
		return nil, fmt.Errorf("sync agents: %w", err)
	}
	result.Agents = agents

	// Sync domains
	domains, err := s.syncDomains(inv)
	if err != nil {
		return nil, fmt.Errorf("sync domains: %w", err)
	}
	result.Domains = domains

	// Sync certificates
	certs, err := s.syncCertificates(inv)
	if err != nil {
		return nil, fmt.Errorf("sync certificates: %w", err)
	}
	result.Certificates = certs

	// Sync compute instances
	compute, err := s.syncComputeInstances(inv)
	if err != nil {
		return nil, fmt.Errorf("sync compute instances: %w", err)
	}
	result.ComputeInstances = compute

	// Sync services
	services, err := s.syncServices(inv)
	if err != nil {
		return nil, fmt.Errorf("sync services: %w", err)
	}
	result.Services = services

	// Sync gateways
	gateways, err := s.syncGateways(inv)
	if err != nil {
		return nil, fmt.Errorf("sync gateways: %w", err)
	}
	result.Gateways = gateways

	// Sync storages
	storages, err := s.syncStorages(inv)
	if err != nil {
		return nil, fmt.Errorf("sync storages: %w", err)
	}
	result.Storages = storages

	log.Printf("Sync completed: agents=%d domains=%d certificates=%d compute=%d services=%d gateways=%d storages=%d",
		result.Agents.Created+result.Agents.Updated,
		result.Domains.Created+result.Domains.Updated,
		result.Certificates.Created+result.Certificates.Updated,
		result.ComputeInstances.Created+result.ComputeInstances.Updated,
		result.Services.Created+result.Services.Updated,
		result.Gateways.Created+result.Gateways.Updated,
		result.Storages.Created+result.Storages.Updated)

	return result, nil
}

func (s *Syncer) syncAgents(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}
	agents := inv.GetAgents()

	for id, agentLoc := range agents {
		storageAgent := &storage.Agent{
			ID:       id,
			Hostname: agentLoc.Hostname,
			IP:       agentLoc.IP,
			Region:   agentLoc.Region,
			Zone:     agentLoc.Zone,
			Status:   "offline",
		}

		caps := agentLoc.Capabilities
		if len(caps) == 0 {
			// 当 inventory 未显式声明 capabilities 时，从 config 键推断
			caps = detectCapabilities(agentLoc.Config)
		}
		for _, cap := range caps {
			storageAgent.Capabilities = append(storageAgent.Capabilities, storage.Capability{
				Type:    cap,
				Version: "",
				Config:  agentLoc.Config,
			})
		}

		// 先查存在性以准确区分 Created/Updated（避免依赖 BeforeCreate hook 时间戳的脆弱判断）
		_, getErr := s.db.GetAgent(id)

		if err := s.db.UpsertAgent(storageAgent); err != nil {
			log.Printf("Failed to upsert agent %s: %v", id, err)
			result.Errors++
			continue
		}

		if getErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
}

// detectCapabilities 从 agent.Config 的键推断 capability 类型
// 约定：config 含 "pve"/"docker"/"openwrt" 键时分别推断对应 capability
func detectCapabilities(config map[string]any) []string {
	if len(config) == 0 {
		return nil
	}
	var caps []string
	for _, key := range []string{"pve", "docker", "openwrt"} {
		if _, ok := config[key]; ok {
			caps = append(caps, key)
		}
	}
	return caps
}

func (s *Syncer) syncDomains(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}

	for id, domain := range inv.Domains {
		if domain == nil {
			continue
		}

		storageDomain := &storage.Domain{
			ID:        id,
			Domain:    domain.Domain,
			Provider:  domain.Provider,
			AutoRenew: domain.AutoRenew,
		}

		if domain.Agent != "" {
			storageDomain.AgentID = &domain.Agent
		}

		_, getErr := s.db.GetDomain(id)

		if err := s.db.UpsertDomain(storageDomain); err != nil {
			log.Printf("Failed to upsert domain %s: %v", id, err)
			result.Errors++
			continue
		}
		if getErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
}

func (s *Syncer) syncCertificates(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}

	for _, cert := range inv.GetCertificates() {
		if cert == nil {
			continue
		}

		var domainID *string
		for domainIDValue, domain := range inv.Domains {
			if domain.Domain == cert.Domain {
				domainID = &domainIDValue
				break
			}
		}

		storageCert := &storage.Certificate{
			ID:              cert.ID,
			DomainID:        domainID,
			DomainName:      cert.Domain,
			Issuer:          cert.Provider,
			AutoRenew:       cert.AutoRenew,
			RenewBeforeDays: cert.RenewBeforeDays,
		}

		if cert.Agent != "" {
			storageCert.AgentID = &cert.Agent
		}

		_, getErr := s.db.GetCertificate(cert.ID)

		if err := s.db.UpsertCertificate(storageCert); err != nil {
			log.Printf("Failed to upsert certificate %s: %v", cert.ID, err)
			result.Errors++
			continue
		}
		if getErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
}

func (s *Syncer) syncComputeInstances(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}

	for id, inst := range inv.ComputeInstances {
		if inst == nil {
			continue
		}

		storageInst := &storage.ComputeInstance{
			ID:       id,
			Name:     inst.Name,
			Type:     inst.Type,
			AgentID:  inst.Agent,
			Region:   inst.Region,
			Zone:     inst.Zone,
			CPUCores: inst.CPU,
			MemoryMB: inst.Memory,
			DiskGB:   inst.Disk,
			IPv4:     inst.IPv4,
			IPv6:     inst.IPv6,
			Labels:   inst.Labels,
		}

		_, getErr := s.db.GetComputeInstance(id)

		if err := s.db.UpsertComputeInstance(storageInst); err != nil {
			log.Printf("Failed to upsert compute instance %s: %v", id, err)
			result.Errors++
			continue
		}
		if getErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
}

func (s *Syncer) syncServices(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}

	for id, svc := range inv.Services {
		if svc == nil {
			continue
		}

		var agentID *string
		if svc.Agent != "" {
			agentID = &svc.Agent
		}

		storageSvc := &storage.Service{
			ID:      id,
			Name:    svc.Name,
			Type:    svc.Type,
			AgentID: agentID,
			URL:     svc.URL,
			Labels:  svc.Labels,
		}

		// Add region/zone to labels if specified
		if storageSvc.Labels == nil {
			storageSvc.Labels = make(map[string]string)
		}
		if svc.Region != "" {
			storageSvc.Labels["region"] = svc.Region
		}
		if svc.Zone != "" {
			storageSvc.Labels["zone"] = svc.Zone
		}

		_, getErr := s.db.GetService(id)

		if err := s.db.UpsertService(storageSvc); err != nil {
			log.Printf("Failed to upsert service %s: %v", id, err)
			result.Errors++
			continue
		}
		if getErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
}

func (s *Syncer) syncGateways(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}

	for id, gw := range inv.Gateways {
		if gw == nil {
			continue
		}

		storageGw := &storage.Gateway{
			ID:       id,
			Name:     gw.Name,
			Type:     gw.Type,
			AgentID:  gw.Agent,
			IPv4:     gw.IPv4,
			IPv6:     gw.IPv6,
			Upstream: gw.Upstream,
			Labels:   gw.Labels,
		}

		// Add region/zone to labels if specified
		if storageGw.Labels == nil {
			storageGw.Labels = make(map[string]string)
		}
		if gw.Region != "" {
			storageGw.Labels["region"] = gw.Region
		}
		if gw.Zone != "" {
			storageGw.Labels["zone"] = gw.Zone
		}

		_, getErr := s.db.GetGateway(id)

		if err := s.db.UpsertGateway(storageGw); err != nil {
			log.Printf("Failed to upsert gateway %s: %v", id, err)
			result.Errors++
			continue
		}
		if getErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
}

func (s *Syncer) syncStorages(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}

	for id, st := range inv.Storages {
		if st == nil {
			continue
		}

		storageSt := &storage.Storage{
			ID:      id,
			Name:    st.Name,
			Type:    st.Type,
			AgentID: st.Agent,
			Path:    st.Path,
			Labels:  st.Labels,
		}

		// Add region/zone to labels if specified
		if storageSt.Labels == nil {
			storageSt.Labels = make(map[string]string)
		}
		if st.Region != "" {
			storageSt.Labels["region"] = st.Region
		}
		if st.Zone != "" {
			storageSt.Labels["zone"] = st.Zone
		}

		_, getErr := s.db.GetStorage(id)

		if err := s.db.UpsertStorage(storageSt); err != nil {
			log.Printf("Failed to upsert storage %s: %v", id, err)
			result.Errors++
			continue
		}
		if getErr == nil {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
}

// SyncResult sync result
type SyncResult struct {
	Agents           *ResourceResult
	Domains          *ResourceResult
	Certificates     *ResourceResult
	ComputeInstances *ResourceResult
	Services         *ResourceResult
	Gateways         *ResourceResult
	Storages         *ResourceResult
}

// ResourceResult resource sync result
type ResourceResult struct {
	Created int
	Updated int
	Deleted int
	Errors  int
}
