package llm

import (
	"reflect"
	"testing"
)

func TestBuildMultiProxyEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		proxies  []ProxySpec
		primary  string
		chain    []string
		expected []Endpoint
	}{
		{
			name:    "empty proxies returns nil",
			proxies: nil,
			primary: "a",
			chain:   []string{"b", "c"},
			expected: nil,
		},
		{
			name:    "single proxy delegates to BuildModelChainEndpoints",
			proxies: []ProxySpec{{URL: "http://p1/v1", Key: "k1"}},
			primary: "a",
			chain:   []string{"b", "c"},
			expected: []Endpoint{
				{URL: "http://p1/v1", Key: "k1", Model: "a"},
				{URL: "http://p1/v1", Key: "k1", Model: "b"},
				{URL: "http://p1/v1", Key: "k1", Model: "c"},
			},
		},
		{
			name: "two proxies cross-product proxy-first order",
			proxies: []ProxySpec{
				{URL: "http://local/v1", Key: "kl"},
				{URL: "http://remote/v1", Key: "kr"},
			},
			primary: "a",
			chain:   []string{"b", "c"},
			expected: []Endpoint{
				{URL: "http://local/v1", Key: "kl", Model: "a"},
				{URL: "http://local/v1", Key: "kl", Model: "b"},
				{URL: "http://local/v1", Key: "kl", Model: "c"},
				{URL: "http://remote/v1", Key: "kr", Model: "a"},
				{URL: "http://remote/v1", Key: "kr", Model: "b"},
				{URL: "http://remote/v1", Key: "kr", Model: "c"},
			},
		},
		{
			name: "primary deduped within each proxy segment",
			proxies: []ProxySpec{
				{URL: "http://p1/v1", Key: "k1"},
				{URL: "http://p2/v1", Key: "k2"},
			},
			primary: "a",
			chain:   []string{"a", "b"},
			expected: []Endpoint{
				{URL: "http://p1/v1", Key: "k1", Model: "a"},
				{URL: "http://p1/v1", Key: "k1", Model: "b"},
				{URL: "http://p2/v1", Key: "k2", Model: "a"},
				{URL: "http://p2/v1", Key: "k2", Model: "b"},
			},
		},
		{
			name: "empty primary skipped",
			proxies: []ProxySpec{
				{URL: "http://p1/v1", Key: "k1"},
				{URL: "http://p2/v1", Key: "k2"},
			},
			primary: "",
			chain:   []string{"a", "b"},
			expected: []Endpoint{
				{URL: "http://p1/v1", Key: "k1", Model: "a"},
				{URL: "http://p1/v1", Key: "k1", Model: "b"},
				{URL: "http://p2/v1", Key: "k2", Model: "a"},
				{URL: "http://p2/v1", Key: "k2", Model: "b"},
			},
		},
		{
			name: "empty chain models skipped",
			proxies: []ProxySpec{
				{URL: "http://p1/v1", Key: "k1"},
			},
			primary: "a",
			chain:   []string{"", "b", ""},
			expected: []Endpoint{
				{URL: "http://p1/v1", Key: "k1", Model: "a"},
				{URL: "http://p1/v1", Key: "k1", Model: "b"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMultiProxyEndpoints(tt.proxies, tt.primary, tt.chain)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("BuildMultiProxyEndpoints() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBuildMultiProxyEndpoints_SingleProxyMatchesChainBuilder(t *testing.T) {
	proxy := ProxySpec{URL: "http://p1/v1", Key: "k1"}
	primary := "a"
	chain := []string{"b", "c"}

	multi := BuildMultiProxyEndpoints([]ProxySpec{proxy}, primary, chain)
	single := BuildModelChainEndpoints(proxy.URL, proxy.Key, primary, chain)

	if !reflect.DeepEqual(multi, single) {
		t.Errorf("single-proxy multi = %v, want %v (same as BuildModelChainEndpoints)", multi, single)
	}
}
