package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alandotcom/tailscale-forwarder/internal/util"
)

// parseServiceMappings parses SERVICE_[n] entries from a list of "KEY=VALUE"
// environment strings (as returned by os.Environ). Entries whose key does not
// start with prefix are ignored. Duplicate service names (compared after
// sanitization, since that is what becomes the Tailscale hostname) and
// duplicate source ports are rejected.
func parseServiceMappings(environ []string, prefix string) ([]ServiceMapping, error) {
	serviceMappings := []ServiceMapping{}
	seenNames := map[string]struct{}{}
	seenPorts := map[int]struct{}{}

	for _, envVar := range environ {
		kv := strings.SplitN(envVar, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if !strings.HasPrefix(kv[0], prefix) {
			continue
		}

		mapping, err := parseServiceMapping(kv[1])
		if err != nil {
			return nil, err
		}

		sanitizedName := util.SanitizeString(mapping.Name)
		if _, dup := seenNames[sanitizedName]; dup {
			return nil, fmt.Errorf("duplicate service name %q found in service mappings (names must be unique after sanitization)", mapping.Name)
		}
		if _, dup := seenPorts[mapping.SourcePort]; dup {
			return nil, fmt.Errorf("duplicate source port %d found in service mappings", mapping.SourcePort)
		}
		seenNames[sanitizedName] = struct{}{}
		seenPorts[mapping.SourcePort] = struct{}{}

		serviceMappings = append(serviceMappings, mapping)
	}

	return serviceMappings, nil
}

// parseServiceMapping parses and validates a single mapping value in the form
// "servicename:sourceport:targetaddr:targetport".
func parseServiceMapping(value string) (ServiceMapping, error) {
	parts := strings.SplitN(value, ":", 4)
	if len(parts) != 4 {
		return ServiceMapping{}, fmt.Errorf("invalid service mapping format: %s (expected: servicename:sourceport:targetaddr:targetport)", value)
	}

	name := strings.TrimSpace(parts[0])
	if name == "" {
		return ServiceMapping{}, fmt.Errorf("service name cannot be empty in mapping: %s", value)
	}
	if util.SanitizeString(name) == "" {
		return ServiceMapping{}, fmt.Errorf("service name %q contains no usable hostname characters", name)
	}

	sourcePort, err := strconv.Atoi(parts[1])
	if err != nil || sourcePort < 1 || sourcePort > 65535 {
		return ServiceMapping{}, fmt.Errorf("invalid source port: %s (must be 1-65535)", parts[1])
	}

	targetAddr := strings.TrimSpace(parts[2])
	if targetAddr == "" {
		return ServiceMapping{}, fmt.Errorf("target address cannot be empty in mapping: %s", value)
	}

	targetPort, err := strconv.Atoi(parts[3])
	if err != nil || targetPort < 1 || targetPort > 65535 {
		return ServiceMapping{}, fmt.Errorf("invalid target port: %s (must be 1-65535)", parts[3])
	}

	return ServiceMapping{
		Name:       name,
		SourcePort: sourcePort,
		TargetAddr: targetAddr,
		TargetPort: targetPort,
	}, nil
}
