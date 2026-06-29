// Copyright 2026 AgentOS Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kazyamaz200/agentos/internal/llm"
)

type llmPreset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model"`
	APIKeyEnv string `json:"apiKeyEnv,omitempty"`
}

type publicLLMPreset struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	BaseURL       string `json:"baseUrl"`
	Model         string `json:"model"`
	KeyConfigured bool   `json:"keyConfigured"`
	Default       bool   `json:"default"`
}

type llmSettings struct {
	DefaultPreset string            `json:"defaultPreset"`
	Presets       []llmPreset       `json:"-"`
	PublicPresets []publicLLMPreset `json:"presets"`
}

func loadLLMSettings() llmSettings {
	presets := parseLLMPresets(os.Getenv("AGENTOS_LLM_PRESETS"))
	if len(presets) == 0 {
		cfg := llm.DefaultConfig()
		presets = []llmPreset{{
			ID:        "default",
			Name:      "Default LiteLLM",
			Provider:  "litellm",
			BaseURL:   cfg.BaseURL,
			Model:     cfg.ModelCoder,
			APIKeyEnv: "LITELLM_API_KEY",
		}}
	}

	defaultID := strings.TrimSpace(os.Getenv("AGENTOS_LLM_DEFAULT_PRESET"))
	if defaultID == "" || !hasLLMPreset(presets, defaultID) {
		defaultID = presets[0].ID
	}

	public := make([]publicLLMPreset, 0, len(presets))
	for _, preset := range presets {
		public = append(public, publicLLMPreset{
			ID:            preset.ID,
			Name:          preset.Name,
			Provider:      preset.Provider,
			BaseURL:       preset.BaseURL,
			Model:         preset.Model,
			KeyConfigured: preset.APIKeyEnv != "" && os.Getenv(preset.APIKeyEnv) != "",
			Default:       preset.ID == defaultID,
		})
	}

	return llmSettings{
		DefaultPreset: defaultID,
		Presets:       presets,
		PublicPresets: public,
	}
}

func parseLLMPresets(raw string) []llmPreset {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var presets []llmPreset
	if err := json.Unmarshal([]byte(raw), &presets); err != nil {
		return nil
	}
	clean := make([]llmPreset, 0, len(presets))
	seen := map[string]bool{}
	for _, preset := range presets {
		preset.ID = strings.TrimSpace(preset.ID)
		preset.Name = strings.TrimSpace(preset.Name)
		preset.Provider = strings.TrimSpace(preset.Provider)
		preset.BaseURL = strings.TrimRight(strings.TrimSpace(preset.BaseURL), "/")
		preset.Model = strings.TrimSpace(preset.Model)
		preset.APIKeyEnv = strings.TrimSpace(preset.APIKeyEnv)
		if preset.ID == "" || preset.BaseURL == "" || preset.Model == "" || seen[preset.ID] {
			continue
		}
		if preset.Name == "" {
			preset.Name = preset.ID
		}
		if preset.Provider == "" {
			preset.Provider = "litellm"
		}
		seen[preset.ID] = true
		clean = append(clean, preset)
	}
	return clean
}

func hasLLMPreset(presets []llmPreset, id string) bool {
	for _, preset := range presets {
		if preset.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) llmClientForPreset(id string) (llm.LLMClient, string, error) {
	if id == "" {
		id = s.llmSettings.DefaultPreset
	}
	for _, preset := range s.llmSettings.Presets {
		if preset.ID != id {
			continue
		}
		cfg := llm.Config{
			BaseURL:    preset.BaseURL,
			ModelCoder: preset.Model,
		}
		if preset.APIKeyEnv != "" {
			cfg.APIKey = os.Getenv(preset.APIKeyEnv)
		}
		return llm.NewLiteLLMClient(cfg), preset.ID, nil
	}
	return nil, "", fmt.Errorf("unknown LLM preset: %s", id)
}
