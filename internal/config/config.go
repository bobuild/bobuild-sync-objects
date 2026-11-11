package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envAPIKeyRef matches an api_key whose entire value is ${ENV_VAR}.
var envAPIKeyRef = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

// DefaultMaxFileSizeMB is used when max_file_size_mb is omitted or zero.
const DefaultMaxFileSizeMB = 100

// App identifies a Bobuild app and how to reach its API.
type App struct {
	Domain     string `yaml:"domain"`
	APIKey     string `yaml:"api_key"`
	PathPrefix string `yaml:"path_prefix"`
	// Host and Port optionally override the TCP connection target (e.g. cluster
	// ingress). When Host is set, the client dials Host:Port over plain HTTP
	// and keeps Domain for the request Host / URL.
	Host string `yaml:"host"`
	Port string `yaml:"port"`
	// InsecureSkipVerify disables TLS certificate verification for this app.
	// Use only for trusted internal/dev endpoints with private CAs.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// Table is one object synced in a job. YAML may be a bare string name or a
// mapping with optional file_fields.
type Table struct {
	Name       string   `yaml:"name"`
	FileFields []string `yaml:"file_fields"`
}

// UnmarshalYAML accepts either "product" or {name: product, file_fields: [...]}.
func (t *Table) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var name string
		if err := value.Decode(&name); err != nil {
			return err
		}
		t.Name = name
		t.FileFields = nil
		return nil
	case yaml.MappingNode:
		type raw Table
		var r raw
		if err := value.Decode(&r); err != nil {
			return err
		}
		*t = Table(r)
		return nil
	default:
		return fmt.Errorf("table must be a string or a mapping with name")
	}
}

// Job is one sync pipeline: tables from a source app into one or more targets.
type Job struct {
	Name    string  `yaml:"name"`
	Tables  []Table `yaml:"tables"`
	Source  App     `yaml:"source"`
	Targets []App   `yaml:"targets"`
}

type Config struct {
	// MaxFileSizeMB caps downloaded file payloads (default 100). Oversized
	// files skip the record to avoid container OOM.
	MaxFileSizeMB int   `yaml:"max_file_size_mb"`
	Jobs          []Job `yaml:"jobs"`
}

// MaxFileBytes returns the configured size cap in bytes.
func (cfg *Config) MaxFileBytes() int64 {
	return int64(cfg.MaxFileSizeMB) * 1024 * 1024
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg *Config) validate() error {
	if cfg.MaxFileSizeMB < 0 {
		return fmt.Errorf("max_file_size_mb must be >= 0")
	}
	if cfg.MaxFileSizeMB == 0 {
		cfg.MaxFileSizeMB = DefaultMaxFileSizeMB
	}
	if len(cfg.Jobs) == 0 {
		return fmt.Errorf("at least one job is required")
	}
	names := make(map[string]struct{}, len(cfg.Jobs))
	for i := range cfg.Jobs {
		job := &cfg.Jobs[i]
		label := fmt.Sprintf("jobs[%d]", i)
		if job.Name == "" {
			return fmt.Errorf("%s: name is required", label)
		}
		if _, dup := names[job.Name]; dup {
			return fmt.Errorf("%s: duplicate job name %q", label, job.Name)
		}
		names[job.Name] = struct{}{}
		if len(job.Tables) == 0 {
			return fmt.Errorf("%s: at least one table is required", label)
		}
		for j := range job.Tables {
			table := &job.Tables[j]
			tLabel := fmt.Sprintf("%s.tables[%d]", label, j)
			if strings.TrimSpace(table.Name) == "" {
				return fmt.Errorf("%s: empty table name", tLabel)
			}
			seenFields := make(map[string]struct{}, len(table.FileFields))
			for k, field := range table.FileFields {
				field = strings.TrimSpace(field)
				if field == "" {
					return fmt.Errorf("%s.file_fields[%d]: empty field name", tLabel, k)
				}
				table.FileFields[k] = field
				if _, dup := seenFields[field]; dup {
					return fmt.Errorf("%s.file_fields: duplicate %q", tLabel, field)
				}
				seenFields[field] = struct{}{}
			}
		}
		if err := validateApp(label+".source", &job.Source); err != nil {
			return err
		}
		if len(job.Targets) == 0 {
			return fmt.Errorf("%s: at least one target is required", label)
		}
		targetDomains := make(map[string]struct{}, len(job.Targets))
		for j := range job.Targets {
			t := &job.Targets[j]
			tLabel := fmt.Sprintf("%s.targets[%d]", label, j)
			if err := validateApp(tLabel, t); err != nil {
				return err
			}
			if _, dup := targetDomains[t.Domain]; dup {
				return fmt.Errorf("%s: duplicate target domain %q", tLabel, t.Domain)
			}
			targetDomains[t.Domain] = struct{}{}
		}
	}
	return nil
}

func validateApp(label string, a *App) error {
	if a.Domain == "" || a.APIKey == "" {
		return fmt.Errorf("%s: domain and api_key are required", label)
	}
	resolved, err := resolveAPIKey(label, a.APIKey)
	if err != nil {
		return err
	}
	a.APIKey = resolved
	if a.PathPrefix == "" {
		return fmt.Errorf("%s: path_prefix is required", label)
	}
	if !strings.HasPrefix(a.PathPrefix, "/") {
		return fmt.Errorf("%s: path_prefix must start with /", label)
	}
	a.PathPrefix = strings.TrimRight(a.PathPrefix, "/")
	if a.Port != "" && a.Host == "" {
		return fmt.Errorf("%s: port requires host", label)
	}
	if a.Host != "" && a.Port == "" {
		a.Port = "80"
	}
	return nil
}

// resolveAPIKey expands api_key when the whole value is ${ENV_VAR}.
// Non-matching values (including $FOO or prefix${FOO}) are returned unchanged.
func resolveAPIKey(label, value string) (string, error) {
	m := envAPIKeyRef.FindStringSubmatch(value)
	if m == nil {
		return value, nil
	}
	name := m[1]
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", fmt.Errorf("%s: environment variable %q is unset or empty", label, name)
	}
	return v, nil
}

// EndpointPath joins path_prefix and table name: /prefix/table.
func EndpointPath(pathPrefix, table string) string {
	return strings.TrimRight(pathPrefix, "/") + "/" + table
}

// RecordPath joins path_prefix, table and id: /prefix/table/123.
func RecordPath(pathPrefix, table string, id int) string {
	return fmt.Sprintf("%s/%d", EndpointPath(pathPrefix, table), id)
}
