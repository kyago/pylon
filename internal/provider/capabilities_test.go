package provider

import (
	"reflect"
	"testing"
)

func TestParseCapabilitiesAndMissing(t *testing.T) {
	available, err := ParseCapabilities([]string{"read_files", "run_shell", "structured_output"})
	if err != nil {
		t.Fatal(err)
	}
	required, err := ParseCapabilities([]string{"read_files", "edit_files", "structured_output"})
	if err != nil {
		t.Fatal(err)
	}

	if available.Satisfies(required) {
		t.Fatal("capability set unexpectedly satisfies edit_files")
	}
	if got, want := available.Missing(required), []Capability{CapabilityEditFiles}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}
	if got := available.Names(); !reflect.DeepEqual(got, []string{"read_files", "run_shell", "structured_output"}) {
		t.Fatalf("names = %v", got)
	}
}

func TestParseCapabilitiesRejectsUnknownName(t *testing.T) {
	if _, err := ParseCapabilities([]string{"read_files", "telepathy"}); err == nil {
		t.Fatal("unknown capability was accepted")
	}
}
