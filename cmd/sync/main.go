package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/bobuild/bobuild-sync-objects/internal/config"
	"github.com/bobuild/bobuild-sync-objects/internal/objsync"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config")
	dryRun := flag.Bool("dry-run", false, "log create/update/delete actions without writing")
	job := flag.String("job", "", "limit sync to a single job name from config")
	source := flag.String("source", "", "limit sync to jobs whose source domain matches")
	target := flag.String("target", "", "limit sync to a single target domain (within selected jobs)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	stats, err := objsync.Run(cfg, objsync.Options{
		DryRun:       *dryRun,
		JobName:      *job,
		SourceDomain: *source,
		TargetDomain: *target,
	})
	if len(stats) > 0 {
		printSummary(stats)
	}
	if err != nil {
		log.Fatalf("%v", err)
	}
}

func printSummary(stats []objsync.Stats) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Summary:")
	for _, s := range stats {
		fmt.Fprintf(os.Stderr,
			"  [%s] %-20s → %-36s table=%-20s source=%d created=%d updated=%d deleted=%d skipped=%d errors=%d\n",
			s.Job, s.Source, s.Target, s.Table, s.SourceN, s.Created, s.Updated, s.Deleted, s.Skipped, s.Errors,
		)
	}
}
