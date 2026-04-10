// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var allowedTopicFamilies = map[string]struct{}{
	"announce":           {},
	"control.roster":     {},
	"control.onboarding": {},
	"requests":           {},
	"responses":          {},
	"tasks.status":       {},
	"traces":             {},
	"observe.audit":      {},
	"memory.shared":      {},
	"memory.context":     {},
	"orchestrator":       {},
	"skills":             {},
}

type Policy struct {
	Version        int           `yaml:"version"`
	GroupName      string        `yaml:"group_name"`
	TopicMode      string        `yaml:"topic_mode"`
	RequiredTopics []string      `yaml:"required_topics"`
	OptionalTopics []string      `yaml:"optional_topics"`
	Grouping       Grouping      `yaml:"grouping"`
	UI             UIPolicy      `yaml:"ui"`
	Hybrid         HybridPolicy  `yaml:"hybrid"`
}

type Grouping struct {
	FlowKey          string `yaml:"flow_key"`
	ReplayMaxRecords int    `yaml:"replay_max_records"`
}

type UIPolicy struct {
	ShowTopicHealth bool `yaml:"show_topic_health"`
	ShowMemory      bool `yaml:"show_memory"`
	ShowOrchestrator bool `yaml:"show_orchestrator"`
}

type HybridPolicy struct {
	EnabledCategories []string `yaml:"enabled_categories"`
}

func DefaultPolicy(groupName string) Policy {
	return Policy{
		Version:   1,
		GroupName: strings.TrimSpace(groupName),
		TopicMode: "auto",
		RequiredTopics: []string{
			"announce",
			"requests",
			"responses",
			"tasks.status",
			"traces",
			"observe.audit",
		},
		OptionalTopics: []string{
			"control.roster",
			"control.onboarding",
			"memory.shared",
			"memory.context",
			"orchestrator",
			"skills",
		},
		Grouping: Grouping{
			FlowKey:          "correlation_id",
			ReplayMaxRecords: 5000,
		},
		UI: UIPolicy{
			ShowTopicHealth:  true,
			ShowMemory:       false,
			ShowOrchestrator: true,
		},
	}
}

func LoadPolicy(path string, groupName string) (Policy, error) {
	if strings.TrimSpace(path) == "" {
		policy := DefaultPolicy(groupName)
		return policy, ValidatePolicy(policy)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := yaml.Unmarshal(raw, &policy); err != nil {
		return Policy{}, fmt.Errorf("parse policy: %w", err)
	}
	if strings.TrimSpace(policy.GroupName) == "" {
		policy.GroupName = strings.TrimSpace(groupName)
	}
	if policy.Version == 0 {
		policy.Version = 1
	}
	if strings.TrimSpace(policy.TopicMode) == "" {
		policy.TopicMode = "auto"
	}
	if strings.TrimSpace(policy.Grouping.FlowKey) == "" {
		policy.Grouping.FlowKey = "correlation_id"
	}
	if policy.Grouping.ReplayMaxRecords == 0 {
		policy.Grouping.ReplayMaxRecords = 5000
	}
	return policy, ValidatePolicy(policy)
}

func ValidatePolicy(policy Policy) error {
	if policy.Version != 1 {
		return fmt.Errorf("unsupported policy version %d", policy.Version)
	}
	switch strings.ToLower(strings.TrimSpace(policy.TopicMode)) {
	case "auto", "manual":
	default:
		return fmt.Errorf("invalid topic_mode %q", policy.TopicMode)
	}
	if strings.TrimSpace(policy.Grouping.FlowKey) != "correlation_id" {
		return fmt.Errorf("unsupported grouping.flow_key %q", policy.Grouping.FlowKey)
	}
	if policy.Grouping.ReplayMaxRecords <= 0 {
		return fmt.Errorf("grouping.replay_max_records must be > 0")
	}
	if err := validateTopicFamilies("required_topics", policy.RequiredTopics); err != nil {
		return err
	}
	if err := validateTopicFamilies("optional_topics", policy.OptionalTopics); err != nil {
		return err
	}
	for _, category := range policy.Hybrid.EnabledCategories {
		if strings.TrimSpace(category) == "" {
			return fmt.Errorf("hybrid.enabled_categories cannot contain empty entries")
		}
	}
	return nil
}

func validateTopicFamilies(field string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("%s cannot contain empty topic families", field)
		}
		if _, ok := allowedTopicFamilies[key]; !ok {
			return fmt.Errorf("%s contains unsupported topic family %q", field, key)
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%s contains duplicate topic family %q", field, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}
