package server

import (
	"net/http"
	"strings"
)

// handleResources 处理资源请求
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request, resourcePath string) {
	if r.Method != http.MethodGet {
		s.handleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 解析资源类型和可能的 ID
	parts := strings.Split(resourcePath, "/")
	resourceType := parts[0]

	switch resourceType {
	case "compute-instances":
		if len(parts) > 1 {
			s.handleComputeInstanceGet(w, r, parts[1])
		} else {
			s.handleComputeInstancesList(w, r)
		}
	case "domains":
		if len(parts) > 1 {
			s.handleDomainGet(w, r, parts[1])
		} else {
			s.handleDomainsList(w, r)
		}
	case "certificates":
		if len(parts) > 1 {
			s.handleCertificateGet(w, r, parts[1])
		} else {
			s.handleCertificatesList(w, r)
		}
	case "services":
		if len(parts) > 1 {
			s.handleServiceGet(w, r, parts[1])
		} else {
			s.handleServicesList(w, r)
		}
	case "gateways":
		if len(parts) > 1 {
			s.handleGatewayGet(w, r, parts[1])
		} else {
			s.handleGatewaysList(w, r)
		}
	case "storages":
		if len(parts) > 1 {
			s.handleStorageGet(w, r, parts[1])
		} else {
			s.handleStoragesList(w, r)
		}
	default:
		s.handleError(w, r, http.StatusNotFound, "Resource type not found")
	}
}

// handleComputeInstancesList 计算实例列表
func (s *Server) handleComputeInstancesList(w http.ResponseWriter, r *http.Request) {
	instances, err := s.db.ListComputeInstances(nil)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list compute instances")
		return
	}
	s.writeList(w, instances, len(instances))
}

// handleComputeInstanceGet 获取单个计算实例
func (s *Server) handleComputeInstanceGet(w http.ResponseWriter, r *http.Request, id string) {
	instance, err := s.db.GetComputeInstance(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Compute instance not found")
		return
	}
	s.writeJSON(w, http.StatusOK, instance)
}

// handleDomainsList 域名列表
func (s *Server) handleDomainsList(w http.ResponseWriter, r *http.Request) {
	domains, err := s.db.ListDomains()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list domains")
		return
	}
	s.writeList(w, domains, len(domains))
}

// handleDomainGet 获取单个域名
func (s *Server) handleDomainGet(w http.ResponseWriter, r *http.Request, id string) {
	domain, err := s.db.GetDomain(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Domain not found")
		return
	}
	s.writeJSON(w, http.StatusOK, domain)
}

// handleCertificatesList 证书列表
func (s *Server) handleCertificatesList(w http.ResponseWriter, r *http.Request) {
	certificates, err := s.db.ListCertificates()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list certificates")
		return
	}
	s.writeList(w, certificates, len(certificates))
}

// handleCertificateGet 获取单个证书
func (s *Server) handleCertificateGet(w http.ResponseWriter, r *http.Request, id string) {
	cert, err := s.db.GetCertificate(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Certificate not found")
		return
	}
	s.writeJSON(w, http.StatusOK, cert)
}

// handleServicesList 服务列表
func (s *Server) handleServicesList(w http.ResponseWriter, r *http.Request) {
	services, err := s.db.ListServices()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list services")
		return
	}
	s.writeList(w, services, len(services))
}

// handleServiceGet 获取单个服务
func (s *Server) handleServiceGet(w http.ResponseWriter, r *http.Request, id string) {
	svc, err := s.db.GetService(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Service not found")
		return
	}
	s.writeJSON(w, http.StatusOK, svc)
}

// handleGatewaysList 网关列表
func (s *Server) handleGatewaysList(w http.ResponseWriter, r *http.Request) {
	gateways, err := s.db.ListGateways()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list gateways")
		return
	}
	s.writeList(w, gateways, len(gateways))
}

// handleGatewayGet 获取单个网关
func (s *Server) handleGatewayGet(w http.ResponseWriter, r *http.Request, id string) {
	gw, err := s.db.GetGateway(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Gateway not found")
		return
	}
	s.writeJSON(w, http.StatusOK, gw)
}

// handleStoragesList 存储列表
func (s *Server) handleStoragesList(w http.ResponseWriter, r *http.Request) {
	storages, err := s.db.ListStorages()
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Failed to list storages")
		return
	}
	s.writeList(w, storages, len(storages))
}

// handleStorageGet 获取单个存储
func (s *Server) handleStorageGet(w http.ResponseWriter, r *http.Request, id string) {
	st, err := s.db.GetStorage(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "Storage not found")
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}
