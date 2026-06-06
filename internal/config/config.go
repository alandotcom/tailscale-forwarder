package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/alandotcom/tailscale-forwarder/internal/util"

	"github.com/caarlos0/env/v10"
)

// Load reads configuration from environment variables, validates it, and
// returns a fully-populated Config. All validation problems are aggregated and
// returned together so the caller can report them in one place. Load performs
// no logging and never exits the process; that decision belongs to main.
func Load() (*Config, error) {
	cfg := &Config{}
	var errs []error

	if err := env.Parse(cfg); err != nil {
		var aggErr env.AggregateError
		if errors.As(err, &aggErr) {
			errs = append(errs, aggErr.Errors...)
		} else {
			errs = append(errs, err)
		}
	}

	mappings, err := parseServiceMappings(os.Environ(), "SERVICE_")
	if err != nil {
		errs = append(errs, err)
	} else if len(mappings) == 0 {
		errs = append(errs, errors.New(`required environment variable "SERVICE_[n]" is not set`))
	}
	cfg.ServiceMappings = mappings

	sanitizedHostname := util.SanitizeString(cfg.TSHostname)
	if sanitizedHostname == "" {
		errs = append(errs, fmt.Errorf("TS_HOSTNAME must be a valid hostname, before sanitization: %q, after sanitization: %q", cfg.TSHostname, sanitizedHostname))
	}
	cfg.TSHostname = sanitizedHostname

	// Require TS_STATE_DIR (env.Parse already enforces presence; this adds an
	// existence check so a misconfigured path fails at startup, not mid-run).
	if cfg.TSStateDir != "" {
		if _, err := os.Stat(cfg.TSStateDir); os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("TS_STATE_DIR directory does not exist: %s", cfg.TSStateDir))
		} else if err != nil {
			errs = append(errs, fmt.Errorf("TS_STATE_DIR directory error: %w", err))
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}
