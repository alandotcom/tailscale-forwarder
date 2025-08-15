package config

import (
	"fmt"
	"os"

	"main/internal/logger"
	"main/internal/util"

	"github.com/caarlos0/env/v10"
)

var Cfg = config{}

func init() {
	var errs []error

	if err := env.Parse(&Cfg); err != nil {
		if e, ok := err.(env.AggregateError); ok {
			errs = append(errs, e.Errors...)
		} else {
			errs = append(errs, err)
		}
	}

	// Parse service mappings
	serviceMappings, err := parseServiceMappingsFromEnv("SERVICE_")
	if err != nil {
		errs = append(errs, err)
	}

	// Require at least one service mapping
	if len(serviceMappings) == 0 && err == nil {
		errs = append(errs, fmt.Errorf("required environment variable \"SERVICE_[n]\" is not set"))
	}

	Cfg.ServiceMappings = serviceMappings

	sanitizedHostname := util.SanitizeString(Cfg.TSHostname)

	if sanitizedHostname == "" {
		errs = append(errs, fmt.Errorf("TS_HOSTNAME must be a valid hostname, before sanitization: \"%s\", after sanitization: \"%s\"", Cfg.TSHostname, sanitizedHostname))
	}

	Cfg.TSHostname = sanitizedHostname

	// Validate HTTPS configuration
	if Cfg.TSEnableHTTPS && Cfg.TSStateDir == "" {
		errs = append(errs, fmt.Errorf("TS_STATE_DIR is required when TS_ENABLE_HTTPS is true"))
	}

	if Cfg.TSStateDir != "" {
		// Check if state directory exists and is writable
		if _, err := os.Stat(Cfg.TSStateDir); os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("TS_STATE_DIR directory does not exist: %s", Cfg.TSStateDir))
		} else if err != nil {
			errs = append(errs, fmt.Errorf("TS_STATE_DIR directory error: %w", err))
		}
	}

	if len(errs) > 0 {
		logger.StderrWithSource.Error("configuration error(s) found", logger.ErrorsAttr(errs...))
		os.Exit(1)
	}
}
