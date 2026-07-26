package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatChoice struct {
	Message Message `json:"message"`
}

type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
}

type Provider interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type OpenRouter struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewOpenRouter(baseURL, model string) *OpenRouter {
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		key = os.Getenv("OPENROUTER_API_KEY")
	}
	if model == "" {
		model = "llama-3.3-70b-versatile"
	}
	return &OpenRouter{
		apiKey:  key,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (o *OpenRouter) ChatCompletion(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if req.Model == "" {
		req.Model = o.model
	}

	log.Printf("[llm] -> model=%s msgs=%d", req.Model, len(req.Messages))

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 10 * time.Second
			log.Printf("[llm] retry attempt %d/%d, waiting %v", attempt+1, 5, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ChatResponse{}, ctx.Err()
			}
		}

		resp, err := o.doRequest(ctx, req)
		if err != nil {
			// Check if it's a 429 rate limit
			if rateLimitErr, ok := err.(*rateLimitError); ok {
				waitTime := rateLimitErr.retryAfter
				// If rate limit resets in more than 5 minutes, fail fast
				// (no point blocking a worker for hours)
				if waitTime > 5*time.Minute {
					log.Printf("[llm] rate limit resets in %v — too long to wait, failing fast", waitTime.Round(time.Second))
					return ChatResponse{}, fmt.Errorf("rate limited (429): quota exhausted, resets at %s (in %v)",
						time.Now().Add(waitTime).Format(time.RFC3339),
						waitTime.Round(time.Second))
				}
				if waitTime > 0 {
					log.Printf("[llm] rate limited, waiting %v until reset", waitTime.Round(time.Second))
					select {
					case <-time.After(waitTime + 2*time.Second):
					case <-ctx.Done():
						return ChatResponse{}, ctx.Err()
					}
				}
				lastErr = fmt.Errorf("rate limited (429): %v", err)
				continue
			}
			lastErr = err
			continue
		}
		if len(resp.Choices) > 0 {
			return resp, nil
		}
		log.Printf("[llm] empty choices on attempt %d", attempt+1)
		lastErr = fmt.Errorf("empty choices after all attempts")
	}
	return ChatResponse{}, lastErr
}

type rateLimitError struct {
	retryAfter time.Duration
	msg        string
}

func (e *rateLimitError) Error() string { return e.msg }

func (o *OpenRouter) doRequest(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	start := time.Now()
	resp, err := o.client.Do(httpReq)
	if err != nil {
		log.Printf("[llm] ERROR %v (%v)", err, time.Since(start))
		return ChatResponse{}, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		// Parse the retry-after header or X-RateLimit-Reset
		var retryAfter time.Duration
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if resetMs, err := strconv.ParseInt(reset, 10, 64); err == nil {
				resetTime := time.UnixMilli(resetMs)
				retryAfter = time.Until(resetTime)
				log.Printf("[llm] rate limit resets at %v (in %v)", resetTime.Format(time.RFC3339), retryAfter)
			}
		}
		if retryAfter <= 0 {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.ParseInt(ra, 10, 64); err == nil {
					retryAfter = time.Duration(secs) * time.Second
				}
			}
		}
		if retryAfter <= 0 {
			retryAfter = 60 * time.Second
		}
		log.Printf("[llm] 429 rate limited, retry-after=%v", retryAfter)
		return ChatResponse{}, &rateLimitError{
			retryAfter: retryAfter,
			msg:        fmt.Sprintf("429: %s", truncate(string(respBody), 200)),
		}
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[llm] ERROR status=%d %s", resp.StatusCode, truncate(string(respBody), 300))
		return ChatResponse{}, fmt.Errorf("api %d: %s", resp.StatusCode, respBody)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		log.Printf("[llm] ERROR unmarshal: %s", truncate(string(respBody), 300))
		return ChatResponse{}, fmt.Errorf("unmarshal: %w", err)
	}

	log.Printf("[llm] <- %d choices in %v", len(chatResp.Choices), time.Since(start))
	return chatResp, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
