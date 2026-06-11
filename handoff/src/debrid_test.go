package main

import (
	"testing"
)

func TestDebridProviderForHost(t *testing.T) {
	// Setup mock providers
	originalProviders := debridProviders
	defer func() { debridProviders = originalProviders }()

	debridProviders = []DebridProvider{
		&RealDebridProvider{},
		&AllDebridProvider{},
		&PremiumizeProvider{},
		&OffcloudProvider{},
	}

	tests := []struct {
		url      string
		expected string
	}{
		{"https://real-debrid.com/some-link", "Real-Debrid"},
		{"https://alldebrid.com/some-link", "AllDebrid"},
		{"https://alldebrid.fr/some-link", "AllDebrid"},
		{"https://premiumize.me/some-link", "Premiumize"},
		{"https://offcloud.com/some-link", "Offcloud"},
		{"https://unknown-host.com/some-link", ""},
	}

	for _, tt := range tests {
		provider := getDebridProviderForHost(tt.url)
		if provider == nil {
			if tt.expected != "" {
				t.Errorf("Expected %s for %s, got nil", tt.expected, tt.url)
			}
		} else {
			if provider.Name() != tt.expected {
				t.Errorf("Expected %s for %s, got %s", tt.expected, tt.url, provider.Name())
			}
		}
	}
}
