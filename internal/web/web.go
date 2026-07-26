package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var client = &http.Client{
	Timeout: 20 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// searchClient uses a shorter timeout so SearXNG instance failures don't block for 20s each.
var searchClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Search queries multiple sources and returns results.
// Order: Tavily first (reliable API), then DDG, SearXNG, Google as fallbacks.
func Search(query string) ([]SearchResult, error) {
	log.Printf("[web] searching: %s", query)

	// 1. Tavily API — reliable, returns structured results + content
	results, err := searchTavily(query)
	if err == nil && len(results) > 0 {
		log.Printf("[web] Tavily found %d results", len(results))
		return results, nil
	}
	log.Printf("[web] Tavily: %v (results=%d)", err, len(results))

	// 2. DuckDuckGo lite
	results, err = searchDDGLite(query)
	if err == nil && len(results) > 0 {
		log.Printf("[web] DDG found %d results", len(results))
		return results, nil
	}
	log.Printf("[web] DDG: %v (results=%d)", err, len(results))

	// 3. SearXNG instances (limit to 4 to avoid long timeouts)
	instances := []string{
		"https://search.sapti.me",
		"https://searx.tiekoetter.com",
		"https://search.bus-hit.me",
		"https://searx.be",
	}
	for _, base := range instances {
		results, err := searchSearXNG(base, query)
		if err != nil {
			continue
		}
		if len(results) > 0 {
			log.Printf("[web] SearXNG %s found %d results", base, len(results))
			return results, nil
		}
	}

	// 4. Google fallback
	results, err = searchGoogle(query)
	if err == nil && len(results) > 0 {
		log.Printf("[web] Google found %d results", len(results))
		return results, nil
	}

	return nil, fmt.Errorf("all search methods returned 0 results for %q", query)
}

func searchTavily(query string) ([]SearchResult, error) {
	apiKey := os.Getenv("TAVILY_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TAVILY_API_KEY not set")
	}

	body, err := json.Marshal(map[string]any{
		"query":        query,
		"api_key":      apiKey,
		"search_depth": "basic",
		"max_results":  10,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := searchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("tavily status %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var apiResp struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("tavily decode: %w", err)
	}

	var results []SearchResult
	for _, r := range apiResp.Results {
		if r.URL == "" || r.Title == "" {
			continue
		}
		results = append(results, SearchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     r.URL,
			Snippet: strings.TrimSpace(r.Content),
		})
	}
	return results, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func searchSearXNG(baseURL, query string) ([]SearchResult, error) {
	searchURL := baseURL + "/search?q=" + url.QueryEscape(query) + "&format=json&categories=general&language=en"

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := searchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited (429)")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var apiResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	var results []SearchResult
	for _, r := range apiResp.Results {
		if r.URL == "" || r.Title == "" {
			continue
		}
		results = append(results, SearchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     r.URL,
			Snippet: strings.TrimSpace(r.Content),
		})
		if len(results) >= 10 {
			break
		}
	}
	return results, nil
}

func searchGoogle(query string) ([]SearchResult, error) {
	u := "https://www.google.com/search?q=" + url.QueryEscape(query) + "&num=10&hl=en"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Google status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseGoogleResults(string(body)), nil
}

func parseGoogleResults(html string) []SearchResult {
	var results []SearchResult

	// Google wraps results in <div class="g"> blocks
	// Each has an <a href="..."> with the URL and title
	linkRe := regexp.MustCompile(`(?i)<a[^>]+href="/url\?q=([^&"]+)[^"]*"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?i)<div[^>]*class="[^"]*VwiC3b[^"]*"[^>]*>(.*?)</div>`)

	linkMatches := linkRe.FindAllStringSubmatch(html, -1)
	snippetMatches := snippetRe.FindAllStringSubmatch(html, -1)

	for i, m := range linkMatches {
		if len(results) >= 8 {
			break
		}
		rawURL := m[1]
		title := stripTags(strings.TrimSpace(m[2]))

		if rawURL == "" || title == "" || strings.Contains(rawURL, "google.com") {
			continue
		}
		// Decode Google redirect URLs
		if decoded, err := url.QueryUnescape(rawURL); err == nil {
			rawURL = decoded
		}
		snippet := ""
		if i < len(snippetMatches) {
			snippet = stripTags(strings.TrimSpace(snippetMatches[i][1]))
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     rawURL,
			Snippet: snippet,
		})
	}
	return results
}

func searchDDGLite(query string) ([]SearchResult, error) {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseDDGResults(string(body)), nil
}

func parseDDGResults(html string) []SearchResult {
	var results []SearchResult

	titleRe := regexp.MustCompile(`(?i)<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?i)<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)

	titleMatches := titleRe.FindAllStringSubmatch(html, -1)
	snippetMatches := snippetRe.FindAllStringSubmatch(html, -1)

	for i, m := range titleMatches {
		if len(results) >= 8 {
			break
		}
		rawURL := strings.TrimSpace(m[1])
		title := stripTags(strings.TrimSpace(m[2]))
		if rawURL == "" {
			continue
		}
		if idx := strings.Index(rawURL, "uddg="); idx != -1 {
			if decoded, err := url.QueryUnescape(rawURL[idx+5:]); err == nil {
				if ampIdx := strings.Index(decoded, "&"); ampIdx != -1 {
					decoded = decoded[:ampIdx]
				}
				rawURL = decoded
			}
		}
		snippet := ""
		if i < len(snippetMatches) {
			snippet = stripTags(strings.TrimSpace(snippetMatches[i][1]))
		}
		if title != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     rawURL,
				Snippet: snippet,
			})
		}
	}
	return results
}

func stripTags(s string) string {
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, " ")
	s = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&nbsp;", " ",
	).Replace(s)
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))
}

// Fetch downloads a URL and returns cleaned text.
func Fetch(rawURL string) (string, error) {
	log.Printf("[web] fetching: %s", rawURL)
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := searchClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: status %d", rawURL, resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "text/") {
		return "", fmt.Errorf("fetch %s: unsupported content type %s", rawURL, ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	text := extractReadableText(string(body))
	if len(text) > 10000 {
		text = text[:10000]
	}
	log.Printf("[web] fetched %s -> %d chars text", rawURL, len(text))
	return text, nil
}

func extractReadableText(html string) string {
	html = regexp.MustCompile(`(?i)<(script|style|nav|header|footer|aside)[^>]*>.*?</\1>`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")
	html = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&nbsp;", " ",
	).Replace(html)
	html = regexp.MustCompile(`[ \t]+`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")
	return strings.TrimSpace(html)
}
