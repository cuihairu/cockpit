package inventory

import (
	"context"
	"log"
	"strings"

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

// locationOrUnknown 资源列地域落库语义（拍板 2026-10-08，todo Phase 2.2
// region/zone 校验项）：为空允许——不拒绝同步，省略或空白统一标记
// "unknown"（语义见 docs/guide/concepts.md「Inventory 文件」与 decision-log）。
func locationOrUnknown(v string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return "unknown"
}

// applyLocationLabels 把 region/zone 写进 labels 并返回该 map（拍板同上）：
// 显式字段优先（非空白即写入）；为空时仅在用户未通过 labels 显式给出该键时
// 补 "unknown"。nil map 入参时新建返回（map 赋值不回传，须由调用方接住）。
func applyLocationLabels(labels map[string]string, region, zone string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}
	set := func(key, v string) {
		if v = strings.TrimSpace(v); v != "" {
			labels[key] = v
			return
		}
		if _, ok := labels[key]; !ok {
			labels[key] = "unknown"
		}
	}
	set("region", region)
	set("zone", zone)
	return labels
}

// Sync syncs inventory to database
func (s *Syncer) Sync(ctx context.Context, inv *Inventory) *SyncResult {
	result := &SyncResult{}

	// Sync agents
	result.Agents = s.syncAgents(inv)

	// Sync domains
	result.Domains = s.syncDomains(inv)

	// Sync certificates
	result.Certificates = s.syncCertificates(inv)

	// Sync compute instances
	result.ComputeInstances = s.syncComputeInstances(inv)

	// Sync services
	result.Services = s.syncServices(inv)

	// Sync gateways
	result.Gateways = s.syncGateways(inv)

	// Sync storages
	result.Storages = s.syncStorages(inv)

	log.Printf("Sync completed: agents=%d domains=%d certificates=%d compute=%d services=%d gateways=%d storages=%d",
		result.Agents.Created+result.Agents.Updated,
		result.Domains.Created+result.Domains.Updated,
		result.Certificates.Created+result.Certificates.Updated,
		result.ComputeInstances.Created+result.ComputeInstances.Updated,
		result.Services.Created+result.Services.Updated,
		result.Gateways.Created+result.Gateways.Updated,
		result.Storages.Created+result.Storages.Updated)

	return result
}

func (s *Syncer) syncAgents(inv *Inventory) *ResourceResult {
	result := &ResourceResult{}
	agents := inv.GetAgents()

	for id, agentLoc := range agents {
		storageAgent := &storage.Agent{
			ID:       id,
			Hostname: agentLoc.Hostname,
			IP:       agentLoc.IP,
			Region:   agentLoc.Region,
			Zone:     agentLoc.Zone,
			// Status 留空：新建时落列默认 offline；已存在（可能已被
			// websocket 注册标 online）不刷状态——UpsertAgent 只写非零字段

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

	return result
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

func (s *Syncer) syncDomains(inv *Inventory) *ResourceResult {
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

	return result
}

func (s *Syncer) syncCertificates(inv *Inventory) *ResourceResult {
	result := &ResourceResult{}

	for _, cert := range inv.GetCertificates() {

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

	return result
}

func (s *Syncer) syncComputeInstances(inv *Inventory) *ResourceResult {
	result := &ResourceResult{}

	for id, inst := range inv.ComputeInstances {
		if inst == nil {
			continue
		}

		// 状态联动：关联 agent 在线则 running，否则 stopped
		status := "stopped"
		if inst.Agent != "" {
			if agent, err := s.db.GetAgent(inst.Agent); err == nil && agent.Status == "online" {
				status = "running"
			}
		}

		storageInst := &storage.ComputeInstance{
			ID:       id,
			Name:     inst.Name,
			Type:     inst.Type,
			AgentID:  inst.Agent,
			Region:   locationOrUnknown(inst.Region),
			Zone:     locationOrUnknown(inst.Zone),
			Status:   status,
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

	return result
}

func (s *Syncer) syncServices(inv *Inventory) *ResourceResult {
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

		// region/zone 落库语义（拍板 2026-10-08）：为空允许，标 unknown
		storageSvc.Labels = applyLocationLabels(storageSvc.Labels, svc.Region, svc.Zone)

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

	return result
}

func (s *Syncer) syncGateways(inv *Inventory) *ResourceResult {
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

		// region/zone 落库语义（拍板 2026-10-08）：为空允许，标 unknown
		storageGw.Labels = applyLocationLabels(storageGw.Labels, gw.Region, gw.Zone)

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

	return result
}

func (s *Syncer) syncStorages(inv *Inventory) *ResourceResult {
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

		// region/zone 落库语义（拍板 2026-10-08）：为空允许，标 unknown
		storageSt.Labels = applyLocationLabels(storageSt.Labels, st.Region, st.Zone)

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

	return result
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
