package server

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ACME 续期巡检（设计见 docs/guide/acme-design.md D7/D8）：
// 模式与 drift/smart/ddns 同构（每分钟醒来对比间隔，Setting 动态可改，
// 0 = 关闭）。每轮对 AutoRenew 且已签发的证书判断临期
// （ExpiresAt - now < RenewBeforeDays），临期重签；失败置 failed +
// 告警（真去重），并按 LastRenewAt 做 1h 失败节流防触 CA 限频。

const ACMEIntervalSettingKey = "acme.scan_interval_seconds"

const (
	acmeMinIntervalSeconds = 300
	acmeMaxIntervalSeconds = 86400
	acmeDefaultInterval    = 3600
)

// acmeRenewRetryAfter 签发失败后的重试节流（D8：LE 失败验证限频
// 5 次/小时/账户/主机名，1h 节流保证巡检连续失败不会撞上去）
const acmeRenewRetryAfter = time.Hour

var (
	acmeScanTick      = time.Minute      // 醒来对比间隔的节奏
	acmeScanStartWait = 30 * time.Second // 启动后先等一轮再首扫
)

var errACMEIntervalRange = fmt.Errorf("scan_interval_seconds out of range [%d, %d]",
	acmeMinIntervalSeconds, acmeMaxIntervalSeconds)

// GetAcmeScanInterval 读巡检间隔；未配置/非法用默认值（0 合法 = 关闭）
func (s *Server) GetAcmeScanInterval() int {
	v, err := s.db.GetSetting(ACMEIntervalSettingKey)
	if err != nil || v == "" {
		return acmeDefaultInterval
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || (n != 0 && (n < acmeMinIntervalSeconds || n > acmeMaxIntervalSeconds)) {
		return acmeDefaultInterval
	}
	return n
}

// SetAcmeScanInterval 校验并写入巡检间隔（0 = 关闭，合法）
func (s *Server) SetAcmeScanInterval(seconds int) error {
	if seconds != 0 && (seconds < acmeMinIntervalSeconds || seconds > acmeMaxIntervalSeconds) {
		return errACMEIntervalRange
	}
	return s.db.SetSetting(ACMEIntervalSettingKey, strconv.Itoa(seconds))
}

// acmeScanLoop 定时续期循环（每分钟醒来对比间隔，间隔可动态改）
func (s *Server) acmeScanLoop() {
	time.Sleep(acmeScanStartWait)
	ticker := time.NewTicker(acmeScanTick)
	defer ticker.Stop()

	var lastScan time.Time
	for {
		select {
		case <-ticker.C:
			interval := s.GetAcmeScanInterval()
			if interval <= 0 {
				continue // 关闭巡检
			}
			if !lastScan.IsZero() && time.Since(lastScan) < time.Duration(interval)*time.Second {
				continue
			}
			lastScan = time.Now()
			s.scanACMEOnce()
		case <-s.ctx.Done():
			return
		}
	}
}

// scanACMEOnce 扫一轮：临期的 issued 证书续签；failed 证书节流后重试
// （否则一次失败证书就永远卡死；1h 节流防撞 CA 失败限频，D8）。
// pending 需用户手动首签（创建 ≠ 立即签发的语义预期）。
func (s *Server) scanACMEOnce() {
	certs, err := s.db.ListAcmeCerts()
	if err != nil || len(certs) == 0 {
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	now := time.Now()
	renewed := 0
	for _, cert := range certs {
		if !cert.AutoRenew {
			continue
		}
		switch cert.Status {
		case "issued":
			// 未临期跳过（剩余有效期 ≥ RenewBeforeDays）
			if time.Until(cert.ExpiresAt) >= time.Duration(cert.RenewBeforeDays)*24*time.Hour {
				continue
			}
		case "failed":
			// 失败节流（D8）：上次签发尝试距今不足 1h 跳过——
			// 手动 issue 不节流，用户明确意图优先
			if cert.LastRenewAt > 0 && now.Sub(time.Unix(cert.LastRenewAt, 0)) < acmeRenewRetryAfter {
				continue
			}
		default:
			continue
		}
		if _, err := s.runACMEIssue(cert, generator, now); err == nil {
			renewed++
		}
	}
	if renewed > 0 {
		log.Printf("ACME scan completed: %d certificates renewed", renewed)
	}
}

// runACMEIssue 单条签发/重签全流程：调 issuer → 回写状态与产物 →
// 联动观测表（D6）→ 失败告警（真去重）。issue 端点与巡检共用。
func (s *Server) runACMEIssue(cert *storage.AcmeCert, generator *alert.Generator, now time.Time) (*IssuedResult, error) {
	if s.acme == nil {
		return nil, errors.New("acme issuer not available")
	}
	res, err := s.acme.Issue(cert)
	cert.LastRenewAt = now.Unix()
	cert.CheckedAt = now.Unix()
	if err != nil {
		cert.Status = "failed"
		cert.LastStatus = "failed"
		cert.LastError = err.Error()
		_ = s.db.UpdateAcmeCert(cert)
		if generator != nil {
			generator.CheckACME(cert.PrimaryDomain, err.Error())
		}
		return nil, err
	}
	cert.Status = "issued"
	cert.LastStatus = "ok"
	cert.LastError = ""
	cert.CertificatePEM = res.CertificatePEM
	cert.IssuerPEM = res.IssuerPEM
	cert.PrivateKeyPEM = res.PrivateKeyPEM
	cert.ExpiresAt = res.ExpiresAt
	if err := s.db.UpdateAcmeCert(cert); err != nil {
		return res, err
	}
	s.upsertAcmeCertObservation(cert)
	s.maybeDeployAcmeCert(cert) // 绑定了部署目标则推送（D14，失败不回滚签发状态）
	return res, nil
}

// upsertAcmeCertObservation 签发成功后联动证书观测表（D6）：probe 的
// TLS 探测对该域名继续独立观测，两层互补；观测记录用确定性 ID
// acme-{配置ID}（幂等），Labels["source"]="acme" 标记来源。
// 失败只记日志（观测联动不是签发的关键路径）。
func (s *Server) upsertAcmeCertObservation(cert *storage.AcmeCert) {
	id := fmt.Sprintf("acme-%d", cert.ID)
	existing, err := s.db.GetCertificate(id)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			log.Printf("ACME observation lookup failed for %s: %v", cert.PrimaryDomain, err)
			return
		}
		existing = &storage.Certificate{
			ID:         id,
			DomainName: cert.PrimaryDomain,
			Labels:     map[string]string{"source": "acme"},
		}
	}
	existing.Issuer = "Let's Encrypt (" + cert.CADirectory + ")"
	existing.Status = "valid"
	existing.ExpiresAt = cert.ExpiresAt
	existing.AutoRenew = cert.AutoRenew
	existing.RenewBeforeDays = cert.RenewBeforeDays
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels["source"] = "acme"
	if err := s.db.UpsertCertificate(existing); err != nil {
		log.Printf("ACME observation upsert failed for %s: %v", cert.PrimaryDomain, err)
	}
}
