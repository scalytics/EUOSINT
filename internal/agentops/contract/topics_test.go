package contract

import (
	"reflect"
	"testing"
)

func TestDeriveTopicsAuto(t *testing.T) {
	got := DeriveTopics("core", []string{"announce", "requests", "responses"}, []string{"orchestrator"}, nil, "auto")
	want := []string{
		"group.core.announce",
		"group.core.orchestrator",
		"group.core.requests",
		"group.core.responses",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("derived topics mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestDeriveTopicsManual(t *testing.T) {
	got := DeriveTopics("core", nil, nil, []string{"x", "x", " y "}, "manual")
	want := []string{"x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manual topics mismatch got=%#v want=%#v", got, want)
	}
}

func TestClassifyTopic(t *testing.T) {
	if family := ClassifyTopic("group.core.tasks.status", "core"); family != "tasks.status" {
		t.Fatalf("expected tasks.status, got %q", family)
	}
	if family := ClassifyTopic("group.core.skill.translate.requests", "core"); family != "skills" {
		t.Fatalf("expected skills, got %q", family)
	}
	if family := ClassifyTopic("group.other.requests", "core"); family != "" {
		t.Fatalf("expected empty family for foreign topic, got %q", family)
	}
}
