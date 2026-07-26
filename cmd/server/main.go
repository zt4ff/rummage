package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zt4ff/gokue"

	"scaff/internal/db"
	"scaff/internal/prompts"
	"scaff/internal/provider"
	schedpkg "scaff/internal/scheduler"
	workerpkg "scaff/internal/worker"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://localhost:5432/scaff?sslmode=disable"
	}
	log.Printf("[server] connecting to database")
	store, err := db.New(ctx, connStr)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer store.Close()
	log.Printf("[server] database connected")

	llm := provider.NewOpenRouter(
		"https://api.groq.com/openai/v1/chat/completions",
		os.Getenv("MODEL"),
	)
	log.Printf("[server] LLM provider ready (model=%s, key_set=%v)", os.Getenv("MODEL"), os.Getenv("GROQ_API_KEY") != "")

	queue, err := gokue.NewQueue(
		gokue.WithWorkerCount(8),
		gokue.WithQueueSize(256),
		gokue.WithMaxRetries(2),
		gokue.WithJobTimeout(180*time.Second),
		gokue.WithRetryDelay(5*time.Second),
	)
	if err != nil {
		log.Fatalf("queue: %v", err)
	}

	if err := queue.RegisterJob("agent_job"); err != nil {
		log.Fatalf("register job: %v", err)
	}
	log.Printf("[server] gokue queue ready (workers=8, timeout=180s)")

	wpool := workerpkg.NewWorkerPool(store, llm)

	sched := schedpkg.New(store, queue, wpool)

	go sched.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/topics", handleListTopics(store))
	mux.HandleFunc("GET /api/topics/{id}", handleGetTopic(store))
	mux.HandleFunc("DELETE /api/topics/{id}", handleDeleteTopic(store))
	mux.HandleFunc("POST /api/topics", handleCreateTopic(store))
	mux.HandleFunc("POST /api/topics/{id}/run-now", handleRunNow(sched, store))
	mux.HandleFunc("GET /api/topics/{id}/runs", handleTopicRuns(store))
	mux.HandleFunc("GET /api/runs/{id}", handleRunWithResult(store))
	mux.HandleFunc("GET /api/prompts/{name}", handleGetPrompt)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		log.Printf("[server] listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("[server] shutting down...")

	queueCtx, queueCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer queueCancel()
	_ = queue.Close(queueCtx)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleListTopics(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[http] GET /api/topics")
		topics, err := store.ListTopics(r.Context())
		if err != nil {
			log.Printf("[http] ERROR listing topics: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if topics == nil {
			topics = []db.Topic{}
		}
		log.Printf("[http] returning %d topics", len(topics))
		jsonResponse(w, topics)
	}
}

func handleGetTopic(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("[http] GET /api/topics/%s", id)
		topic, err := store.GetTopic(r.Context(), id)
		if err != nil {
			log.Printf("[http] ERROR getting topic %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, topic)
	}
}

func handleCreateTopic(store *db.Store) http.HandlerFunc {
	type createReq struct {
		Name       string      `json:"name"`
		Schedule   string      `json:"schedule"`
		Sources    []db.Source `json:"sources"`
		AgentCount int         `json:"agent_count"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.AgentCount <= 0 {
			req.AgentCount = 3
		}
		log.Printf("[http] POST /api/topics name=%q schedule=%q sources=%d agents=%d", req.Name, req.Schedule, len(req.Sources), req.AgentCount)
		topic, err := store.CreateTopic(r.Context(), req.Name, req.Schedule, req.Sources, req.AgentCount)
		if err != nil {
			log.Printf("[http] ERROR creating topic: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[http] created topic %s (%q)", topic.ID, topic.Name)
		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, topic)
	}
}

func handleDeleteTopic(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("[http] DELETE /api/topics/%s", id)

		cancelled, err := store.CancelRunningRuns(r.Context(), id)
		if err != nil {
			log.Printf("[http] ERROR cancelling runs: %v", err)
		} else {
			log.Printf("[http] cancelled %d running/pending runs for topic %s", cancelled, id)
		}

		if err := store.DeleteTopic(r.Context(), id); err != nil {
			log.Printf("[http] ERROR deleting topic %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[http] deleted topic %s", id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRunNow(sched *schedpkg.Scheduler, store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("[http] POST /api/topics/%s/run-now", id)
		topic, err := store.GetTopic(r.Context(), id)
		if err != nil {
			log.Printf("[http] ERROR topic %s not found: %v", id, err)
			http.Error(w, "topic not found", http.StatusNotFound)
			return
		}
		runID, err := sched.RunTopicNow(r.Context(), topic)
		if err != nil {
			log.Printf("[http] ERROR running topic %s now: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("[http] started immediate run %s for topic %q", runID, topic.Name)
		jsonResponse(w, map[string]string{"run_id": runID, "status": "dispatched"})
	}
}

func handleTopicRuns(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("[http] GET /api/topics/%s/runs", id)
		runs, err := store.TopicRuns(r.Context(), id)
		if err != nil {
			log.Printf("[http] ERROR listing runs for topic %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if runs == nil {
			runs = []db.Run{}
		}
		log.Printf("[http] returning %d runs for topic %s", len(runs), id)
		jsonResponse(w, runs)
	}
}

func handleRunWithResult(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		log.Printf("[http] GET /api/runs/%s", id)
		run, result, jobs, err := store.RunWithResult(r.Context(), id)
		if err != nil {
			log.Printf("[http] ERROR getting run %s: %v", id, err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		resp := map[string]any{
			"run":    run,
			"jobs":   jobs,
			"result": result,
		}
		log.Printf("[http] returning run %s (status=%s, jobs=%d, has_result=%v)", id, run.Status, len(jobs), result != nil)
		jsonResponse(w, resp)
	}
}

func handleGetPrompt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var content string
	switch name {
	case "general_search":
		content = prompts.GeneralSearchTmpl
	case "source_specific":
		content = prompts.SourceSpecificTmpl
	case "merge":
		content = prompts.MergeTmpl
	default:
		http.Error(w, "unknown prompt", http.StatusNotFound)
		return
	}
	jsonResponse(w, map[string]string{"name": name, "content": content})
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}
