package inventory

import (
	"context"
	"fmt"
	"log"

	"github.com/cuihairu/cockpit/internal/storage"
)

// Syncer 同步 inventory 到数据库
type Syncer struct {
	db *storage.DB
}

// NewSyncer 创建同步器
func NewSyncer(db *storage.DB) *Syncer {
	return &Syncer{db: db}
}

// Sync 同步 inventory 到数据库
func (s *Syncer) Sync(ctx context.Context, inv *Inventory) (*SyncResult, error) {
	result := &SyncResult{}

	// 同步 Agent
	agents, err := s.syncAgents(inv)
	if err != nil {
		return nil, fmt.Errorf("sync agents: %w", err)
	}
	result.Agents = agents

	// 同步域名
	domains, err := s.syncDomains(inv)
	if err != nil {
		return nil, fmt.Errorf("sync domains: %w", err)
	}
	result.Domains = domains

	// 同步证书
	certs, err := s.syncCertificates(inv)
	if err != nil {
		return nil, fmt.Errorf("sync certificates: %w", err)
	}
	result.Certificates = certs

	log.Printf("Sync completed: agents=%d domains=%d certificates=%d",
		result.Agents.Created+result.Agents.Updated,
		result.Domains.Created+result.Domains.Updated,
		result.Certificates.Created+result.Certificates.Updated)

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
			Status:   "offline", // 初始状态，连接后更新为 online
		}

		// 转换 capabilities
		for _, cap := range agentLoc.Capabilities {
			storageAgent.Capabilities = append(storageAgent.Capabilities, storage.Capability{
				Type:    cap,
				Version: "",
				Config:  agentLoc.Config,
			})
		}

		// 使用 UpsertAgent 插入或更新
		if err := s.db.UpsertAgent(storageAgent); err != nil {
			log.Printf("Failed to upsert agent %s: %v", id, err)
			result.Errors++
			continue
		}

		// 检查是新创建还是更新
		existing, err := s.db.GetAgent(id)
		if err == nil && existing.FirstSeen.Equal(storageAgent.FirstSeen) {
			result.Updated++
		} else {
			result.Created++
		}
	}

	return result, nil
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

		// 关联 Agent
		if domain.Agent != "" {
			storageDomain.AgentID = &domain.Agent
		}

		if err := s.db.UpsertDomain(storageDomain); err != nil {
			log.Printf("Failed to upsert domain %s: %v", id, err)
			result.Errors++
			continue
		}
		result.Created++
	}

	return result, nil
}

func (s *Syncer) syncCertificates(inv *Inventory) (*ResourceResult, error) {
	result := &ResourceResult{}

	for _, cert := range inv.GetCertificates() {
		if cert == nil {
			continue
		}

		// 查找关联的域名 ID
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
			Issuer:          cert.Provider, // 使用 Provider 作为 Issuer
			AutoRenew:       cert.AutoRenew,
			RenewBeforeDays: cert.RenewBeforeDays,
		}

		if cert.Agent != "" {
			storageCert.AgentID = &cert.Agent
		}

		if err := s.db.UpsertCertificate(storageCert); err != nil {
			log.Printf("Failed to upsert certificate %s: %v", cert.ID, err)
			result.Errors++
			continue
		}
		result.Created++
	}

	return result, nil
}

// SyncResult 同步结果
type SyncResult struct {
	Agents       *ResourceResult
	Domains      *ResourceResult
	Certificates *ResourceResult
}

// ResourceResult 资源同步结果
type ResourceResult struct {
	Created int
	Updated int
	Deleted int
	Errors  int
}
