package dns

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/resolver"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
)

// tlsResources bundles TLS config plus optional ACME/HTTP servers.
type tlsResources struct {
	TLSConfig      *tls.Config
	ACMEHTTPServer *http.Server
	ACMERenewer    *acmeManager
}

// buildTLSResources prepares TLS for DoT using one of: manual cert, HTTP-01 autocert, or native DNS-01 ACME (Cloudflare).
func buildTLSResources(cfg *config.ServerConfig, upstreams []string, logger *logging.Logger) (*tlsResources, error) {
	if cfg == nil || !cfg.DotEnabled {
		return &tlsResources{}, nil
	}

	// Manual PEMs
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load x509 key pair: %w", err)
		}
		return &tlsResources{TLSConfig: tlsConfigFromCert(&cert)}, nil
	}

	// Native DNS-01 via Cloudflare
	if cfg.TLS.ACME.Enabled {
		acmeUpstreams := upstreams
		if len(cfg.TLS.ACME.Upstreams) > 0 {
			acmeUpstreams = cfg.TLS.ACME.Upstreams
		}
		mgr, tlsCfg, err := newACMEManager(cfg, acmeUpstreams, logger)
		if err != nil {
			return nil, err
		}
		return &tlsResources{TLSConfig: tlsCfg, ACMERenewer: mgr}, nil
	}

	// HTTP-01 autocert fallback
	if cfg.TLS.Autocert.Enabled {
		tlsCfg, acmeHTTP, err := buildAutocert(cfg, logger)
		if err != nil {
			return nil, err
		}
		return &tlsResources{TLSConfig: tlsCfg, ACMEHTTPServer: acmeHTTP}, nil
	}

	return nil, fmt.Errorf("DoT enabled but no TLS configuration provided")
}

func tlsConfigFromCert(cert *tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"dot", "h2", "http/1.1"},
	}
}

// ------------------------------------------------------------------
// HTTP-01 Autocert (existing behavior)
// ------------------------------------------------------------------

func buildAutocert(cfg *config.ServerConfig, logger *logging.Logger) (*tls.Config, *http.Server, error) {
	cacheDir := cfg.TLS.Autocert.CacheDir
	if cacheDir == "" {
		if usrCache, err := os.UserCacheDir(); err == nil {
			cacheDir = filepath.Join(usrCache, "gloryhole-autocert")
		} else {
			cacheDir = "./.cache/autocert"
		}
	}

	m := &autocertManagerWrapper{
		CacheDir: cacheDir,
		Hosts:    cfg.TLS.Autocert.Hosts,
		Email:    cfg.TLS.Autocert.Email,
	}

	acmeHTTP := &http.Server{
		Addr:    cfg.TLS.Autocert.HTTP01Address,
		Handler: m.HTTPHandler(),
	}

	tlsCfg := &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS13,
		NextProtos:     []string{"dot", "h2", "http/1.1", "acme-tls/1"},
	}

	logger.Info("Autocert enabled for DoT (HTTP-01)", "hosts", cfg.TLS.Autocert.Hosts, "cache", cacheDir)
	return tlsCfg, acmeHTTP, nil
}

// Minimal wrapper to avoid pulling in full autocert package into this file.
// We keep implementation in autocert_impl.go to keep dependencies isolated.
type autocertManagerWrapper struct {
	CacheDir string
	Hosts    []string
	Email    string
}

func (m *autocertManagerWrapper) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return getAutocertManager(m.CacheDir, m.Hosts, m.Email).GetCertificate(hello)
}

func (m *autocertManagerWrapper) HTTPHandler() http.Handler {
	return getAutocertManager(m.CacheDir, m.Hosts, m.Email).HTTPHandler(nil)
}

// ------------------------------------------------------------------
// ACME DNS-01 (Cloudflare) using lego
// ------------------------------------------------------------------

type acmeManager struct {
	cfg         *config.ServerConfig
	upstreams   []string
	logger      *logging.Logger
	certStore   atomic.Value // *tls.Certificate
	stopCh      chan struct{}
	wg          sync.WaitGroup
	renewBefore time.Duration
	cacheDir    string
	hosts       []string
	email       string
	providerTok string
	clientMu    sync.Mutex
}

func newACMEManager(cfg *config.ServerConfig, upstreams []string, logger *logging.Logger) (*acmeManager, *tls.Config, error) {
	token := os.Getenv("CF_DNS_API_TOKEN")
	if cfg.TLS.ACME.Cloudflare.APIToken != "" {
		token = cfg.TLS.ACME.Cloudflare.APIToken
	}
	if token == "" {
		return nil, nil, errors.New("cloudflare DNS-01 requires CF_DNS_API_TOKEN (or tls.acme.cloudflare.api_token)")
	}

	mgr := &acmeManager{
		cfg:         cfg,
		upstreams:   upstreams,
		logger:      logger,
		stopCh:      make(chan struct{}),
		renewBefore: cfg.TLS.ACME.RenewBefore,
		cacheDir:    cfg.TLS.ACME.CacheDir,
		hosts:       cfg.TLS.ACME.Hosts,
		email:       cfg.TLS.ACME.Email,
		providerTok: token,
	}

	// Try loading cached cert synchronously (instant if exists).
	// If no cached cert, obtain one in the background so the DNS/API
	// servers can start immediately. DoT will return errors until
	// the cert is ready; UDP/TCP DNS and HTTP are unaffected.
	if cert, err := mgr.loadCached(); err == nil {
		mgr.certStore.Store(cert)
		logger.Info("ACME: loaded cached certificate")
	} else {
		logger.Info("ACME: no cached certificate, obtaining in background")
		mgr.startRetryLoop()
	}

	tlsCfg := &tls.Config{
		GetCertificate: mgr.getCertificate,
		MinVersion:     tls.VersionTLS13,
		NextProtos:     []string{"dot", "h2", "http/1.1"},
	}

	mgr.startRenewLoop()
	return mgr, tlsCfg, nil
}

func (m *acmeManager) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	v := m.certStore.Load()
	if v == nil {
		return nil, errors.New("certificate not initialized")
	}
	return v.(*tls.Certificate), nil
}

func (m *acmeManager) ensureCert() error {
	// Try loading existing cert
	if cert, err := m.loadCached(); err == nil {
		m.certStore.Store(cert)
		return nil
	}

	// Obtain new cert
	cert, err := m.obtainCert()
	if err != nil {
		return err
	}
	m.certStore.Store(cert)
	return nil
}

func (m *acmeManager) loadCached() (*tls.Certificate, error) {
	certPath := filepath.Join(m.cacheDir, "cert.pem")
	keyPath := filepath.Join(m.cacheDir, "key.pem")
	if _, err := os.Stat(certPath); err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	// Parse leaf for expiry and SAN validation.
	if len(cert.Certificate) > 0 {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("parse cached leaf: %w", err)
		}
		cert.Leaf = leaf

		// Reject expired certificates.
		if time.Now().After(leaf.NotAfter) {
			return nil, fmt.Errorf("cached cert expired at %s", leaf.NotAfter)
		}

		// Reject the cached cert if its SANs don't cover all configured hosts.
		if !certCoversHosts(leaf, m.hosts) {
			return nil, fmt.Errorf("cached cert SANs %v do not match configured hosts %v", leaf.DNSNames, m.hosts)
		}
	}
	return &cert, nil
}

// certCoversHosts returns true if the certificate's DNS SANs cover every host
// in the requested list (case-insensitive).
func certCoversHosts(leaf *x509.Certificate, hosts []string) bool {
	sans := make(map[string]struct{}, len(leaf.DNSNames))
	for _, name := range leaf.DNSNames {
		sans[strings.ToLower(name)] = struct{}{}
	}
	for _, h := range hosts {
		if _, ok := sans[strings.ToLower(h)]; !ok {
			return false
		}
	}
	return true
}

func (m *acmeManager) obtainCert() (*tls.Certificate, error) {
	if err := os.MkdirAll(m.cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	user, err := loadOrCreateACMEUser(m.email, m.cacheDir)
	if err != nil {
		return nil, err
	}
	cfg := lego.NewConfig(user)
	cfg.Certificate.KeyType = certcrypto.RSA2048

	// Honor configured upstream DNS servers (ACME-specific override already resolved by config)
	// for ACME/Cloudflare HTTP traffic instead of relying on the host resolver.
	var httpClient *http.Client
	var dnsChallengeOpts []dns01.ChallengeOption
	if len(m.upstreams) > 0 {
		res := resolver.NewStrict(m.upstreams, m.logger)
		m.logger.Info("ACME using strict upstream resolvers", "upstreams", m.upstreams)
		httpClient = res.NewHTTPClient(60 * time.Second)
		cfg.HTTPClient = httpClient
		dnsChallengeOpts = append(dnsChallengeOpts, dns01.AddRecursiveNameservers(m.upstreams))
	} else {
		m.logger.Warn("ACME using system resolver (no upstreams configured); API/DNS lookups may be blocked")
	}

	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	cfCfg := cloudflare.NewDefaultConfig()
	cfCfg.AuthToken = m.providerTok
	cfCfg.TTL = m.cfg.TLS.ACME.Cloudflare.TTL
	cfCfg.PropagationTimeout = m.cfg.TLS.ACME.Cloudflare.PropagationTimeout
	cfCfg.PollingInterval = m.cfg.TLS.ACME.Cloudflare.PollingInterval
	if httpClient != nil {
		cfCfg.HTTPClient = httpClient
	}

	var provider challenge.ProviderTimeout
	// Prefer recursive propagation checks (skip authoritative) when requested.
	if m.cfg.TLS.ACME.Cloudflare.SkipAuthNSCheck {
		dnsChallengeOpts = append(dnsChallengeOpts,
			dns01.DisableAuthoritativeNssPropagationRequirement(),
			dns01.RecursiveNSsPropagationRequirement(),
		)
	}

	// Add initial delay (once) before first propagation poll to avoid
	// poisoning recursive resolvers with negative cache entries.
	if delay := m.cfg.TLS.ACME.Cloudflare.InitialDelay; delay > 0 {
		var once sync.Once
		dnsChallengeOpts = append(dnsChallengeOpts, dns01.WrapPreCheck(func(domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
			once.Do(func() {
				m.logger.Info("ACME: initial DNS propagation delay", "delay", delay, "fqdn", fqdn)
				time.Sleep(delay)
			})
			return check(fqdn, value)
		}))
	}
	if m.cfg.TLS.ACME.Cloudflare.ZoneID != "" {
		provider, err = newCFZoneProvider(m.cfg.TLS.ACME.Cloudflare, m.providerTok, httpClient, m.logger)
		if err != nil {
			return nil, fmt.Errorf("init cloudflare provider with zone: %w", err)
		}
	} else {
		provider, err = cloudflare.NewDNSProviderConfig(cfCfg)
		if err != nil {
			return nil, fmt.Errorf("init cloudflare provider: %w", err)
		}
	}

	if err = client.Challenge.SetDNS01Provider(provider, dnsChallengeOpts...); err != nil {
		return nil, fmt.Errorf("set dns01 provider: %w", err)
	}

	if user.Registration != nil {
		m.logger.Info("ACME: reusing persisted account registration", "uri", user.Registration.URI)
	} else {
		reg, regErr := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if regErr != nil {
			return nil, fmt.Errorf("register acme account: %w", regErr)
		}
		user.Registration = reg
		if saveErr := user.saveRegistration(m.cacheDir); saveErr != nil {
			m.logger.Warn("ACME: failed to persist registration", "error", saveErr)
		}
	}

	req := certificate.ObtainRequest{Domains: m.hosts, Bundle: true}
	certRes, err := client.Certificate.Obtain(req)
	if err != nil {
		return nil, fmt.Errorf("obtain certificate: %w", err)
	}

	if err = os.WriteFile(filepath.Join(m.cacheDir, "cert.pem"), certRes.Certificate, 0o600); err != nil {
		return nil, fmt.Errorf("write cert: %w", err)
	}
	if err = os.WriteFile(filepath.Join(m.cacheDir, "key.pem"), certRes.PrivateKey, 0o600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	cert, err := tls.X509KeyPair(certRes.Certificate, certRes.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("load obtained keypair: %w", err)
	}
	if len(cert.Certificate) > 0 {
		if leaf, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			cert.Leaf = leaf
		}
	}

	m.logger.Info("ACME certificate obtained (DNS-01 Cloudflare)", "hosts", m.hosts, "cache", m.cacheDir)
	return &cert, nil
}

// startRetryLoop attempts to obtain an initial certificate with exponential backoff.
// It runs when the first ensureCert() call fails at startup.
func (m *acmeManager) startRetryLoop() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		backoff := 30 * time.Second
		const maxBackoff = 15 * time.Minute
		for {
			select {
			case <-time.After(backoff):
				m.logger.Info("ACME: retrying certificate obtain", "backoff", backoff)
				if err := m.ensureCert(); err != nil {
					m.logger.Error("ACME retry failed", "error", err, "next_retry", backoff*2)
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
					continue
				}
				m.logger.Info("ACME: certificate obtained after retry")
				return
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *acmeManager) startRenewLoop() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.maybeRenew()
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *acmeManager) maybeRenew() {
	v := m.certStore.Load()
	if v == nil {
		return
	}
	cert := v.(*tls.Certificate)
	leaf := cert.Leaf
	if leaf == nil && len(cert.Certificate) > 0 {
		if parsed, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
			leaf = parsed
		}
	}
	if leaf == nil {
		return
	}
	renewAt := leaf.NotAfter.Add(-m.renewBefore)
	if time.Now().Before(renewAt) {
		return
	}

	m.logger.Info("Attempting ACME renewal", "expires", leaf.NotAfter)

	const maxRetries = 3
	backoff := 2 * time.Minute
	for attempt := 1; attempt <= maxRetries; attempt++ {
		newCert, err := m.obtainCert()
		if err != nil {
			m.logger.Error("ACME renewal failed",
				"error", err,
				"attempt", attempt,
				"max_retries", maxRetries,
				"expires", leaf.NotAfter,
			)
			if attempt < maxRetries {
				select {
				case <-time.After(backoff):
					backoff *= 2
				case <-m.stopCh:
					return
				}
				continue
			}
			m.logger.Error("ACME renewal exhausted retries, will try again next cycle",
				"expires", leaf.NotAfter)
			return
		}
		m.certStore.Store(newCert)
		m.logger.Info("ACME renewal succeeded", "new_expiry", newCert.Leaf.NotAfter)
		return
	}
}

func (m *acmeManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// ------------------------------------------------------------------
// Cloudflare DNS-01 provider with fixed ZoneID (bypasses zone discovery)
// ------------------------------------------------------------------

type cfZoneProvider struct {
	zoneID      string
	token       string
	ttl         int
	timeout     time.Duration
	interval    time.Duration
	httpClient  *http.Client
	logger      *logging.Logger
	recordIDs   map[string]string
	recordIDsMu sync.Mutex
}

type cfRecordResponse struct {
	Result struct {
		ID string `json:"id"`
	} `json:"result"`
	Success bool            `json:"success"`
	Errors  json.RawMessage `json:"errors"`
}

func newCFZoneProvider(cfg config.CFConfig, token string, httpClient *http.Client, logger *logging.Logger) (*cfZoneProvider, error) {
	if token == "" {
		return nil, errors.New("cloudflare: api token required")
	}
	if strings.TrimSpace(cfg.ZoneID) == "" {
		return nil, errors.New("cloudflare: zone_id is required when using zone override")
	}
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &cfZoneProvider{
		zoneID:     cfg.ZoneID,
		token:      token,
		ttl:        cfg.TTL,
		timeout:    cfg.PropagationTimeout,
		interval:   cfg.PollingInterval,
		httpClient: client,
		logger:     logger,
		recordIDs:  make(map[string]string),
	}, nil
}

// purgeStaleRecords deletes any existing TXT records for the given FQDN.
// This prevents accumulation of orphaned challenge records from previous
// failed attempts or process restarts.
func (p *cfZoneProvider) purgeStaleRecords(fqdn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	listURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=TXT&name=%s",
		url.PathEscape(p.zoneID), url.QueryEscape(fqdn))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		p.logger.Warn("cloudflare: failed to build list request for stale records", "error", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Warn("cloudflare: failed to list stale records", "error", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}
	for _, r := range result.Result {
		p.deleteRecord(r.ID)
		p.logger.Info("cloudflare: purged stale challenge record", "fqdn", fqdn, "id", r.ID)
	}
}

// deleteRecord removes a single DNS record by ID (best-effort).
func (p *cfZoneProvider) deleteRecord(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", url.PathEscape(p.zoneID), url.PathEscape(id)),
		nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// Present creates the TXT using the configured ZoneID, skipping zone discovery.
func (p *cfZoneProvider) Present(domain, token, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	fqdn := dns01.UnFqdn(info.EffectiveFQDN)

	// Remove any stale challenge records from previous attempts
	p.purgeStaleRecords(fqdn)

	body, err := json.Marshal(map[string]any{
		"type":    "TXT",
		"name":    fqdn,
		"content": `"` + info.Value + `"`,
		"ttl":     p.ttl,
		"proxied": false,
	})
	if err != nil {
		return fmt.Errorf("cloudflare: marshal request body: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", url.PathEscape(p.zoneID)),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			p.logger.Warn("cloudflare: close response body (present)", "error", cerr)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare: API returned HTTP %d for record create", resp.StatusCode)
	}

	var parsed cfRecordResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("cloudflare: decode response: %w", err)
	}
	if !parsed.Success {
		return fmt.Errorf("cloudflare: API create failed: status %d errors %s", resp.StatusCode, string(parsed.Errors))
	}

	p.recordIDsMu.Lock()
	p.recordIDs[token] = parsed.Result.ID
	p.recordIDsMu.Unlock()
	p.logger.Info("cloudflare: new record (zone_id override)", "fqdn", fqdn, "id", parsed.Result.ID)
	return nil
}

// CleanUp deletes the TXT created in Present.
func (p *cfZoneProvider) CleanUp(domain, token, keyAuth string) error {
	p.recordIDsMu.Lock()
	id := p.recordIDs[token]
	delete(p.recordIDs, token)
	p.recordIDsMu.Unlock()
	if id == "" {
		return nil // nothing to clean
	}
	p.deleteRecord(id)
	return nil
}

// Timeout satisfies challenge.ProviderTimeout.
func (p *cfZoneProvider) Timeout() (timeout, interval time.Duration) {
	return p.timeout, p.interval
}

// acmeUser implements lego.User with persistent key storage.
type acmeUser struct {
	Email        string
	Registration *registration.Resource
	key          *ecdsa.PrivateKey
}

// loadOrCreateACMEUser loads a persisted ACME account key from cacheDir, or
// generates a new one and saves it. This avoids creating new LE accounts on
// every restart (which risks hitting rate limits).
func loadOrCreateACMEUser(email, cacheDir string) (*acmeUser, error) {
	keyPath := filepath.Join(cacheDir, "account.key.pem")
	regPath := filepath.Join(cacheDir, "account.json")

	user := &acmeUser{Email: email}

	// Try loading existing key
	if keyPEM, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(keyPEM)
		if block != nil {
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err == nil {
				user.key = key
			}
		}
	}

	// Generate new key if none loaded
	if user.key == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ACME account key: %w", err)
		}
		user.key = key

		// Persist the key
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("marshal ACME account key: %w", err)
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
		if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
			return nil, fmt.Errorf("write ACME account key: %w", err)
		}
	}

	// Try loading existing registration
	if regJSON, err := os.ReadFile(regPath); err == nil {
		var reg registration.Resource
		if err := json.Unmarshal(regJSON, &reg); err == nil {
			user.Registration = &reg
		}
	}

	return user, nil
}

// saveRegistration persists the ACME registration to disk.
func (u *acmeUser) saveRegistration(cacheDir string) error {
	if u.Registration == nil {
		return nil
	}
	regJSON, err := json.Marshal(u.Registration)
	if err != nil {
		return fmt.Errorf("marshal ACME registration: %w", err)
	}
	regPath := filepath.Join(cacheDir, "account.json")
	return os.WriteFile(regPath, regJSON, 0o600)
}

func (u *acmeUser) GetEmail() string                        { return u.Email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }
