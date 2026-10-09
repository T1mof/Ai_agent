package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai_agent/internal/types"

	"gopkg.in/yaml.v3"
)

type localSecrets struct {
	APIKey string `yaml:"api_key"`
}

func NewFromSpec(spec *types.Spec, specPath string) (Client, error) {
	if spec == nil || !spec.LLM.Enabled {
		return nil, nil
	}

	provider := strings.TrimSpace(spec.LLM.Provider)
	if provider == "" {
		provider = "openai_compatible"
	}
	if provider != "openai_compatible" {
		return nil, fmt.Errorf("unsupported llm provider: %s", provider)
	}

	baseURL := strings.TrimSpace(spec.LLM.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("llm.base_url is required when llm is enabled")
	}

	model := strings.TrimSpace(spec.LLM.Model)
	if model == "" {
		return nil, fmt.Errorf("llm.model is required when llm is enabled")
	}

	apiKey, err := resolveAPIKey(spec, specPath)
	if err != nil {
		return nil, err
	}

	return NewOpenAICompatibleClient(baseURL, apiKey, model), nil
}

func resolveAPIKey(spec *types.Spec, specPath string) (string, error) {
	localPath := strings.TrimSpace(spec.LLM.LocalConfigPath)
	if localPath == "" {
		if specPath != "" {
			localPath = filepath.Join(filepath.Dir(specPath), "llm.local.yaml")
		}
	}

	if localPath != "" && !filepath.IsAbs(localPath) && specPath != "" {
		localPath = filepath.Join(filepath.Dir(specPath), localPath)
	}

	if localPath != "" {
		if data, err := os.ReadFile(localPath); err == nil {
			var secrets localSecrets
			if err := yaml.Unmarshal(data, &secrets); err != nil {
				return "", fmt.Errorf("parse llm local config: %w", err)
			}
			if strings.TrimSpace(secrets.APIKey) != "" {
				return strings.TrimSpace(secrets.APIKey), nil
			}
		}
	}

	// Optional fallback for local experiments.
	if env := strings.TrimSpace(os.Getenv("LLM_API_KEY")); env != "" {
		return env, nil
	}

	return "", fmt.Errorf("llm api key not found; provide llm.local.yaml or LLM_API_KEY")
}
