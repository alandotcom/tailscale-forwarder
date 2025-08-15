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

	if len(errs) > 0 {
		logger.StderrWithSource.Error("configuration error(s) found", logger.ErrorsAttr(errs...))
		os.Exit(1)
	}
}
