package unbound

// UnboundServerConfig is the full typed representation of unbound.conf.
// Glory-Hole owns this config and always writes the complete file.
type UnboundServerConfig struct {
	Server        ServerBlock   `yaml:"server" json:"server"`
	ForwardZones  []ForwardZone `yaml:"forward_zones" json:"forward_zones"`
	StubZones     []StubZone    `yaml:"stub_zones" json:"stub_zones"`
	RemoteControl RemoteControl `yaml:"remote_control" json:"remote_control"`
}

// ServerBlock holds the server: section of unbound.conf.
type ServerBlock struct {
	// Network
	Interface string `yaml:"interface" json:"interface"`
	Port      int    `yaml:"port" json:"port"`

	// Cache (essential — exposed in UI)
	MsgCacheSize   string `yaml:"msg_cache_size" json:"msg_cache_size"`
	RRSetCacheSize string `yaml:"rrset_cache_size" json:"rrset_cache_size"`
	KeyCacheSize   string `yaml:"key_cache_size" json:"key_cache_size"`
	CacheMaxTTL    int    `yaml:"cache_max_ttl,omitempty" json:"cache_max_ttl,omitempty"`
	CacheMinTTL    int    `yaml:"cache_min_ttl" json:"cache_min_ttl"`
	CacheMaxNegTTL int    `yaml:"cache_max_negative_ttl" json:"cache_max_negative_ttl"`

	// DNSSEC (essential)
	ModuleConfig   string   `yaml:"module_config" json:"module_config"`
	DomainInsecure []string `yaml:"domain_insecure,omitempty" json:"domain_insecure,omitempty"`

	// Hardening (essential)
	HardenGlue     bool `yaml:"harden_glue" json:"harden_glue"`
	HardenDNSSEC   bool `yaml:"harden_dnssec_stripped" json:"harden_dnssec_stripped"`
	HardenBelowNX  bool `yaml:"harden_below_nxdomain" json:"harden_below_nxdomain"`
	HardenAlgoDown bool `yaml:"harden_algo_downgrade" json:"harden_algo_downgrade"`
	QnameMin       bool `yaml:"qname_minimisation" json:"qname_minimisation"`
	AggressiveNSEC bool `yaml:"aggressive_nsec" json:"aggressive_nsec"`

	// Performance (essential: threads)
	NumThreads int `yaml:"num_threads" json:"num_threads"`

	// Performance (advanced)
	EDNSBufferSize      int  `yaml:"edns_buffer_size,omitempty" json:"edns_buffer_size,omitempty"`
	OutgoingRange       int  `yaml:"outgoing_range,omitempty" json:"outgoing_range,omitempty"`
	NumQueriesPerThread int  `yaml:"num_queries_per_thread,omitempty" json:"num_queries_per_thread,omitempty"`
	SoReusePort         bool `yaml:"so_reuseport,omitempty" json:"so_reuseport,omitempty"`

	// Serve Stale (essential)
	ServeExpired              bool `yaml:"serve_expired" json:"serve_expired"`
	ServeExpiredTTL           int  `yaml:"serve_expired_ttl" json:"serve_expired_ttl"`
	ServeExpiredClientTimeout int  `yaml:"serve_expired_client_timeout,omitempty" json:"serve_expired_client_timeout,omitempty"`

	// Prefetch
	Prefetch    bool `yaml:"prefetch" json:"prefetch"`
	PrefetchKey bool `yaml:"prefetch_key" json:"prefetch_key"`

	// Logging (essential: verbosity)
	Verbosity   int  `yaml:"verbosity" json:"verbosity"`
	LogQueries  bool `yaml:"log_queries,omitempty" json:"log_queries,omitempty"`
	LogReplies  bool `yaml:"log_replies,omitempty" json:"log_replies,omitempty"`
	LogServfail bool `yaml:"log_servfail,omitempty" json:"log_servfail,omitempty"`

	// Privacy
	HideIdentity     bool `yaml:"hide_identity" json:"hide_identity"`
	HideVersion      bool `yaml:"hide_version" json:"hide_version"`
	MinimalResponses bool `yaml:"minimal_responses" json:"minimal_responses"`

	// Statistics
	ExtendedStatistics   bool `yaml:"extended_statistics" json:"extended_statistics"`
	StatisticsCumulative bool `yaml:"statistics_cumulative" json:"statistics_cumulative"`

	// TLS
	TLSCertBundle string `yaml:"tls_cert_bundle,omitempty" json:"tls_cert_bundle,omitempty"`

	// Paths
	RootHints       string `yaml:"root_hints,omitempty" json:"root_hints,omitempty"`
	AutoTrustAnchor string `yaml:"auto_trust_anchor_file,omitempty" json:"auto_trust_anchor_file,omitempty"`

	// Access control
	AccessControl  []ACLEntry `yaml:"access_control,omitempty" json:"access_control,omitempty"`
	PrivateAddress []string   `yaml:"private_address,omitempty" json:"private_address,omitempty"`
}

// ACLEntry represents an access-control directive.
type ACLEntry struct {
	Netblock string `yaml:"netblock" json:"netblock"`
	Action   string `yaml:"action" json:"action"` // allow, deny, refuse
}

// ForwardZone represents a forward-zone block.
type ForwardZone struct {
	Name         string   `yaml:"name" json:"name"`
	ForwardAddrs []string `yaml:"forward_addrs" json:"forward_addrs"`
	ForwardFirst bool     `yaml:"forward_first,omitempty" json:"forward_first,omitempty"`
	ForwardTLS   bool     `yaml:"forward_tls_upstream,omitempty" json:"forward_tls_upstream,omitempty"`
}

// StubZone represents a stub-zone block.
type StubZone struct {
	Name      string   `yaml:"name" json:"name"`
	StubAddrs []string `yaml:"stub_addrs" json:"stub_addrs"`
	StubPrime bool     `yaml:"stub_prime,omitempty" json:"stub_prime,omitempty"`
	StubFirst bool     `yaml:"stub_first,omitempty" json:"stub_first,omitempty"`
}

// RemoteControl represents the remote-control: block.
type RemoteControl struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	ControlInterface string `yaml:"control_interface" json:"control_interface"`
	ControlUseCert   bool   `yaml:"control_use_cert" json:"control_use_cert"`
}
