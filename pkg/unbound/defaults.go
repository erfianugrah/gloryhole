package unbound

// DefaultServerConfig returns sensible defaults matching the shipped
// deploy/unbound/unbound.conf. These values are used when no custom
// config is provided in config.yml.
func DefaultServerConfig(listenPort int, controlSocket string) *UnboundServerConfig {
	return &UnboundServerConfig{
		Server: ServerBlock{
			Interface: "127.0.0.1",
			Port:      listenPort,

			// Cache (sized for sidecar behind Glory-Hole)
			MsgCacheSize:   "32m",
			RRSetCacheSize: "64m",
			KeyCacheSize:   "16m",
			CacheMinTTL:    0,
			CacheMaxNegTTL: 60,

			// DNSSEC
			ModuleConfig: "validator iterator",

			// Hardening
			HardenGlue:     true,
			HardenDNSSEC:   true,
			HardenBelowNX:  true,
			HardenAlgoDown: true,
			QnameMin:       true,
			AggressiveNSEC: true,

			// Performance
			NumThreads:          2,
			EDNSBufferSize:      1232,
			OutgoingRange:       4096,
			NumQueriesPerThread: 1024,
			SoReusePort:         true,

			// Serve stale
			ServeExpired:              true,
			ServeExpiredTTL:           86400,
			ServeExpiredClientTimeout: 1800,

			// Prefetch
			Prefetch:    true,
			PrefetchKey: true,

			// Logging (minimal — Glory-Hole handles query logging)
			Verbosity:   1,
			LogServfail: true,

			// Privacy
			HideIdentity:     true,
			HideVersion:      true,
			MinimalResponses: true,

			// Statistics
			ExtendedStatistics:   true,
			StatisticsCumulative: true,

			// TLS
			TLSCertBundle: "/etc/ssl/certs/ca-certificates.crt",

			// Paths
			RootHints:       "/etc/unbound/root.hints",
			AutoTrustAnchor: "/etc/unbound/root.key",

			// Access control (loopback only)
			AccessControl: []ACLEntry{
				{Netblock: "127.0.0.1/32", Action: "allow"},
				{Netblock: "0.0.0.0/0", Action: "refuse"},
			},

			// Privacy
			PrivateAddress: []string{
				"192.168.0.0/16",
				"169.254.0.0/16",
				"172.16.0.0/12",
				"10.0.0.0/8",
				"fd00::/8",
				"fe80::/10",
			},
		},

		Dnstap: DnstapConfig{
			Enabled:                     true,
			SocketPath:                  "/var/run/unbound/dnstap.sock",
			SendIdentity:                true,
			SendVersion:                 true,
			LogClientQueryMessages:      true,
			LogClientResponseMessages:   true,
			LogResolverQueryMessages:    true,
			LogResolverResponseMessages: true,
		},

		RemoteControl: RemoteControl{
			Enabled:          true,
			ControlInterface: controlSocket,
			ControlUseCert:   false,
		},
	}
}
