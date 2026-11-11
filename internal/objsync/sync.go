package objsync

import (
	"fmt"
	"log"
	"net/http"

	bobuild "github.com/bobuild/bobuild-client-go"

	"github.com/bobuild/bobuild-sync-objects/internal/config"
)

type Options struct {
	DryRun       bool
	JobName      string // empty = all jobs
	SourceDomain string // empty = all sources (within selected jobs)
	TargetDomain string // empty = all targets (within selected jobs)
}

type Stats struct {
	Job     string
	Table   string
	Source  string
	Target  string
	SourceN int
	Created int
	Updated int
	Deleted int
	Skipped int
	Errors  int
}

func filterJobs(cfg *config.Config, opts Options) ([]config.Job, error) {
	jobs := cfg.Jobs
	if opts.JobName != "" {
		filtered := make([]config.Job, 0, 1)
		for _, j := range cfg.Jobs {
			if j.Name == opts.JobName {
				filtered = append(filtered, j)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("unknown job %q", opts.JobName)
		}
		jobs = filtered
	}

	out := make([]config.Job, 0, len(jobs))
	for _, job := range jobs {
		if opts.SourceDomain != "" && job.Source.Domain != opts.SourceDomain {
			continue
		}
		if opts.TargetDomain != "" {
			targets := make([]config.App, 0)
			for _, t := range job.Targets {
				if t.Domain == opts.TargetDomain {
					targets = append(targets, t)
				}
			}
			if len(targets) == 0 {
				continue
			}
			job.Targets = targets
		}
		out = append(out, job)
	}
	if len(out) == 0 {
		switch {
		case opts.SourceDomain != "" && opts.TargetDomain != "":
			return nil, fmt.Errorf("no job matched source %q and target %q", opts.SourceDomain, opts.TargetDomain)
		case opts.SourceDomain != "":
			return nil, fmt.Errorf("no job matched source %q", opts.SourceDomain)
		case opts.TargetDomain != "":
			return nil, fmt.Errorf("no job matched target %q", opts.TargetDomain)
		default:
			return nil, fmt.Errorf("no jobs to run")
		}
	}
	return out, nil
}

type tableSource struct {
	rows []map[string]any
	err  error
}

// Run reconciles every selected job: create missing, update changed, delete extras.
// Table order is create/update order (FK parents first); deletes run in reverse.
func Run(cfg *config.Config, opts Options) ([]Stats, error) {
	jobs, err := filterJobs(cfg, opts)
	if err != nil {
		return nil, err
	}

	maxFileBytes := cfg.MaxFileBytes()

	var allStats []Stats
	var hadErrors bool

	for _, job := range jobs {
		sourceClient := newClient(job.Source)
		fileClient := fileHTTPClient(job.Source.InsecureSkipVerify)
		sources := make(map[string]tableSource, len(job.Tables))
		for _, table := range job.Tables {
			path := config.EndpointPath(job.Source.PathPrefix, table.Name)
			log.Printf("[%s] loading source %s", job.Name, path)
			rows, err := bobuild.GetList[map[string]any](sourceClient, path)
			if err != nil {
				log.Printf("[%s] ERROR GET source %s: %v", job.Name, path, err)
				sources[table.Name] = tableSource{err: err}
				continue
			}
			log.Printf("[%s] source %s: %d records", job.Name, table.Name, len(rows))
			sources[table.Name] = tableSource{rows: rows}
		}

		for _, target := range job.Targets {
			statsByTable := make(map[string]*Stats, len(job.Tables))
			for _, table := range job.Tables {
				st := &Stats{
					Job:    job.Name,
					Table:  table.Name,
					Source: job.Source.Domain,
					Target: target.Domain,
				}
				statsByTable[table.Name] = st

				src := sources[table.Name]
				if src.err != nil {
					st.Errors++
					hadErrors = true
					continue
				}
				st.SourceN = len(src.rows)
				if errN := upsertTable(job, table, src.rows, target, st, opts.DryRun, maxFileBytes, fileClient); errN > 0 {
					hadErrors = true
				}
			}

			for i := len(job.Tables) - 1; i >= 0; i-- {
				table := job.Tables[i]
				st := statsByTable[table.Name]
				src := sources[table.Name]
				if src.err != nil {
					continue
				}
				if errN := deleteExtras(job, table.Name, src.rows, target, st, opts.DryRun); errN > 0 {
					hadErrors = true
				}
			}

			for _, table := range job.Tables {
				allStats = append(allStats, *statsByTable[table.Name])
			}
		}
	}

	if hadErrors {
		return allStats, fmt.Errorf("sync completed with errors")
	}
	return allStats, nil
}

func upsertTable(job config.Job, table config.Table, sourceRows []map[string]any, target config.App, st *Stats, dryRun bool, maxFileBytes int64, fileClient *http.Client) int {
	errors := 0
	client := newClient(target)
	targetPath := config.EndpointPath(target.PathPrefix, table.Name)
	log.Printf("[%s→%s] loading target %s", job.Source.Domain, target.Domain, targetPath)
	targetRows, err := bobuild.GetList[map[string]any](client, targetPath)
	if err != nil {
		st.Errors++
		log.Printf("[%s→%s] ERROR GET %s: %v", job.Source.Domain, target.Domain, targetPath, err)
		return 1
	}

	targetByID, _ := indexByID(targetRows)
	sourceByID, invalid := indexByID(sourceRows)
	for range invalid {
		st.Errors++
		errors++
		log.Printf("[%s→%s] %s: source record without id, skipping", job.Source.Domain, target.Domain, table.Name)
	}

	for id, row := range sourceByID {
		existing, found := targetByID[id]
		if !found {
			if dryRun {
				log.Printf("[%s→%s] dry-run: would create %s id=%d", job.Source.Domain, target.Domain, table.Name, id)
				for _, field := range table.FileFields {
					if u, err := fileFieldURL(row[field]); err == nil && u != "" {
						log.Printf("[%s→%s] dry-run: would upload %s id=%d field=%s", job.Source.Domain, target.Domain, table.Name, id, field)
					}
				}
				st.Created++
				continue
			}
			if err := writeRecordWithFiles(job, table, target, client, fileClient, targetPath, id, row, true, maxFileBytes); err != nil {
				st.Errors++
				errors++
				log.Printf("[%s→%s] ERROR create %s id=%d: %v", job.Source.Domain, target.Domain, table.Name, id, err)
				continue
			}
			st.Created++
			log.Printf("[%s→%s] created %s id=%d", job.Source.Domain, target.Domain, table.Name, id)
			continue
		}
		if recordsEqualIgnoring(row, existing, table.FileFields) {
			st.Skipped++
			continue
		}
		recordPath := config.RecordPath(target.PathPrefix, table.Name, id)
		if dryRun {
			log.Printf("[%s→%s] dry-run: would update %s id=%d", job.Source.Domain, target.Domain, table.Name, id)
			for _, field := range table.FileFields {
				if u, err := fileFieldURL(row[field]); err == nil && u != "" {
					log.Printf("[%s→%s] dry-run: would upload %s id=%d field=%s", job.Source.Domain, target.Domain, table.Name, id, field)
				}
			}
			st.Updated++
			continue
		}
		if err := writeRecordWithFiles(job, table, target, client, fileClient, recordPath, id, row, false, maxFileBytes); err != nil {
			st.Errors++
			errors++
			log.Printf("[%s→%s] ERROR update %s id=%d: %v", job.Source.Domain, target.Domain, table.Name, id, err)
			continue
		}
		st.Updated++
		log.Printf("[%s→%s] updated %s id=%d", job.Source.Domain, target.Domain, table.Name, id)
	}
	return errors
}

// writeRecordWithFiles writes non-file fields first, then downloads and PATCHes
// each file field one at a time. create=true uses POST to listPath; otherwise
// listPath is the record Modify path. On oversize preflight, skips the record
// entirely (no meta write).
func writeRecordWithFiles(job config.Job, table config.Table, target config.App, apiClient *bobuild.Client, fileClient *http.Client, listOrRecordPath string, id int, row map[string]any, create bool, maxFileBytes int64) error {
	if err := preflightFileSizes(fileClient, row, table.FileFields, maxFileBytes); err != nil {
		return err
	}

	meta := omitKeys(row, table.FileFields...)
	if create {
		if err := createRecord(apiClient, listOrRecordPath, meta); err != nil {
			return err
		}
	} else {
		if err := modifyRecord(apiClient, listOrRecordPath, meta); err != nil {
			return err
		}
	}

	if len(table.FileFields) == 0 {
		return nil
	}

	recordPath := listOrRecordPath
	if create {
		recordPath = config.RecordPath(target.PathPrefix, table.Name, id)
	}

	for _, field := range table.FileFields {
		rawURL, err := fileFieldURL(row[field])
		if err != nil {
			return fmt.Errorf("field %q: %w", field, err)
		}
		if rawURL == "" {
			continue
		}
		payload, err := downloadFilePayload(fileClient, field, rawURL, maxFileBytes)
		if err != nil {
			return err
		}
		if err := modifyRecord(apiClient, recordPath, map[string]any{field: payload}); err != nil {
			return fmt.Errorf("upload field %q: %w", field, err)
		}
		log.Printf("[%s→%s] uploaded %s id=%d field=%s", job.Source.Domain, target.Domain, table.Name, id, field)
		payload = nil // help GC before next file
	}
	return nil
}

func deleteExtras(job config.Job, table string, sourceRows []map[string]any, target config.App, st *Stats, dryRun bool) int {
	errors := 0
	client := newClient(target)
	targetPath := config.EndpointPath(target.PathPrefix, table)
	targetRows, err := bobuild.GetList[map[string]any](client, targetPath)
	if err != nil {
		st.Errors++
		log.Printf("[%s→%s] ERROR GET %s (delete pass): %v", job.Source.Domain, target.Domain, targetPath, err)
		return 1
	}

	sourceByID, _ := indexByID(sourceRows)
	targetByID, _ := indexByID(targetRows)
	for id := range targetByID {
		if _, ok := sourceByID[id]; ok {
			continue
		}
		recordPath := config.RecordPath(target.PathPrefix, table, id)
		if dryRun {
			log.Printf("[%s→%s] dry-run: would delete %s id=%d", job.Source.Domain, target.Domain, table, id)
			st.Deleted++
			continue
		}
		if err := deleteRecord(client, recordPath); err != nil {
			st.Errors++
			errors++
			log.Printf("[%s→%s] ERROR delete %s id=%d: %v", job.Source.Domain, target.Domain, table, id, err)
			continue
		}
		st.Deleted++
		log.Printf("[%s→%s] deleted %s id=%d", job.Source.Domain, target.Domain, table, id)
	}
	return errors
}
