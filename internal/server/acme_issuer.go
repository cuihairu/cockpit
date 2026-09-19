package server

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/dnspod"
	"github.com/go-acme/lego/v4/registration"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ACME 证书签发（设计见 docs/guide/acme-design.md）。
//
// 签发抽成 AcmeIssuer 接口：生产实现包 lego（DNS-01 + 多 DNS provider，
// D13：Cloudflare/DNSPod/阿里云），测试注入 fake——完整 ACME 流程需要
// 真实 CA，单测不跑真流程。
//
//	POST /api/acme/certs/{id}/issue → Issue(cert)（同步）
//	巡检续期（acme_scan.go）→ Issue(cert)

// CA 目录两档（D4）：默认 staging（假证书不触生产限频），确认后切 production
const (
	AcmeCADirectoryStaging    = "staging"
	AcmeCADirectoryProduction = "production"
)

// 签发链路注入点：lego 客户端方法与 crypto 原语在生产路径恒成功（库语义
// 保证），错误分支仅供测试覆盖。测试注入后务必 defer 恢复。
var (
	// generateAccountKeyPEMFn 账户私钥生成（测试注入失败覆盖 ensureAccount 防御）
	generateAccountKeyPEMFn = generateAccountKeyPEM
	// legoNewClient lego 客户端构造（配置恒合法，错误分支不可达）
	legoNewClient = lego.NewClient
	// setDNS01Provider DNS-01 provider 挂载（lego v4 恒返回 nil，包装后可注入）
	setDNS01Provider = func(c *lego.Client, p challenge.Provider) error {
		return c.Challenge.SetDNS01Provider(p)
	}
	// obtainCertificate 证书签发（测试注入假 Resource 覆盖落库路径）
	obtainCertificate = func(c *lego.Client, req certificate.ObtainRequest) (*certificate.Resource, error) {
		return c.Certificate.Obtain(req)
	}
	// pemLeafNotAfterFn 叶子证书解析（测试注入固定时间）
	pemLeafNotAfterFn = pemLeafNotAfter
	// cryptoRandReader 账户密钥熵源（测试注入失败 reader，锁定 GenerateKey
	// 对坏 reader 也成功的行为）
	cryptoRandReader = rand.Reader
)

var acmeDirectoryURLs = map[string]string{
	AcmeCADirectoryStaging:    "https://acme-staging-v02.api.letsencrypt.org/directory",
	AcmeCADirectoryProduction: "https://acme-v02.api.letsencrypt.org/directory",
}

// AcmeDNSConfig DNS provider 配置快照（D13）：provider 决定 ACME DNS-01
// 用哪家 API，与 DNS 管理/DDNS 的 Cloudflare 专属判定语义分叉
type AcmeDNSConfig struct {
	Provider        string // 空 = cloudflare（向后兼容）/ dnspod / alidns
	CloudflareToken string
	DNSPodToken     string // "ID,Token" 合并格式
	AliAccessKey    string
	AliSecretKey    string
}

// acmeDNSProviders ACME 支持的 provider 名单（API 层校验与文档对齐）
var acmeDNSProviders = []string{"cloudflare", "dnspod", "alidns"}

// newAcmeDNSConfig 从全局配置快照 ACME DNS provider 配置（D13）。
// Start 用它构造延迟取值闭包；测试直构 Server{} 未走 Start 时
// Server.acmeDNS() 也用它兜底，两路语义一致。
func newAcmeDNSConfig(cfg *config.Config) AcmeDNSConfig {
	if cfg == nil || cfg.DNS == nil {
		return AcmeDNSConfig{}
	}
	out := AcmeDNSConfig{Provider: cfg.DNS.Provider}
	if cfg.DNS.Cloudflare != nil {
		out.CloudflareToken = cfg.DNS.Cloudflare.APIToken
	}
	if cfg.DNS.DNSPod != nil {
		out.DNSPodToken = cfg.DNS.DNSPod.LoginToken
	}
	if cfg.DNS.AliDNS != nil {
		out.AliAccessKey = cfg.DNS.AliDNS.AccessKey
		out.AliSecretKey = cfg.DNS.AliDNS.SecretKey
	}
	return out
}

// Ready 返回凭据是否就绪；未就绪返回面向用户的缺失键提示（D13）
func (c AcmeDNSConfig) Ready() (bool, string) {
	provider := c.Provider
	if provider == "" {
		provider = "cloudflare"
	}
	switch provider {
	case "cloudflare":
		if c.CloudflareToken == "" {
			return false, "Cloudflare token not configured: set dns.cloudflare.api_token in config.yaml or CLOUDFLARE_API_TOKEN env"
		}
	case "dnspod":
		if c.DNSPodToken == "" {
			return false, "DNSPod token not configured: set dns.dnspod.login_token in config.yaml or DNSPOD_LOGIN_TOKEN env"
		}
	case "alidns":
		if c.AliAccessKey == "" || c.AliSecretKey == "" {
			return false, "AliDNS credentials not configured: set dns.alidns.access_key/secret_key in config.yaml or ALIYUN_ACCESS_KEY/ALIYUN_ACCESS_KEY_SECRET env"
		}
	default:
		return false, fmt.Sprintf("unknown ACME DNS provider: %s (supported: cloudflare dnspod alidns)", provider)
	}
	return true, ""
}

// dnsProvider 按 D13 工厂构造 lego DNS provider。provider 合法性与凭据
// 校验统一前置到 Ready()（单一真源），未知 provider 到达不了 switch；
// 若未来新增 provider 只改 Ready 漏改此处，nil factory fail-fast 即可暴露。
func dnsProvider(cfg AcmeDNSConfig) (challenge.Provider, error) {
	if ok, msg := cfg.Ready(); !ok {
		return nil, errors.New(msg)
	}
	var factory func(AcmeDNSConfig) (challenge.Provider, error)
	switch cfg.Provider {
	case "", "cloudflare":
		factory = func(c AcmeDNSConfig) (challenge.Provider, error) {
			cfCfg := cloudflare.NewDefaultConfig()
			cfCfg.AuthToken = c.CloudflareToken
			return cloudflare.NewDNSProviderConfig(cfCfg)
		}
	case "dnspod":
		factory = func(c AcmeDNSConfig) (challenge.Provider, error) {
			dpCfg := dnspod.NewDefaultConfig()
			dpCfg.LoginToken = c.DNSPodToken
			return dnspod.NewDNSProviderConfig(dpCfg)
		}
	case "alidns":
		factory = func(c AcmeDNSConfig) (challenge.Provider, error) {
			aliCfg := alidns.NewDefaultConfig()
			aliCfg.APIKey = c.AliAccessKey
			aliCfg.SecretKey = c.AliSecretKey
			return alidns.NewDNSProviderConfig(aliCfg)
		}
	}
	return factory(cfg)
}

// acmeDNS 快照当前 ACME DNS provider 配置。acmeDNSConfig 闭包由 Start
// 注入；测试直构 Server{} 未走 Start 时从 cfg 即时快照兜底（语义一致）。
func (s *Server) acmeDNS() AcmeDNSConfig {
	if s.acmeDNSConfig != nil {
		return s.acmeDNSConfig()
	}
	return newAcmeDNSConfig(s.cfg)
}

// IssuedResult 一次签发的产物
type IssuedResult struct {
	CertificatePEM string
	IssuerPEM      string
	PrivateKeyPEM  string
	ExpiresAt      time.Time
}

// AcmeIssuer 签发接口
type AcmeIssuer interface {
	Issue(cert *storage.AcmeCert) (*IssuedResult, error)
}

// NewLegoIssuer 生产实现；dnsConfig 延迟取（config 加载后固定，测试可注入）
func NewLegoIssuer(db *storage.DB, dnsConfig func() AcmeDNSConfig) AcmeIssuer {
	return &legoIssuer{db: db, dnsConfig: dnsConfig}
}

type legoIssuer struct {
	db        *storage.DB
	dnsConfig func() AcmeDNSConfig
}

// legoUser 实现 lego registration.User
type legoUser struct {
	email string
	reg   *registration.Resource
	key   crypto.PrivateKey
}

func (u *legoUser) GetEmail() string                        { return u.email }
func (u *legoUser) GetRegistration() *registration.Resource { return u.reg }
func (u *legoUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// ensureAccount 惰性注册账户：无账户→生成 key 注册；已有账户（任意目录）→
// 同一把 key 再 Register——ACME 按 key 幂等，同 key 不产生新账户
// （账户 key 跨重启持久化，重复换 key 才会触发账户数限频，D5）。
func (l *legoIssuer) ensureAccount(caDir, dirURL string) (*legoUser, error) {
	acc, err := l.db.GetAcmeAccount()
	switch {
	case errors.Is(err, storage.ErrAcmeAccountNotFound):
		pemStr, genErr := generateAccountKeyPEMFn()
		if genErr != nil {
			return nil, fmt.Errorf("generate account key: %w", genErr)
		}
		acc = &storage.AcmeAccount{PrivateKeyPEM: pemStr}
	case err != nil:
		return nil, err
	}

	key, err := loadAccountKey(acc.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load account key: %w", err)
	}
	user := &legoUser{email: acc.Email, key: key}

	cfg := lego.NewConfig(user)
	cfg.CADirURL = dirURL
	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("lego client: %w", err)
	}
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("acme register: %w", err)
	}
	user.reg = reg

	// 持久化（目录切换后 RegistrationURI 随最近一次注册覆盖——同 key 可在任何目录重注册）
	acc.RegistrationURI = reg.URI
	acc.CADirectory = caDir
	if err := l.db.SaveAcmeAccount(acc); err != nil {
		return nil, err
	}
	return user, nil
}

func (l *legoIssuer) Issue(cert *storage.AcmeCert) (*IssuedResult, error) {
	dirURL, ok := acmeDirectoryURLs[cert.CADirectory]
	if !ok {
		return nil, fmt.Errorf("unknown CA directory: %s", cert.CADirectory)
	}
	dnsCfg := l.dnsConfig()

	user, err := l.ensureAccount(cert.CADirectory, dirURL)
	if err != nil {
		return nil, err
	}

	cfg := lego.NewConfig(user)
	cfg.CADirURL = dirURL
	cfg.Certificate.KeyType = certcrypto.EC256
	client, err := legoNewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("lego client: %w", err)
	}

	// DNS-01 provider 按 D13 工厂构造（凭据缺失/未知 provider 各报各的键）；
	// 默认顺序传播策略查 authoritative NS，不盲等固定时长（D8）
	provider, err := dnsProvider(dnsCfg)
	if err != nil {
		return nil, err
	}
	if err := setDNS01Provider(client, provider); err != nil {
		return nil, fmt.Errorf("set dns01 provider: %w", err)
	}

	// Bundle=false：叶子与中间证书分开取，两段 PEM 分别落库（D6）
	res, err := obtainCertificate(client, certificate.ObtainRequest{
		Domains: cert.Domains,
		Bundle:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("acme obtain: %w", err)
	}

	expires, err := pemLeafNotAfterFn(res.Certificate)
	if err != nil {
		return nil, fmt.Errorf("parse issued certificate: %w", err)
	}
	return &IssuedResult{
		CertificatePEM: string(res.Certificate),
		IssuerPEM:      string(res.IssuerCertificate),
		PrivateKeyPEM:  string(res.PrivateKey),
		ExpiresAt:      expires,
	}, nil
}

// generateAccountKeyPEM 生成 ECDSA P-256 账户私钥并编 PEM
func generateAccountKeyPEM() (string, error) {
	// Go 1.26：ecdsa.GenerateKey 对失败的熵源 reader 也返回成功（行为锁定见
	// TestCovFinalAcmeCryptoRand），错误分支在该版本下不可达
	key, _ := ecdsa.GenerateKey(elliptic.P256(), cryptoRandReader)
	// 有效 P-256 私钥的 Marshal 恒成功
	der, _ := x509.MarshalECPrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})), nil
}

func loadAccountKey(pemStr string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("account key PEM not found")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

// pemLeafNotAfter 从证书 PEM（第一段 CERTIFICATE block）解叶子证书 NotAfter
func pemLeafNotAfter(certPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, errors.New("no CERTIFICATE PEM block")
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return crt.NotAfter, nil
}
