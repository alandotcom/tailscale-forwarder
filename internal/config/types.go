package config

type ServiceMapping struct {
	Name       string
	SourcePort int
	TargetAddr string
	TargetPort int
}

type Config struct {
	TSHostname    string `env:"TS_HOSTNAME,required"`  // Base hostname for Tailscale machines
	TSAuthKey     string `env:"TS_AUTHKEY,required"`   // Tailscale authentication key
	TSStateDir    string `env:"TS_STATE_DIR,required"` // Persistent storage directory (prevents duplicate machines)
	TSEnableHTTPS bool   `env:"TS_ENABLE_HTTPS"`       // Enable HTTPS proxy with automatic TLS certificates
	LogLevel      string `env:"LOG_LEVEL"`             // Log level: debug, info (default), warn, error
	HealthAddr    string `env:"HEALTH_ADDR"`           // If set, serve /healthz and /readyz on this address (e.g. ":8080")

	ServiceMappings []ServiceMapping // List of TCP service mappings to forward
}
