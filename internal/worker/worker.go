package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"text/template"
	"time"

	"scaff/internal/db"
	"scaff/internal/prompts"
	"scaff/internal/provider"
	"scaff/internal/web"
)

type AgentJob struct {
	JobID   string
	RunID   string
	Role    string
	Topic   db.Topic
	Handler func(ctx context.Context, aj AgentJob) error
}

func (aj AgentJob) Process(ctx context.Context) error {
	if aj.Handler == nil {
		return fmt.Errorf("agent job %s has no handler", aj.JobID)
	}
	return aj.Handler(ctx, aj)
}

type WorkerPool struct {
	store *db.Store
	llm   provider.Provider
}

func NewWorkerPool(store *db.Store, llm provider.Provider) *WorkerPool {
	return &WorkerPool{store: store, llm: llm}
}

func (wp *WorkerPool) HandleAgentJob(ctx context.Context, aj AgentJob) error {
	log.Printf("[worker] job %s start role=%s run=%s topic=%q", aj.JobID, aj.Role, aj.RunID, aj.Topic.Name)

	// Guard: skip if run is already failed/completed (gokue retries)
	run, _ := wp.store.RunByID(ctx, aj.RunID)
	if run.Status == "failed" || run.Status == "completed" {
		log.Printf("[worker] run %s already %s, skipping job %s", aj.RunID, run.Status, aj.JobID)
		return nil
	}

	// Guard: skip if this specific job is already completed/failed (query DB, not stale struct)
	job, err := wp.store.GetAgentJob(ctx, aj.JobID)
	if err == nil && (job.Status == "completed" || job.Status == "failed") {
		log.Printf("[worker] job %s already %s in DB, skipping", aj.JobID, job.Status)
		return nil
	}

	if err := wp.store.UpdateRunStatus(ctx, aj.RunID, "running"); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}
	if err := wp.store.UpdateAgentJobResult(ctx, aj.JobID, "running", nil); err != nil {
		return fmt.Errorf("update job status: %w", err)
	}

	// Step 1: Gather real web content
	var webContent string
	switch aj.Role {
	case "general_search":
		webContent = wp.searchAndFetch(aj.Topic.Name)
	case "source_specific":
		if len(aj.Topic.Sources) > 0 {
			webContent = wp.fetchSources(aj.Topic.Sources)
		} else {
			log.Printf("[worker] source_specific has no sources, falling back to search for job %s", aj.JobID)
			webContent = wp.searchAndFetch(aj.Topic.Name)
		}
	}

	if webContent == "" {
		webContent = "(no web content could be retrieved)"
	}

	// Step 2: Build prompt with real content and send to LLM
	systemPrompt, err := wp.buildPrompt(aj)
	if err != nil {
		wp.markJobFailedAndCheckCompletion(ctx, aj)
		return err
	}

	userMsg := fmt.Sprintf("Here is the raw web content I gathered:\n\n%s\n\nNow extract findings and return a JSON array.", webContent)

	log.Printf("[worker] calling LLM for job %s (role=%s, content=%d chars)", aj.JobID, aj.Role, len(webContent))
	resp, err := wp.llm.ChatCompletion(ctx, provider.ChatRequest{
		Messages: []provider.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
	})
	if err != nil {
		log.Printf("[worker] LLM error job %s: %v", aj.JobID, err)
		wp.markJobFailedAndCheckCompletion(ctx, aj)
		return fmt.Errorf("llm call: %w", err)
	}
	if len(resp.Choices) == 0 {
		wp.markJobFailedAndCheckCompletion(ctx, aj)
		return fmt.Errorf("llm no choices")
	}

	raw := resp.Choices[0].Message.Content
	resultBytes := []byte(raw)
	if cleaned := extractJSON(raw); cleaned != nil {
		resultBytes = cleaned
	}

	if err := wp.store.UpdateAgentJobResult(ctx, aj.JobID, "completed", resultBytes); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	log.Printf("[worker] job %s completed", aj.JobID)

	wp.checkAndMerge(ctx, aj.RunID)
	return nil
}

// markJobFailedAndCheckCompletion marks a job as failed, then checks if all jobs
// in the run are terminal. If so, marks the run as failed (since this job failed).
// Uses a fresh context to avoid failures when the job's context is already expired.
func (wp *WorkerPool) markJobFailedAndCheckCompletion(ctx context.Context, aj AgentJob) {
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = wp.store.UpdateAgentJobResult(dbCtx, aj.JobID, "failed", nil)
	allDone, err := wp.store.AllJobsCompleted(dbCtx, aj.RunID)
	if err != nil {
		log.Printf("[worker] check completion after fail: %v", err)
		return
	}
	if allDone {
		log.Printf("[worker] run %s all jobs terminal (with failures), marking as failed", aj.RunID)
		_ = wp.store.UpdateRunStatus(dbCtx, aj.RunID, "failed")
	}
}

// checkAndMerge checks if all jobs are done and triggers merge or marks run failed.
func (wp *WorkerPool) checkAndMerge(ctx context.Context, runID string) {
	allDone, err := wp.store.AllJobsCompleted(ctx, runID)
	if err != nil {
		log.Printf("[worker] check completion: %v", err)
		return
	}
	if !allDone {
		return
	}

	run, _ := wp.store.RunByID(ctx, runID)
	if run.Status == "completed" || run.Status == "failed" {
		log.Printf("[worker] run %s already %s, skipping merge", runID, run.Status)
		return
	}

	failed, _ := wp.store.AnyJobsFailed(ctx, runID)
	if failed {
		log.Printf("[worker] run %s has failed jobs, marking as failed", runID)
		_ = wp.store.UpdateRunStatus(ctx, runID, "failed")
		return
	}

	log.Printf("[worker] all jobs done for run %s, merging", runID)
	if err := wp.merge(ctx, runID); err != nil {
		log.Printf("[worker] merge failed for run %s: %v", runID, err)
	}
}

func (wp *WorkerPool) searchAndFetch(topicName string) string {
	query := topicName + " 2025 2026"

	results, err := web.Search(query)
	if err != nil {
		log.Printf("[web] search error for %q: %v", query, err)
		results, err = web.Search(topicName)
		if err != nil {
			log.Printf("[web] fallback search also failed: %v", err)
			return ""
		}
	}
	log.Printf("[web] got %d search results for %q", len(results), query)

	if len(results) == 0 {
		log.Printf("[web] WARNING: zero search results for %q", query)
		return ""
	}

	var parts []string
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("SEARCH RESULT: %s\nURL: %s\nSnippet: %s", r.Title, r.URL, r.Snippet))
	}

	log.Printf("[web] gathered %d search result snippets", len(parts))
	return strings.Join(parts, "\n\n---\n\n")
}

func (wp *WorkerPool) fetchSources(sources []db.Source) string {
	var parts []string
	for _, src := range sources {
		text, err := web.Fetch(src.Value)
		if err != nil {
			log.Printf("[web] fetch error %s: %v", src.Value, err)
			parts = append(parts, fmt.Sprintf("[error fetching %s: %v]", src.Value, err))
			continue
		}
		parts = append(parts, fmt.Sprintf("SOURCE: %s\n%s", src.Value, text))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func (wp *WorkerPool) buildPrompt(aj AgentJob) (string, error) {
	switch aj.Role {
	case "general_search":
		return renderTemplate(prompts.GeneralSearchTmpl, map[string]any{
			"TopicName": aj.Topic.Name,
		})
	case "source_specific":
		return renderTemplate(prompts.SourceSpecificTmpl, map[string]any{
			"TopicName": aj.Topic.Name,
			"Sources":   aj.Topic.Sources,
		})
	default:
		return "", fmt.Errorf("unknown role: %s", aj.Role)
	}
}

func (wp *WorkerPool) merge(ctx context.Context, runID string) error {
	log.Printf("[merge] starting for run %s", runID)

	results, err := wp.store.JobResults(ctx, runID)
	if err != nil {
		return fmt.Errorf("fetch results: %w", err)
	}
	run, err := wp.store.RunByID(ctx, runID)
	if err != nil {
		return fmt.Errorf("fetch run: %w", err)
	}
	topic, err := wp.store.GetTopic(ctx, run.TopicID)
	if err != nil {
		return fmt.Errorf("fetch topic: %w", err)
	}

	type entry struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var entries []entry
	for _, r := range results {
		entries = append(entries, entry{Role: r.Role, Content: string(r.Result)})
	}
	log.Printf("[merge] %d agent results for %q", len(entries), topic.Name)

	mergePrompt, err := renderTemplate(prompts.MergeTmpl, map[string]any{
		"TopicName": topic.Name,
		"Results":   entries,
	})
	if err != nil {
		return fmt.Errorf("render merge prompt: %w", err)
	}

	log.Printf("[merge] calling LLM for run %s", runID)
	resp, err := wp.llm.ChatCompletion(ctx, provider.ChatRequest{
		Messages: []provider.Message{
			{Role: "system", Content: mergePrompt},
			{Role: "user", Content: "Return a single JSON array. No text outside the JSON."},
		},
	})
	if err != nil {
		log.Printf("[merge] LLM error: %v", err)
		_ = wp.store.UpdateRunStatus(ctx, runID, "failed")
		return fmt.Errorf("merge llm call: %w", err)
	}
	if len(resp.Choices) == 0 {
		_ = wp.store.UpdateRunStatus(ctx, runID, "failed")
		return fmt.Errorf("merge llm no choices")
	}

	raw := resp.Choices[0].Message.Content
	mergedBytes := []byte(raw)
	if cleaned := extractJSON(raw); cleaned != nil {
		mergedBytes = cleaned
	}

	if err := wp.store.UpsertRunResult(ctx, runID, mergedBytes); err != nil {
		return fmt.Errorf("store result: %w", err)
	}
	if err := wp.store.UpdateRunStatus(ctx, runID, "completed"); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	log.Printf("[merge] done for run %s", runID)
	return nil
}

func extractJSON(s string) []byte {
	s = strings.TrimSpace(s)
	if json.Valid([]byte(s)) {
		return []byte(s)
	}
	if idx := strings.Index(s, "```json"); idx != -1 {
		s = s[idx+7:]
		if end := strings.Index(s, "```"); end != -1 {
			s = strings.TrimSpace(s[:end])
			if json.Valid([]byte(s)) {
				return []byte(s)
			}
		}
	}
	start := -1
	for i, c := range s {
		if c == '[' || c == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return nil
	}
	end := -1
	for i := len(s) - 1; i >= start; i-- {
		if s[i] == ']' || s[i] == '}' {
			end = i
			break
		}
	}
	if end <= start {
		return nil
	}
	candidate := s[start : end+1]
	if json.Valid([]byte(candidate)) {
		return []byte(candidate)
	}
	return nil
}

func renderTemplate(tmplStr string, data any) (string, error) {
	t, err := template.New("").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf []byte
	bw := &byteWriter{buf: buf}
	if err := t.Execute(bw, data); err != nil {
		return "", err
	}
	return string(bw.buf), nil
}

type byteWriter struct {
	buf []byte
}

func (w *byteWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}
