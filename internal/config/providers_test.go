package config

import "testing"

func TestParseProvidersJSONAcceptsSnakeCase(t *testing.T) {
	raw := `{"deepseek":{"base_url":"https://api.deepseek.com/v1","api_key":"sk-test"}}`
	providers, err := ParseProvidersJSON(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	p := providers["deepseek"]
	if p.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("base_url=%q", p.BaseURL)
	}
	if p.APIKey != "sk-test" {
		t.Fatalf("api_key=%q", p.APIKey)
	}
}

func TestModelToolsDenied(t *testing.T) {
	if ModelToolsDenied(nil) {
		t.Fatal("nil meta must allow")
	}
	if ModelToolsDenied(&ModelDefinition{ID: "x", SupportsTools: true}) {
		t.Fatal("supports_tools=true must allow")
	}
	// Builtin known-false (reasoner)
	if !ModelToolsDenied(&ModelDefinition{ID: "deepseek-reasoner", SupportsTools: false}) {
		t.Fatal("builtin reasoner must deny")
	}
	// Sparse unknown (API ID-only shaped)
	if ModelToolsDenied(&ModelDefinition{ID: "vendor-mystery-v1", Name: "vendor-mystery-v1", SupportsTools: false}) {
		t.Fatal("sparse unknown must allow")
	}
	// Intentionally configured custom with tools off
	if !ModelToolsDenied(&ModelDefinition{
		ID:            "my-local",
		Name:          "My Local",
		ContextWindow: 8192,
		SupportsTools: false,
	}) {
		t.Fatal("filled custom with tools off must deny")
	}
}

func TestParseProvidersJSONAcceptsLegacyPascalCase(t *testing.T) {
	raw := `{"deepseek":{"BaseURL":"https://token.sensenova.cn/v1","APIKey":"sk-legacy"}}`
	providers, err := ParseProvidersJSON(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	p := providers["deepseek"]
	if p.BaseURL != "https://token.sensenova.cn/v1" {
		t.Fatalf("base_url=%q", p.BaseURL)
	}
	if p.APIKey != "sk-legacy" {
		t.Fatalf("api_key=%q", p.APIKey)
	}
}

func TestParseProvidersFromInterfaceMap(t *testing.T) {
	raw := map[string]interface{}{
		"deepseek": map[string]interface{}{
			"BaseURL": "https://api.example.com/v1",
			"APIKey":  "sk-map",
		},
	}
	providers, err := ParseProvidersFromInterface(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if providers["deepseek"].APIKey != "sk-map" {
		t.Fatalf("api_key=%q", providers["deepseek"].APIKey)
	}
}
