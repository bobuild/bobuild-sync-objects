package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if job.Tables[0].Name != "category" || job.Tables[1].Name != "product" {
		t.Fatalf("tables: %+v", job.Tables)
	}
	if cfg.MaxFileSizeMB != DefaultMaxFileSizeMB {
		t.Fatalf("default max_file_size_mb: got %d want %d", cfg.MaxFileSizeMB, DefaultMaxFileSizeMB)
	}
	if job.Source.PathPrefix != "/sync" {
		t.Fatalf("trailing slash should be trimmed, got %q", job.Source.PathPrefix)
	}
	if job.Targets[0].PathPrefix != "/sync" {
		t.Fatalf("target prefix: %q", job.Targets[0].PathPrefix)
	}
}

func TestLoadTableFileFieldsAndMaxSize(t *testing.T) {
	path := writeConfig(t, `
max_file_size_mb: 50
jobs:
  - name: catalog
    tables:
      - category
      - name: product
        file_fields: [cover, datasheet]
    source:
      domain: source.example.com
      api_key: "src-key"
      path_prefix: /sync
    targets:
      - domain: replica.example.com
        api_key: "dst-key"
        path_prefix: /sync
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxFileSizeMB != 50 {
		t.Fatalf("max_file_size_mb: %d", cfg.MaxFileSizeMB)
	}
	if cfg.MaxFileBytes() != 50*1024*1024 {
		t.Fatalf("MaxFileBytes: %d", cfg.MaxFileBytes())
	}
	job := cfg.Jobs[0]
	if job.Tables[0].Name != "category" || len(job.Tables[0].FileFields) != 0 {
		t.Fatalf("category: %+v", job.Tables[0])
	}
	if job.Tables[1].Name != "product" {
		t.Fatalf("product name: %+v", job.Tables[1])
	}
	if len(job.Tables[1].FileFields) != 2 || job.Tables[1].FileFields[0] != "cover" || job.Tables[1].FileFields[1] != "datasheet" {
		t.Fatalf("file_fields: %+v", job.Tables[1].FileFields)
	}
}

func TestLoadRejectsBadFileFieldsAndMaxSize(t *testing.T) {
	cases := []string{
		`
max_file_size_mb: -1
jobs:
  - name: a
    tables: [t]
    source: {domain: s.example.com, api_key: k, path_prefix: /sync}
    targets: [{domain: t.example.com, api_key: k, path_prefix: /sync}]
`,
		`
jobs:
  - name: a
    tables:
      - name: product
        file_fields: [cover, cover]
    source: {domain: s.example.com, api_key: k, path_prefix: /sync}
    targets: [{domain: t.example.com, api_key: k, path_prefix: /sync}]
`,
		`
jobs:
  - name: a
    tables:
      - name: product
        file_fields: [""]
    source: {domain: s.example.com, api_key: k, path_prefix: /sync}
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

func TestLoadExpandsAPIKeyEnv(t *testing.T) {
	t.Setenv("BOBUILD_SYNC_TEST_SRC", "secret-src")
	t.Setenv("BOBUILD_SYNC_TEST_DST", "secret-dst")
	path := writeConfig(t, `
jobs:
  - name: catalog
    tables: [t]
    source:
      domain: source.example.com
      api_key: ${BOBUILD_SYNC_TEST_SRC}
      path_prefix: /sync
    targets:
      - domain: replica.example.com
        api_key: ${BOBUILD_SYNC_TEST_DST}
        path_prefix: /sync
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jobs[0].Source.APIKey != "secret-src" {
		t.Fatalf("source api_key: %q", cfg.Jobs[0].Source.APIKey)
	}
	if cfg.Jobs[0].Targets[0].APIKey != "secret-dst" {
		t.Fatalf("target api_key: %q", cfg.Jobs[0].Targets[0].APIKey)
	}
}

func TestLoadAPIKeyEnvUnsetOrEmpty(t *testing.T) {
	t.Setenv("BOBUILD_SYNC_TEST_EMPTY", "")
	path := writeConfig(t, `
jobs:
  - name: catalog
    tables: [t]
    source:
      domain: source.example.com
      api_key: ${BOBUILD_SYNC_TEST_MISSING}
      path_prefix: /sync
    targets:
      - domain: replica.example.com
        api_key: k
        path_prefix: /sync
`)
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `environment variable "BOBUILD_SYNC_TEST_MISSING" is unset or empty`) {
		t.Fatalf("expected unset env error, got %v", err)
	}

	path = writeConfig(t, `
jobs:
  - name: catalog
    tables: [t]
    source:
      domain: source.example.com
      api_key: ${BOBUILD_SYNC_TEST_EMPTY}
      path_prefix: /sync
    targets:
      - domain: replica.example.com
        api_key: k
        path_prefix: /sync
`)
	_, err = Load(path)
	if err == nil || !strings.Contains(err.Error(), `environment variable "BOBUILD_SYNC_TEST_EMPTY" is unset or empty`) {
		t.Fatalf("expected empty env error, got %v", err)
	}
}

func TestLoadAPIKeyLiteralAndNonMatching(t *testing.T) {
	t.Setenv("BOBUILD_SYNC_TEST_FOO", "should-not-expand")
	path := writeConfig(t, `
jobs:
  - name: catalog
    tables: [t]
    source:
      domain: source.example.com
      api_key: plain-key
      path_prefix: /sync
    targets:
      - domain: a.example.com
        api_key: $BOBUILD_SYNC_TEST_FOO
        path_prefix: /sync
      - domain: b.example.com
        api_key: ${BOBUILD_SYNC_TEST_FOO}bar
        path_prefix: /sync
      - domain: c.example.com
        api_key: prefix${BOBUILD_SYNC_TEST_FOO}
        path_prefix: /sync
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	job := cfg.Jobs[0]
	if job.Source.APIKey != "plain-key" {
		t.Fatalf("literal: %q", job.Source.APIKey)
	}
	if job.Targets[0].APIKey != "$BOBUILD_SYNC_TEST_FOO" {
		t.Fatalf("$FOO form: %q", job.Targets[0].APIKey)
	}
	if job.Targets[1].APIKey != "${BOBUILD_SYNC_TEST_FOO}bar" {
		t.Fatalf("suffix form: %q", job.Targets[1].APIKey)
	}
	if job.Targets[2].APIKey != "prefix${BOBUILD_SYNC_TEST_FOO}" {
		t.Fatalf("prefix form: %q", job.Targets[2].APIKey)
	}
}
