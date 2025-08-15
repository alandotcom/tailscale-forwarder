package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

func parseServiceMappingsFromEnv(prefix string) ([]ServiceMapping, error) {
	serviceMappings := []ServiceMapping{}

	for _, envVar := range os.Environ() {
		kv := strings.SplitN(envVar, "=", 2)

		if len(kv) != 2 {
			continue
		}

		if !strings.HasPrefix(kv[0], prefix) {
			continue
		}

		// Expected format: SERVICE_01=servicename:sourceport:targetaddr:targetport
		parts := strings.SplitN(kv[1], ":", 4)

		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid service mapping format: %s (expected: servicename:sourceport:targetaddr:targetport)", kv[1])
		}

		serviceName := strings.TrimSpace(parts[0])
		if serviceName == "" {
			return nil, fmt.Errorf("service name cannot be empty in mapping: %s", kv[1])
		}

		sourcePort, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid source port: %s", parts[1])
		}

		targetPort, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid target port: %s", parts[3])
		}

		serviceMappings = append(serviceMappings, ServiceMapping{
			Name:       serviceName,
			SourcePort: sourcePort,
			TargetAddr: parts[2],
			TargetPort: targetPort,
		})
	}

	// Check for duplicate service names and source ports
	serviceNames := []string{}
	sourcePorts := []int{}

	for _, serviceMapping := range serviceMappings {
		if slices.Contains(serviceNames, serviceMapping.Name) {
			return nil, fmt.Errorf("duplicate service name %s found in service mappings", serviceMapping.Name)
		}
		if slices.Contains(sourcePorts, serviceMapping.SourcePort) {
			return nil, fmt.Errorf("duplicate source port %d found in service mappings", serviceMapping.SourcePort)
		}

		serviceNames = append(serviceNames, serviceMapping.Name)
		sourcePorts = append(sourcePorts, serviceMapping.SourcePort)
	}

	return serviceMappings, nil
}

func ParseExtraArgs(extraArgs string) []string {
	if extraArgs == "" {
		return nil
	}
	
	// Split by spaces, but handle quoted arguments
	var args []string
	var current strings.Builder
	inQuote := false
	
	for _, char := range extraArgs {
		switch char {
		case '"':
			inQuote = !inQuote
		case ' ':
			if inQuote {
				current.WriteRune(char)
			} else {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			}
		default:
			current.WriteRune(char)
		}
	}
	
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	
	return args
}
