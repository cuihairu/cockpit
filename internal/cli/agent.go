package cli

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/cuihairu/cockpit/internal/agent"
)

// AgentStartCmd starts a Cockpit agent instance.
type AgentStartCmd struct {
	Server string
	ID     string
	Secret string
	Region string
	Zone   string
	Labels string
}

// AgentStartUsage customizes shared flag descriptions.
type AgentStartUsage struct {
	Server string
	ID     string
	Secret string
	Region string
	Zone   string
	Labels string
}

var defaultAgentStartUsage = AgentStartUsage{
	Server: "Server WebSocket address (required)",
	ID:     "Agent ID (optional, auto-generated when omitted)",
	Secret: "Agent authentication secret (optional but recommended)",
	Region: "Region (optional)",
	Zone:   "Zone (optional)",
	Labels: "Labels, e.g. env=prod,services=[docker,k8s],gpu=true",
}

// Bind registers shared agent start flags on the provided FlagSet.
func (c *AgentStartCmd) Bind(fs *flag.FlagSet) {
	c.BindWithUsage(fs, defaultAgentStartUsage)
}

// BindWithUsage registers shared agent start flags with custom descriptions.
func (c *AgentStartCmd) BindWithUsage(fs *flag.FlagSet, usage AgentStartUsage) {
	fs.StringVar(&c.Server, "server", "", fallbackUsage(usage.Server, defaultAgentStartUsage.Server))
	fs.StringVar(&c.ID, "id", "", fallbackUsage(usage.ID, defaultAgentStartUsage.ID))
	fs.StringVar(&c.Secret, "secret", "", fallbackUsage(usage.Secret, defaultAgentStartUsage.Secret))
	fs.StringVar(&c.Region, "region", "", fallbackUsage(usage.Region, defaultAgentStartUsage.Region))
	fs.StringVar(&c.Zone, "zone", "", fallbackUsage(usage.Zone, defaultAgentStartUsage.Zone))
	fs.StringVar(&c.Labels, "labels", "", fallbackUsage(usage.Labels, defaultAgentStartUsage.Labels))
}

// Validate checks command arguments.
func (c *AgentStartCmd) Validate() error {
	if strings.TrimSpace(c.Server) == "" {
		return errors.New("missing required -server flag")
	}
	return nil
}

// BuildConfig converts CLI options to runtime agent config.
func (c *AgentStartCmd) BuildConfig() (agent.Config, error) {
	if err := c.Validate(); err != nil {
		return agent.Config{}, err
	}

	return agent.Config{
		ServerURL: c.Server,
		AgentID:   c.ID,
		Secret:    c.Secret,
		Region:    c.Region,
		Zone:      c.Zone,
		Labels:    parseLabels(c.Labels),
	}, nil
}

// Run starts the Cockpit agent.
func (c *AgentStartCmd) Run() error {
	cfg, err := c.BuildConfig()
	if err != nil {
		return err
	}

	a := agent.NewAgent(cfg)
	if err := a.Start(); err != nil {
		return fmt.Errorf("agent error: %w", err)
	}

	return nil
}

// parseLabels parses labels from a comma-separated key=value string.
func parseLabels(labelsStr string) map[string]interface{} {
	labels := make(map[string]interface{})
	if labelsStr == "" {
		return labels
	}

	for _, part := range splitLabelParts(labelsStr) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		idx := strings.Index(part, "=")
		if idx == -1 {
			log.Printf("Warning: invalid label format: %s", part)
			continue
		}

		key := strings.TrimSpace(part[:idx])
		valueStr := strings.TrimSpace(part[idx+1:])
		if key == "" {
			log.Printf("Warning: empty label key in: %s", part)
			continue
		}

		labels[key] = parseLabelValue(valueStr)
	}

	return labels
}

func splitLabelParts(input string) []string {
	parts := make([]string, 0, 4)
	var current strings.Builder
	bracketDepth := 0

	for _, r := range input {
		switch r {
		case '[':
			bracketDepth++
			current.WriteRune(r)
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
			current.WriteRune(r)
		case ',':
			if bracketDepth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func fallbackUsage(usage, fallback string) string {
	if strings.TrimSpace(usage) == "" {
		return fallback
	}
	return usage
}

// parseLabelValue parses a label value from string form.
func parseLabelValue(valueStr string) interface{} {
	if strings.HasPrefix(valueStr, "[") && strings.HasSuffix(valueStr, "]") {
		inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(valueStr, "["), "]"))
		if inner == "" {
			return []string{}
		}

		items := splitLabelParts(inner)
		result := make([]string, 0, len(items))
		for _, item := range items {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}

	lower := strings.ToLower(valueStr)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}

	if num, err := strconv.Atoi(valueStr); err == nil {
		return num
	}

	return valueStr
}
