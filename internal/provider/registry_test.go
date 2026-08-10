package provider_test

import (
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/provider"
	"github.com/kyago/pylon/internal/provider/fake"
)

func TestRegistryRejectsInvalidProviderNames(t *testing.T) {
	for _, name := range []string{"", "auto"} {
		registry := provider.NewRegistry()
		err := registry.Register(fake.New(name, provider.CapabilitySet{}))
		if err == nil || !strings.Contains(err.Error(), "invalid provider name") {
			t.Fatalf("Register(%q) error = %v", name, err)
		}
	}
}

func TestRegistryRejectsDuplicateProviders(t *testing.T) {
	registry := provider.NewRegistry()
	if err := registry.Register(fake.New("provider-a", provider.CapabilitySet{})); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(fake.New("provider-a", provider.CapabilitySet{})); err == nil {
		t.Fatal("duplicate provider registration succeeded")
	}
}

func TestRegistryPreservesRegistrationOrder(t *testing.T) {
	registry := provider.NewRegistry()
	for _, name := range []string{"provider-b", "provider-a"} {
		if err := registry.Register(fake.New(name, provider.CapabilitySet{})); err != nil {
			t.Fatal(err)
		}
	}

	adapters := registry.Adapters()
	if len(adapters) != 2 || adapters[0].Name() != "provider-b" || adapters[1].Name() != "provider-a" {
		t.Fatalf("registration order = %v, %v", adapters[0].Name(), adapters[1].Name())
	}
}
