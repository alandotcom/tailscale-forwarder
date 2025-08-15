package config

type ServiceMapping struct {
	Name       string
	SourcePort int
	TargetAddr string
	TargetPort int
}

type config struct {
	TSHostname    string `env:"TS_HOSTNAME,required"`    // Base hostname for Tailscale machines
	TSAuthKey     string `env:"TS_AUTHKEY,required"`     // Tailscale authentication key
	TSExtraArgs   string `env:"TS_EXTRA_ARGS"`          // Additional Tailscale arguments (e.g., tags)
	TSStateDir    string `env:"TS_STATE_DIR,required"`   // Persistent storage directory (prevents duplicate machines)
	TSEnableHTTPS bool   `env:"TS_ENABLE_HTTPS"`        // Enable HTTPS proxy with automatic TLS certificates

	ServiceMappings []ServiceMapping // List of TCP service mappings to forward
}
