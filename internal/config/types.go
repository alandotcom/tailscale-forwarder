package config

type ServiceMapping struct {
	Name       string
	SourcePort int
	TargetAddr string
	TargetPort int
	HTTPS      bool
}

type Config struct {
	TSHostname      string   `env:"TS_HOSTNAME,required,notEmpty"`    // Base hostname for Tailscale machines
	TSAuthKey       string   `env:"TS_AUTHKEY,required,notEmpty"`     // Tailscale authentication key
	TSStateDir      string   `env:"TS_STATE_DIR,required,notEmpty"`   // Persistent storage directory
	TSEnableHTTPS   bool     `env:"TS_ENABLE_HTTPS"`                  // Enable HTTPS proxy with automatic TLS certificates
	TSHTTPSServices []string `env:"TS_HTTPS_SERVICES"`                // Overrides the global flag with selected service names
	LogLevel        string   `env:"LOG_LEVEL"`                        // Log level: debug, info (default), warn, error
	HealthAddr      string   `env:"HEALTH_ADDR"`                      // If set, serve /healthz and /readyz on this address (e.g. ":8080")
	MaxConnections  int      `env:"MAX_CONNECTIONS" envDefault:"256"` // Concurrent TCP forwards per service

	ServiceMappings []ServiceMapping // List of TCP service mappings to forward
}
