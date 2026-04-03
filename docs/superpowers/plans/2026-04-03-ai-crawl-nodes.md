# AI Crawl Nodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement two new workflow nodes (`ai.read_page`, `ai.extract_page`) that crawl web pages and return clean, token-efficient content for AI consumption.

**Architecture:** Shared content engine (`internal/nodes/ai/crawl/engine.go`) with three pipeline stages: Fetch (static/browser/auto) → Clean (goquery DOM stripping) → ToMarkdown (structured output). Two focused node executors consume this engine. No new Go dependencies.

**Tech Stack:** Go, goquery (existing), Rod (existing), monoes config API (existing), standard `net/http`.

---

### Task 1: Shared Engine — Types & Fetch

**Files:**
- Create: `internal/nodes/ai/crawl/engine.go`

- [ ] **Step 1: Create the package and types**

```go
package crawl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// FetchOptions controls how FetchPage retrieves content.
type FetchOptions struct {
	RenderMode   string // "auto", "static", "browser"
	WaitSelector string // CSS selector to wait for (browser mode)
	Timeout      time.Duration
}

// PageContent holds the structured extraction result.
type PageContent struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Author      string     `json:"author,omitempty"`
	PublishedAt string     `json:"published_at,omitempty"`
	URL         string     `json:"url"`
	Favicon     string     `json:"favicon,omitempty"`
	MainText    string     `json:"main_text"`
	Markdown    string     `json:"markdown"`
	Links       []Link     `json:"links,omitempty"`
	Images      []Image    `json:"images,omitempty"`
	Headings    []Heading  `json:"headings,omitempty"`
	Tables      [][]string `json:"tables,omitempty"`
	TokenCount  int        `json:"token_count"`
}

type Link struct {
	Text       string `json:"text"`
	URL        string `json:"url"`
	IsExternal bool   `json:"external"`
}

type Image struct {
	Alt    string `json:"alt"`
	Src    string `json:"src"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// FetchResult is the raw result from fetching a page.
type FetchResult struct {
	HTML     string
	FinalURL string
	Mode     string // which mode was actually used
	Duration time.Duration
}

var httpClient = &http.Client{Timeout: 30 * time.Second}
```

- [ ] **Step 2: Implement FetchPage with static mode**

Add to `engine.go`:

```go
// FetchPage retrieves a web page. In "auto" mode it tries static first,
// falling back to browser if the page has very little visible content.
func FetchPage(ctx context.Context, rawURL string, opts FetchOptions) (*FetchResult, error) {
	if opts.RenderMode == "" {
		opts.RenderMode = "auto"
	}

	switch opts.RenderMode {
	case "static":
		return fetchStatic(ctx, rawURL)
	case "browser":
		return fetchBrowser(ctx, rawURL, opts)
	case "auto":
		result, err := fetchStatic(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		// If the body has very little visible text, retry with browser
		doc, parseErr := goquery.NewDocumentFromReader(strings.NewReader(result.HTML))
		if parseErr == nil {
			bodyText := strings.TrimSpace(doc.Find("body").Text())
			if len(bodyText) < 200 {
				browserResult, browserErr := fetchBrowser(ctx, rawURL, opts)
				if browserErr == nil {
					browserResult.Mode = "browser (auto-fallback)"
					return browserResult, nil
				}
				// Browser failed, return the static result with a warning
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown render_mode: %q", opts.RenderMode)
	}
}

func fetchStatic(ctx context.Context, rawURL string) (*FetchResult, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MonoAgent/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return &FetchResult{
		HTML:     string(body),
		FinalURL: resp.Request.URL.String(),
		Mode:     "static",
		Duration: time.Since(start),
	}, nil
}
```

- [ ] **Step 3: Implement browser fetch with Rod**

Add to `engine.go`:

```go
func fetchBrowser(ctx context.Context, rawURL string, opts FetchOptions) (*FetchResult, error) {
	start := time.Now()
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	path, _ := launcher.LookPath()
	u := launcher.New().Bin(path).Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		// Browser unavailable — return error so caller can fall back
		return nil, fmt.Errorf("browser connect: %w", err)
	}
	defer browser.MustClose()

	page, err := browser.Page(proto.TargetCreateTarget{URL: rawURL})
	if err != nil {
		return nil, fmt.Errorf("browser navigate: %w", err)
	}

	// Wait for network idle or timeout
	_ = page.Timeout(timeout).WaitStable(500 * time.Millisecond)

	if opts.WaitSelector != "" {
		_ = page.Timeout(timeout).MustElement(opts.WaitSelector)
	}

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("browser get HTML: %w", err)
	}

	finalURL := page.MustInfo().URL

	return &FetchResult{
		HTML:     html,
		FinalURL: finalURL,
		Mode:     "browser",
		Duration: time.Since(start),
	}, nil
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./internal/nodes/ai/crawl/...`
Expected: PASS (no errors)

- [ ] **Step 5: Commit**

```bash
git add internal/nodes/ai/crawl/engine.go
git commit -m "feat(crawl): add shared engine — types and FetchPage (static/browser/auto)"
```

---

### Task 2: Shared Engine — CleanContent

**Files:**
- Modify: `internal/nodes/ai/crawl/engine.go`

- [ ] **Step 1: Implement CleanContent**

Add to `engine.go`:

```go
// CleanOptions controls content cleaning behavior.
type CleanOptions struct {
	KeepNav bool // keep <nav>, <header>, <footer>
}

// CleanContent strips non-visible elements and extracts structured content.
func CleanContent(rawHTML string, pageURL string, opts CleanOptions) (*PageContent, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	pc := &PageContent{URL: pageURL}

	// Extract metadata before cleaning
	pc.Title = strings.TrimSpace(doc.Find("title").First().Text())
	if pc.Title == "" {
		pc.Title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	pc.Description, _ = doc.Find(`meta[name="description"]`).Attr("content")
	if pc.Description == "" {
		pc.Description, _ = doc.Find(`meta[property="og:description"]`).Attr("content")
	}
	pc.Author, _ = doc.Find(`meta[name="author"]`).Attr("content")
	if pc.Author == "" {
		pc.Author, _ = doc.Find(`meta[property="article:author"]`).Attr("content")
	}
	pc.PublishedAt, _ = doc.Find(`meta[property="article:published_time"]`).Attr("content")
	if pc.PublishedAt == "" {
		pc.PublishedAt = strings.TrimSpace(doc.Find("time").First().AttrOr("datetime", ""))
	}
	pc.Favicon, _ = doc.Find(`link[rel="icon"], link[rel="shortcut icon"]`).First().Attr("href")
	if pc.Favicon != "" && !strings.HasPrefix(pc.Favicon, "http") {
		if base, err := url.Parse(pageURL); err == nil {
			if ref, err := url.Parse(pc.Favicon); err == nil {
				pc.Favicon = base.ResolveReference(ref).String()
			}
		}
	}

	canonical, _ := doc.Find(`link[rel="canonical"]`).Attr("href")
	if canonical != "" {
		pc.URL = canonical
	}

	// Remove non-visible elements
	doc.Find("script, style, noscript, svg, iframe, link, meta").Remove()
	doc.Find(`[style*="display:none"], [style*="display: none"]`).Remove()
	doc.Find(`[style*="visibility:hidden"], [style*="visibility: hidden"]`).Remove()
	doc.Find(`[aria-hidden="true"]`).Remove()

	if !opts.KeepNav {
		doc.Find("nav, header, footer").Remove()
	}

	// Extract links
	baseURL, _ := url.Parse(pageURL)
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		text := strings.TrimSpace(s.Text())
		if href == "" || text == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			return
		}
		linkURL := href
		isExternal := false
		if parsed, err := url.Parse(href); err == nil {
			if !parsed.IsAbs() && baseURL != nil {
				linkURL = baseURL.ResolveReference(parsed).String()
			}
			if parsed.Host != "" && baseURL != nil && parsed.Host != baseURL.Host {
				isExternal = true
			}
		}
		pc.Links = append(pc.Links, Link{Text: text, URL: linkURL, IsExternal: isExternal})
	})

	// Extract images
	doc.Find("img[src]").Each(func(_ int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		if src == "" {
			return
		}
		if !strings.HasPrefix(src, "http") && baseURL != nil {
			if ref, err := url.Parse(src); err == nil {
				src = baseURL.ResolveReference(ref).String()
			}
		}
		alt, _ := s.Attr("alt")
		img := Image{Alt: strings.TrimSpace(alt), Src: src}
		if w, exists := s.Attr("width"); exists {
			fmt.Sscanf(w, "%d", &img.Width)
		}
		if h, exists := s.Attr("height"); exists {
			fmt.Sscanf(h, "%d", &img.Height)
		}
		pc.Images = append(pc.Images, img)
	})

	// Extract headings
	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(_ int, s *goquery.Selection) {
		tag := goquery.NodeName(s)
		level := int(tag[1] - '0')
		text := strings.TrimSpace(s.Text())
		if text != "" {
			pc.Headings = append(pc.Headings, Heading{Level: level, Text: text})
		}
	})

	// Extract tables
	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		var rows [][]string
		table.Find("tr").Each(func(_ int, tr *goquery.Selection) {
			var row []string
			tr.Find("th, td").Each(func(_ int, cell *goquery.Selection) {
				row = append(row, strings.TrimSpace(cell.Text()))
			})
			if len(row) > 0 {
				rows = append(rows, row)
			}
		})
		if len(rows) > 0 {
			pc.Tables = append(pc.Tables, rows...)
		}
	})

	// Extract plain text
	pc.MainText = strings.TrimSpace(doc.Find("body").Text())
	// Collapse whitespace
	for strings.Contains(pc.MainText, "  ") {
		pc.MainText = strings.ReplaceAll(pc.MainText, "  ", " ")
	}
	for strings.Contains(pc.MainText, "\n\n\n") {
		pc.MainText = strings.ReplaceAll(pc.MainText, "\n\n\n", "\n\n")
	}

	return pc, nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/nodes/ai/crawl/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nodes/ai/crawl/engine.go
git commit -m "feat(crawl): add CleanContent — DOM stripping and structured extraction"
```

---

### Task 3: Shared Engine — ToMarkdown

**Files:**
- Modify: `internal/nodes/ai/crawl/engine.go`

- [ ] **Step 1: Implement ToMarkdown and token estimation**

Add to `engine.go`:

```go
// ToMarkdown converts a goquery document (after cleaning) into clean markdown.
// Call this after CleanContent has populated the PageContent fields.
func ToMarkdown(rawHTML string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return ""
	}
	// Remove non-visible before converting
	doc.Find("script, style, noscript, svg, iframe, link, meta").Remove()

	var buf strings.Builder
	convertNode(&buf, doc.Find("body"), 0)

	result := buf.String()
	// Collapse excessive blank lines
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

func convertNode(buf *strings.Builder, sel *goquery.Selection, depth int) {
	sel.Contents().Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "#text" {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				buf.WriteString(text)
			}
			return
		}

		tag := goquery.NodeName(s)
		switch tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(tag[1] - '0')
			buf.WriteString("\n\n")
			buf.WriteString(strings.Repeat("#", level))
			buf.WriteString(" ")
			buf.WriteString(strings.TrimSpace(s.Text()))
			buf.WriteString("\n\n")

		case "p", "div", "section", "article", "main":
			buf.WriteString("\n\n")
			convertNode(buf, s, depth)
			buf.WriteString("\n\n")

		case "br":
			buf.WriteString("\n")

		case "a":
			href, _ := s.Attr("href")
			text := strings.TrimSpace(s.Text())
			if text != "" && href != "" && !strings.HasPrefix(href, "javascript:") {
				fmt.Fprintf(buf, "[%s](%s)", text, href)
			} else if text != "" {
				buf.WriteString(text)
			}

		case "img":
			alt, _ := s.Attr("alt")
			src, _ := s.Attr("src")
			if src != "" {
				fmt.Fprintf(buf, "![%s](%s)", alt, src)
			}

		case "strong", "b":
			text := strings.TrimSpace(s.Text())
			if text != "" {
				fmt.Fprintf(buf, "**%s**", text)
			}

		case "em", "i":
			text := strings.TrimSpace(s.Text())
			if text != "" {
				fmt.Fprintf(buf, "*%s*", text)
			}

		case "code":
			// Check if inside <pre>
			if s.Parent().Is("pre") {
				return // handled by pre case
			}
			fmt.Fprintf(buf, "`%s`", strings.TrimSpace(s.Text()))

		case "pre":
			lang, _ := s.Find("code").Attr("class")
			lang = strings.TrimPrefix(lang, "language-")
			code := strings.TrimSpace(s.Text())
			fmt.Fprintf(buf, "\n\n```%s\n%s\n```\n\n", lang, code)

		case "blockquote":
			text := strings.TrimSpace(s.Text())
			lines := strings.Split(text, "\n")
			buf.WriteString("\n\n")
			for _, line := range lines {
				fmt.Fprintf(buf, "> %s\n", strings.TrimSpace(line))
			}
			buf.WriteString("\n")

		case "ul":
			buf.WriteString("\n")
			s.Children().Each(func(_ int, li *goquery.Selection) {
				text := strings.TrimSpace(li.Text())
				if text != "" {
					fmt.Fprintf(buf, "%s- %s\n", strings.Repeat("  ", depth), text)
				}
			})
			buf.WriteString("\n")

		case "ol":
			buf.WriteString("\n")
			s.Children().Each(func(i int, li *goquery.Selection) {
				text := strings.TrimSpace(li.Text())
				if text != "" {
					fmt.Fprintf(buf, "%s%d. %s\n", strings.Repeat("  ", depth), i+1, text)
				}
			})
			buf.WriteString("\n")

		case "table":
			buf.WriteString("\n\n")
			s.Find("tr").Each(func(i int, tr *goquery.Selection) {
				tr.Find("th, td").Each(func(j int, cell *goquery.Selection) {
					if j > 0 {
						buf.WriteString(" | ")
					}
					buf.WriteString(strings.TrimSpace(cell.Text()))
				})
				buf.WriteString("\n")
				// Add separator after header row
				if i == 0 {
					cols := tr.Find("th, td").Length()
					for c := 0; c < cols; c++ {
						if c > 0 {
							buf.WriteString(" | ")
						}
						buf.WriteString("---")
					}
					buf.WriteString("\n")
				}
			})
			buf.WriteString("\n")

		case "hr":
			buf.WriteString("\n\n---\n\n")

		default:
			// Recurse into unknown elements
			convertNode(buf, s, depth)
		}
	})
}

// EstimateTokens approximates token count at ~0.75 tokens per word.
func EstimateTokens(text string) int {
	words := len(strings.Fields(text))
	return int(float64(words) * 0.75)
}

// TruncateToTokens truncates markdown to approximately maxTokens.
func TruncateToTokens(markdown string, maxTokens int) string {
	if maxTokens <= 0 {
		return markdown
	}
	words := strings.Fields(markdown)
	maxWords := int(float64(maxTokens) / 0.75)
	if len(words) <= maxWords {
		return markdown
	}
	return strings.Join(words[:maxWords], " ") + "\n\n[... truncated]"
}
```

- [ ] **Step 2: Wire ToMarkdown into CleanContent**

Add these two lines at the end of `CleanContent`, before `return pc, nil`:

```go
	pc.Markdown = ToMarkdown(rawHTML)
	pc.TokenCount = EstimateTokens(pc.Markdown)
```

- [ ] **Step 3: Verify build**

Run: `go build ./internal/nodes/ai/crawl/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nodes/ai/crawl/engine.go
git commit -m "feat(crawl): add ToMarkdown converter and token estimation"
```

---

### Task 4: `ai.read_page` Node

**Files:**
- Create: `internal/nodes/ai/crawl/read_page.go`

- [ ] **Step 1: Implement the ReadPageNode**

```go
package crawl

import (
	"context"
	"fmt"
	"time"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// ReadPageNode crawls a web page and returns clean readable content.
// Type: "ai.read_page"
type ReadPageNode struct{}

func (n *ReadPageNode) Type() string { return "ai.read_page" }

func (n *ReadPageNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	// Resolve URL from config or upstream item
	rawURL, _ := config["url"].(string)
	if rawURL == "" && len(input.Items) > 0 {
		rawURL, _ = input.Items[0].JSON["url"].(string)
	}
	if rawURL == "" {
		return nil, fmt.Errorf("ai.read_page: 'url' is required")
	}

	renderMode, _ := config["render_mode"].(string)
	waitSelector, _ := config["wait_selector"].(string)
	keepNav, _ := config["keep_nav"].(bool)
	includeLinks := boolDefault(config, "include_links", true)
	includeImages := boolDefault(config, "include_images", true)
	includeTables := boolDefault(config, "include_tables", true)
	maxTokens := intVal(config, "max_tokens")

	// Fetch
	fetchResult, err := FetchPage(ctx, rawURL, FetchOptions{
		RenderMode:   renderMode,
		WaitSelector: waitSelector,
	})
	if err != nil {
		errItem := workflow.NewItem(map[string]interface{}{
			"url":   rawURL,
			"error": err.Error(),
		})
		return []workflow.NodeOutput{{Handle: "error", Items: []workflow.Item{errItem}}}, nil
	}

	// Clean
	content, err := CleanContent(fetchResult.HTML, fetchResult.FinalURL, CleanOptions{KeepNav: keepNav})
	if err != nil {
		return nil, fmt.Errorf("ai.read_page: clean: %w", err)
	}

	// Truncate if requested
	if maxTokens > 0 {
		content.Markdown = TruncateToTokens(content.Markdown, maxTokens)
		content.TokenCount = EstimateTokens(content.Markdown)
	}

	// Build output
	out := map[string]interface{}{
		"url":              content.URL,
		"title":            content.Title,
		"description":      content.Description,
		"author":           content.Author,
		"published_at":     content.PublishedAt,
		"markdown":         content.Markdown,
		"main_text":        content.MainText,
		"headings":         content.Headings,
		"token_count":      content.TokenCount,
		"render_mode_used": fetchResult.Mode,
		"fetch_time_ms":    fetchResult.Duration.Milliseconds(),
	}
	if includeLinks {
		out["links"] = content.Links
	}
	if includeImages {
		out["images"] = content.Images
	}
	if includeTables {
		out["tables"] = content.Tables
	}

	item := workflow.NewItem(out)
	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil
}

// helpers
func boolDefault(config map[string]interface{}, key string, defaultVal bool) bool {
	v, ok := config[key].(bool)
	if !ok {
		return defaultVal
	}
	return v
}

func intVal(config map[string]interface{}, key string) int {
	if v, ok := config[key].(float64); ok {
		return int(v)
	}
	return 0
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/nodes/ai/crawl/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nodes/ai/crawl/read_page.go
git commit -m "feat(crawl): add ai.read_page node executor"
```

---

### Task 5: `ai.extract_page` Node

**Files:**
- Create: `internal/nodes/ai/crawl/extract_page.go`

- [ ] **Step 1: Implement the ExtractPageNode**

```go
package crawl

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	cfgpkg "github.com/nokhodian/mono-agent/internal/config"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// ExtractPageNode crawls a page and extracts specific fields using AI or CSS selectors.
// Type: "ai.extract_page"
type ExtractPageNode struct {
	CfgClient *cfgpkg.APIClient // optional — needed for natural mode
}

func (n *ExtractPageNode) Type() string { return "ai.extract_page" }

func (n *ExtractPageNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	rawURL, _ := config["url"].(string)
	if rawURL == "" && len(input.Items) > 0 {
		rawURL, _ = input.Items[0].JSON["url"].(string)
	}
	if rawURL == "" {
		return nil, fmt.Errorf("ai.extract_page: 'url' is required")
	}

	extractMode, _ := config["extract_mode"].(string)
	if extractMode == "" {
		extractMode = "natural"
	}
	renderMode, _ := config["render_mode"].(string)
	waitSelector, _ := config["wait_selector"].(string)
	prompt, _ := config["prompt"].(string)
	fieldsJSON, _ := config["fields"].(string)
	listSelector, _ := config["list_selector"].(string)

	// Fetch
	fetchResult, err := FetchPage(ctx, rawURL, FetchOptions{
		RenderMode:   renderMode,
		WaitSelector: waitSelector,
	})
	if err != nil {
		errItem := workflow.NewItem(map[string]interface{}{
			"url":   rawURL,
			"error": err.Error(),
		})
		return []workflow.NodeOutput{{Handle: "error", Items: []workflow.Item{errItem}}}, nil
	}

	// Resolve selectors
	var selectors map[string]string
	switch extractMode {
	case "css":
		selectors, err = parseCSSFields(fieldsJSON)
		if err != nil {
			return nil, fmt.Errorf("ai.extract_page: parse fields: %w", err)
		}
	case "natural":
		if prompt == "" {
			return nil, fmt.Errorf("ai.extract_page: 'prompt' is required for natural extraction mode")
		}
		selectors, err = n.generateSelectorsFromAI(ctx, fetchResult.HTML, rawURL, prompt)
		if err != nil {
			// AI failed — return error but include cleaned markdown as fallback
			content, _ := CleanContent(fetchResult.HTML, fetchResult.FinalURL, CleanOptions{})
			errItem := workflow.NewItem(map[string]interface{}{
				"url":      rawURL,
				"error":    fmt.Sprintf("AI extraction failed: %v", err),
				"markdown": content.Markdown,
			})
			return []workflow.NodeOutput{{Handle: "error", Items: []workflow.Item{errItem}}}, nil
		}
	default:
		return nil, fmt.Errorf("ai.extract_page: unknown extract_mode %q", extractMode)
	}

	// Apply selectors
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fetchResult.HTML))
	if err != nil {
		return nil, fmt.Errorf("ai.extract_page: parse HTML: %w", err)
	}

	out := map[string]interface{}{
		"url":            fetchResult.FinalURL,
		"selectors_used": selectors,
		"extract_mode":   extractMode,
		"fetch_time_ms":  fetchResult.Duration.Milliseconds(),
	}

	if listSelector != "" {
		// List extraction
		var items []map[string]interface{}
		doc.Find(listSelector).Each(func(_ int, s *goquery.Selection) {
			item := applySelectors(s, selectors)
			if len(item) > 0 {
				items = append(items, item)
			}
		})
		out["extracted"] = items
		out["count"] = len(items)
		out["list_selector"] = listSelector
	} else {
		// Single item extraction
		extracted := applySelectors(doc.Selection, selectors)
		out["extracted"] = extracted
	}

	item := workflow.NewItem(out)
	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{item}}}, nil
}

func parseCSSFields(fieldsJSON string) (map[string]string, error) {
	if fieldsJSON == "" {
		return nil, fmt.Errorf("'fields' JSON is required for css mode")
	}
	var selectors map[string]string
	if err := json.Unmarshal([]byte(fieldsJSON), &selectors); err != nil {
		return nil, fmt.Errorf("parse fields JSON: %w", err)
	}
	if len(selectors) == 0 {
		return nil, fmt.Errorf("fields map is empty")
	}
	return selectors, nil
}

func applySelectors(scope *goquery.Selection, selectors map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for field, selector := range selectors {
		sel := scope.Find(selector)
		if sel.Length() == 0 {
			result[field] = ""
			continue
		}
		// If the selector has an attribute hint (e.g., "img.hero@src"), split it
		if parts := strings.SplitN(selector, "@", 2); len(parts) == 2 {
			val, _ := scope.Find(parts[0]).First().Attr(parts[1])
			result[field] = val
		} else {
			result[field] = strings.TrimSpace(sel.First().Text())
		}
	}
	return result
}

func (n *ExtractPageNode) generateSelectorsFromAI(ctx context.Context, html, pageURL, prompt string) (map[string]string, error) {
	if n.CfgClient == nil {
		return nil, fmt.Errorf("AI extraction requires the config API client (monoes_apis)")
	}

	// Generate a config name from the URL domain + prompt hash
	parsed, _ := url.Parse(pageURL)
	domain := "unknown"
	if parsed != nil {
		domain = parsed.Host
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))[:8]
	configName := fmt.Sprintf("extract_%s_%s", domain, hash)

	// Build a simple schema from the prompt (the API infers fields from the purpose string)
	schema := map[string]interface{}{
		"type": "object",
		"description": prompt,
	}

	result, err := n.CfgClient.GenerateConfig(ctx, configName, html, prompt, schema)
	if err != nil {
		return nil, err
	}

	// Parse the generated config into selectors
	selectors := make(map[string]string)
	if fields, ok := result["fields"].(map[string]interface{}); ok {
		for k, v := range fields {
			if sel, ok := v.(string); ok {
				selectors[k] = sel
			}
		}
	}
	// Also check for a flat selector map at the top level
	if len(selectors) == 0 {
		for k, v := range result {
			if sel, ok := v.(string); ok && strings.Contains(sel, ".") || strings.Contains(sel, "#") || strings.Contains(sel, "[") {
				selectors[k] = sel
			}
		}
	}

	if len(selectors) == 0 {
		return nil, fmt.Errorf("AI did not return any usable selectors")
	}

	return selectors, nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/nodes/ai/crawl/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nodes/ai/crawl/extract_page.go
git commit -m "feat(crawl): add ai.extract_page node executor (natural + css modes)"
```

---

### Task 6: Registration + Schemas + UI Palette

**Files:**
- Create: `internal/nodes/ai/crawl/register.go`
- Create: `internal/workflow/schemas/ai.read_page.json`
- Create: `internal/workflow/schemas/ai.extract_page.json`
- Modify: `cmd/monoes/node.go` (add registration call)
- Modify: `wails-app/app.go` (add to GetWorkflowNodeTypes)

- [ ] **Step 1: Create register.go**

```go
package crawl

import (
	cfgpkg "github.com/nokhodian/mono-agent/internal/config"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// RegisterAll registers the AI crawl nodes.
func RegisterAll(r *workflow.NodeTypeRegistry, cfgClient *cfgpkg.APIClient) {
	r.Register("ai.read_page", func() workflow.NodeExecutor {
		return &ReadPageNode{}
	})
	r.Register("ai.extract_page", func() workflow.NodeExecutor {
		return &ExtractPageNode{CfgClient: cfgClient}
	})
}
```

- [ ] **Step 2: Create schema files**

`internal/workflow/schemas/ai.read_page.json`:
```json
{
  "credential_platform": null,
  "fields": [
    {"key": "url", "label": "URL", "type": "text", "required": true, "placeholder": "https://example.com/article"},
    {"key": "render_mode", "label": "Render Mode", "type": "select", "default": "auto", "options": ["auto", "static", "browser"], "help": "auto = try static first, fall back to browser if page needs JS."},
    {"key": "include_links", "label": "Include Links", "type": "boolean", "default": true},
    {"key": "include_images", "label": "Include Images", "type": "boolean", "default": true},
    {"key": "include_tables", "label": "Include Tables", "type": "boolean", "default": true},
    {"key": "keep_nav", "label": "Keep Nav/Header/Footer", "type": "boolean", "default": false, "help": "Keep navigation, header, and footer content (usually boilerplate)."},
    {"key": "max_tokens", "label": "Max Tokens", "type": "number", "default": 0, "help": "Truncate markdown to ~N tokens. 0 = no limit."},
    {"key": "wait_selector", "label": "Wait For Selector", "type": "text", "placeholder": "#main-content", "depends_on": {"key": "render_mode", "values": ["browser"]}, "help": "CSS selector to wait for before extracting (browser mode only)."}
  ]
}
```

`internal/workflow/schemas/ai.extract_page.json`:
```json
{
  "credential_platform": null,
  "fields": [
    {"key": "url", "label": "URL", "type": "text", "required": true, "placeholder": "https://shop.com/product/123"},
    {"key": "extract_mode", "label": "Extraction Mode", "type": "select", "required": true, "default": "natural", "options": ["natural", "css"], "help": "natural = describe what to extract in plain English. css = provide CSS selectors."},
    {"key": "prompt", "label": "What to Extract", "type": "textarea", "rows": 3, "placeholder": "Extract product name, price, rating, and all reviews", "depends_on": {"key": "extract_mode", "values": ["natural"]}, "help": "Describe what you want to extract in plain English."},
    {"key": "fields", "label": "CSS Selectors (JSON)", "type": "code", "language": "json", "rows": 5, "placeholder": "{\"name\": \"h1.title\", \"price\": \".price-tag\"}", "depends_on": {"key": "extract_mode", "values": ["css"]}, "help": "JSON map of field name to CSS selector."},
    {"key": "list_selector", "label": "List Selector", "type": "text", "placeholder": ".product-card", "help": "If set, extracts an array of items matching this selector."},
    {"key": "render_mode", "label": "Render Mode", "type": "select", "default": "auto", "options": ["auto", "static", "browser"]},
    {"key": "wait_selector", "label": "Wait For Selector", "type": "text", "depends_on": {"key": "render_mode", "values": ["browser"]}}
  ]
}
```

- [ ] **Step 3: Register in CLI node builder**

In `cmd/monoes/node.go`, find where `ainodes.RegisterAll(registry, store)` is called (~line 298). Add after it:

```go
	crawlnodes "github.com/nokhodian/mono-agent/internal/nodes/ai/crawl"
```
(add to imports)

And in the registration block:
```go
	// AI crawl nodes (no store dependency, but need config API client)
	var cfgClient *cfgpkg.APIClient
	if cfg != nil {
		cfgClient = cfgpkg.NewAPIClient("", zerolog.Nop())
	}
	crawlnodes.RegisterAll(registry, cfgClient)
```

- [ ] **Step 4: Add to UI palette**

In `wails-app/app.go`, find the `"ai"` category in `GetWorkflowNodeTypes` (~line 1817). Add two entries:

```go
	mkNode("ai.read_page", "Read Page", "ai", "Crawl a webpage and return clean readable content (markdown, links, images)"),
	mkNode("ai.extract_page", "Extract Page", "ai", "Extract specific fields from a webpage using AI or CSS selectors"),
```

- [ ] **Step 5: Verify full build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nodes/ai/crawl/register.go \
       internal/workflow/schemas/ai.read_page.json \
       internal/workflow/schemas/ai.extract_page.json \
       cmd/monoes/node.go \
       wails-app/app.go
git commit -m "feat(crawl): register ai.read_page + ai.extract_page — schemas, CLI, UI palette"
```

---

### Task 7: Rebuild CLI + Smoke Test

**Files:** None new — just build and test.

- [ ] **Step 1: Rebuild the CLI binary**

```bash
make build && cp bin/monoes ~/go/bin/monoes
```

- [ ] **Step 2: Smoke test ai.read_page**

```bash
~/go/bin/monoes node run ai.read_page --config '{"url":"https://example.com"}' --output json 2>&1 | head -20
```

Expected: JSON output with `title`, `markdown`, `main_text`, `links`, `token_count` fields. `render_mode_used` should be `"static"`.

- [ ] **Step 3: Smoke test ai.read_page with browser mode**

```bash
~/go/bin/monoes node run ai.read_page --config '{"url":"https://example.com","render_mode":"browser"}' --output json 2>&1 | head -20
```

Expected: Same fields, `render_mode_used` should be `"browser"`.

- [ ] **Step 4: Smoke test ai.extract_page with CSS mode**

```bash
~/go/bin/monoes node run ai.extract_page --config '{"url":"https://example.com","extract_mode":"css","fields":"{\"title\":\"h1\",\"description\":\"p\"}"}' --output json 2>&1
```

Expected: `extracted.title` = "Example Domain", `extracted.description` contains text, `selectors_used` present.

- [ ] **Step 5: Commit version bump**

```bash
git add -A
git commit -m "feat(crawl): smoke-tested ai.read_page + ai.extract_page — ready for release"
git push origin master
```

---

### E2E Test Recipe

After all tasks, verify the full pipeline:

```bash
# 1. Read a real article
monoes node run ai.read_page --config '{"url":"https://blog.golang.org/go1.21"}' --output json | jq '.items[0].json | {title, token_count, render_mode_used}'

# 2. Extract from a product page
monoes node run ai.extract_page --config '{"url":"https://books.toscrape.com/catalogue/a-light-in-the-attic_1000/index.html","extract_mode":"css","fields":"{\"title\":\"h1\",\"price\":\".price_color\",\"stock\":\".instock\"}"}' --output json | jq '.items[0].json.extracted'

# 3. In the Wails UI: open Workflow Editor → add ai.read_page node → configure URL → run → check output
```
