// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package contract

import (
	"fmt"
	"sort"
	"strings"
)

type TopicNames struct {
	Announce          string
	ControlRoster     string
	ControlOnboarding string
	Requests          string
	Responses         string
	TaskStatus        string
	Traces            string
	ObserveAudit      string
	MemoryShared      string
	MemoryContext     string
	Orchestrator      string
}

func Topics(groupName string) TopicNames {
	return TopicNames{
		Announce:          fmt.Sprintf("group.%s.announce", groupName),
		ControlRoster:     fmt.Sprintf("group.%s.control.roster", groupName),
		ControlOnboarding: fmt.Sprintf("group.%s.control.onboarding", groupName),
		Requests:          fmt.Sprintf("group.%s.requests", groupName),
		Responses:         fmt.Sprintf("group.%s.responses", groupName),
		TaskStatus:        fmt.Sprintf("group.%s.tasks.status", groupName),
		Traces:            fmt.Sprintf("group.%s.traces", groupName),
		ObserveAudit:      fmt.Sprintf("group.%s.observe.audit", groupName),
		MemoryShared:      fmt.Sprintf("group.%s.memory.shared", groupName),
		MemoryContext:     fmt.Sprintf("group.%s.memory.context", groupName),
		Orchestrator:      fmt.Sprintf("group.%s.orchestrator", groupName),
	}
}

func TopicForFamily(groupName, family string) (string, bool) {
	topics := Topics(groupName)
	switch strings.TrimSpace(family) {
	case "announce":
		return topics.Announce, true
	case "control.roster":
		return topics.ControlRoster, true
	case "control.onboarding":
		return topics.ControlOnboarding, true
	case "requests":
		return topics.Requests, true
	case "responses":
		return topics.Responses, true
	case "tasks.status":
		return topics.TaskStatus, true
	case "traces":
		return topics.Traces, true
	case "observe.audit":
		return topics.ObserveAudit, true
	case "memory.shared":
		return topics.MemoryShared, true
	case "memory.context":
		return topics.MemoryContext, true
	case "orchestrator":
		return topics.Orchestrator, true
	default:
		return "", false
	}
}

func DeriveTopics(groupName string, required []string, optional []string, manual []string, mode string) []string {
	if strings.EqualFold(strings.TrimSpace(mode), "manual") {
		return normalizeTopics(manual)
	}
	topics := make([]string, 0, len(required)+len(optional))
	for _, family := range append(append([]string{}, required...), optional...) {
		if family == "skills" {
			continue
		}
		if topic, ok := TopicForFamily(groupName, family); ok {
			topics = append(topics, topic)
		}
	}
	return normalizeTopics(topics)
}

func SkillTopicPrefix(groupName string) string {
	return fmt.Sprintf("group.%s.skill.", groupName)
}

func ClassifyTopic(topic string, groupName string) string {
	switch strings.TrimSpace(topic) {
	case Topics(groupName).Announce:
		return "announce"
	case Topics(groupName).ControlRoster:
		return "control.roster"
	case Topics(groupName).ControlOnboarding:
		return "control.onboarding"
	case Topics(groupName).Requests:
		return "requests"
	case Topics(groupName).Responses:
		return "responses"
	case Topics(groupName).TaskStatus:
		return "tasks.status"
	case Topics(groupName).Traces:
		return "traces"
	case Topics(groupName).ObserveAudit:
		return "observe.audit"
	case Topics(groupName).MemoryShared:
		return "memory.shared"
	case Topics(groupName).MemoryContext:
		return "memory.context"
	case Topics(groupName).Orchestrator:
		return "orchestrator"
	default:
		if strings.HasPrefix(strings.TrimSpace(topic), SkillTopicPrefix(groupName)) {
			return "skills"
		}
		return ""
	}
}

func normalizeTopics(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
