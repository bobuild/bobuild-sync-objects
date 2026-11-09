package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	path := writeConfig(t, `
jobs:
  - name: catalog
    tables: [category, product]
    source:
      domain: source.example.com
      api_key: "src-key"
      path_prefix: /sync/
    targets:
      - domain: replica.example.com
        api_key: "dst-key"
        path_prefix: /sync
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Jobs) != 1 {
		t.Fatalf("jobs: %+v", cfg.Jobs)
	}
	job := cfg.Jobs[0]
	if job.Name != "catalog" || len(job.Tables) != 2 {
		t.Fatalf("job: %+v", job)
	}
	if job.Source.PathPrefix != "/sync" {
		t.Fatalf("trailing slash should be trimmed, got %q", job.Source.PathPrefix)
	}
	if job.Targets[0].PathPrefix != "/sync" {
		t.Fatalf("target prefix: %q", job.Targets[0].PathPrefix)
	}
}

func TestLoadHostPortDefault(t *testing.T) {
	path := writeConfig(t, `
jobs:
  - name: j
    tables: [t]
    source:
      domain: source.example.com
      api_key: "k"
      path_prefix: /sync
      host: ingress.example
    targets:
      - domain: replica.example.com
        api_key: "k"
        path_prefix: /sync
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jobs[0].Source.Port != "80" {
		t.Fatalf("expected default port 80, got %q", cfg.Jobs[0].Source.Port)
	}
}

func TestLoadRejectsDuplicatesAndMissing(t *testing.T) {
	cases := []string{
		`jobs: []`,
		`
jobs:
  - name: a
    tables: [t]
    source: {domain: s.example.com, api_key: k, path_prefix: /sync}
    targets: [{domain: t.example.com, api_key: k, path_prefix: /sync}]
  - name: a
    tables: [t]
    source: {domain: s2.example.com, api_key: k, path_prefix: /sync}
    targets: [{domain: t2.example.com, api_key: k, path_prefix: /sync}]
`,
		`
jobs:
  - name: a
    tables: []
    source: {domain: s.example.com, api_key: k, path_prefix: /sync}
    targets: [{domain: t.example.com, api_key: k, path_prefix: /sync}]
`,
		`
jobs:
  - name: a
    tables: [t]
    source: {domain: s.example.com, api_key: k, path_prefix: sync}
    targets: [{domain: t.example.com, api_key: k, path_prefix: /sync}]
`,
		`
jobs:
  - name: a
    tables: [t]
    source: {domain: s.example.com, api_key: k, path_prefix: /sync, port: "80"}
    targets: [{domain: t.example.com, api_key: k, path_prefix: /sync}]
`,
	}
	for i, content := range cases {
		path := writeConfig(t, content)
		if _, err := Load(path); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestEndpointPaths(t *testing.T) {
	if got := EndpointPath("/sync", "product"); got != "/sync/product" {
		t.Fatalf("EndpointPath: %q", got)
	}
	if got := RecordPath("/sync", "product", 42); got != "/sync/product/42" {
		t.Fatalf("RecordPath: %q", got)
	}
}
