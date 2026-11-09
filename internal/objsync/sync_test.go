package objsync

import (
	"testing"

	"github.com/bobuild/bobuild-sync-objects/internal/config"
)

func sampleConfig() *config.Config {
	return &config.Config{
		Jobs: []config.Job{
			{
				Name:   "catalog",
				Tables: []string{"category", "product"},
				Source: config.App{Domain: "source.example.com", APIKey: "s", PathPrefix: "/sync"},
				Targets: []config.App{
					{Domain: "a.example.com", APIKey: "a", PathPrefix: "/sync"},
					{Domain: "b.example.com", APIKey: "b", PathPrefix: "/replica"},
				},
			},
			{
				Name:   "users",
				Tables: []string{"user"},
				Source: config.App{Domain: "hr.example.com", APIKey: "h", PathPrefix: "/sync"},
				Targets: []config.App{
					{Domain: "a.example.com", APIKey: "a2", PathPrefix: "/hr"},
				},
			},
		},
	}
}

func TestFilterJobs(t *testing.T) {
	cfg := sampleConfig()

	jobs, err := filterJobs(cfg, Options{})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("all: %v %d", err, len(jobs))
	}

	jobs, err = filterJobs(cfg, Options{JobName: "users"})
	if err != nil || len(jobs) != 1 || jobs[0].Name != "users" {
		t.Fatalf("job: %v %+v", err, jobs)
	}

	jobs, err = filterJobs(cfg, Options{SourceDomain: "source.example.com", TargetDomain: "b.example.com"})
	if err != nil || len(jobs) != 1 || len(jobs[0].Targets) != 1 || jobs[0].Targets[0].Domain != "b.example.com" {
		t.Fatalf("source+target: %v %+v", err, jobs)
	}

	if _, err := filterJobs(cfg, Options{JobName: "missing"}); err == nil {
		t.Fatal("expected unknown job error")
	}
	if _, err := filterJobs(cfg, Options{TargetDomain: "missing.example.com"}); err == nil {
		t.Fatal("expected unknown target error")
	}
}

func TestEndpointHelpersUsedBySync(t *testing.T) {
	if config.EndpointPath("/sync", "product") != "/sync/product" {
		t.Fatal("endpoint")
	}
	if config.RecordPath("/sync", "product", 9) != "/sync/product/9" {
		t.Fatal("record")
	}
}
