package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// ── node catalogue ────────────────────────────────────────────────────────────

type nodeDoc struct {
	Type        string
	Category    string
	Short       string
	Description string
	Config      string // example config JSON (indented)
	Inputs      string
	Outputs     string
	Notes       string
}

var nodeDocs = []nodeDoc{
	// ── Triggers ──────────────────────────────────────────────────────────────
	{
		Type:     "trigger.schedule",
		Category: "trigger",
		Short:    "Run workflow on a cron schedule",
		Description: `Starts the workflow at a recurring time. Uses a 6-field cron expression
(seconds minute hour day month weekday) as required by robfig/cron v3.`,
		Config: `{
  "cron": "0 0 9 * * *"   // every day at 09:00:00
}`,
		Inputs:  "none",
		Outputs: "one empty item that triggers downstream nodes",
		Notes:   `6-field cron: sec min hour day month weekday. Run "monoes schedule run" to start the scheduler daemon.`,
	},

	// ── System ────────────────────────────────────────────────────────────────
	{
		Type:     "system.rss_read",
		Category: "system",
		Short:    "Fetch items from an RSS / Atom feed",
		Description: `Downloads and parses an RSS or Atom feed, emitting one item per entry.
Commonly used to pull news, blog posts, or podcast episodes.`,
		Config: `{
  "url":   "https://rss.dw.com/rdf/rss-en-all",
  "limit": 15
}`,
		Inputs:  "none (URL comes from config)",
		Outputs: "title, description, link, pubDate, guid, author, categories",
	},
	{
		Type:     "system.execute_command",
		Category: "system",
		Short:    "Run a shell command and capture output",
		Config: `{
  "command": "echo hello",
  "timeout": 30
}`,
		Inputs:  "none",
		Outputs: "stdout, stderr, exit_code",
	},

	// ── Core / Control ────────────────────────────────────────────────────────
	{
		Type:     "core.merge",
		Category: "core",
		Short:    "Merge items from multiple upstream nodes",
		Description: `Collects all items from every connected input and passes them on together.
Use this after branching (e.g. three parallel RSS reads) to rejoin the stream.`,
		Config:  `{}   // no config required`,
		Inputs:  "any number of upstream items",
		Outputs: "all items combined into one stream",
	},
	{
		Type:     "core.aggregate",
		Category: "core",
		Short:    "Collect items into arrays (group by)",
		Description: `Collects a field from every incoming item into a single array output.
Useful for building a list of titles or descriptions for a batch prompt.`,
		Config: `{
  "operations": [
    { "field": "title",       "operation": "array", "output_field": "titles" },
    { "field": "description", "operation": "array", "output_field": "descriptions" }
  ]
}`,
		Inputs:  "many items with the aggregated fields",
		Outputs: "one item containing the collected arrays",
	},
	{
		Type:     "core.set",
		Category: "core",
		Short:    "Compute / set fields on each item",
		Description: `Evaluates template expressions and assigns their results as new fields.
Use to build prompts, format strings, or reshape data before the next node.`,
		Config: `{
  "assignments": [
    {
      "field": "my_prompt",
      "value": "Summarise: {{ index $json.titles 0 }}",
      "type":  "string"
    },
    {
      "field": "count",
      "value": "{{ len $json.items }}",
      "type":  "string"
    }
  ]
}`,
		Inputs:  "any item",
		Outputs: "same item with new fields added / overwritten",
	},
	{
		Type:     "core.filter",
		Category: "core",
		Short:    "Pass only items that satisfy a condition",
		Config: `{
  "condition": "{{ gt (len $json.title) 5 }}"
}`,
		Inputs:  "any items",
		Outputs: "items where condition evaluates to true",
	},
	{
		Type:     "core.if",
		Category: "core",
		Short:    "Branch on a boolean condition (true / false outputs)",
		Config: `{
  "condition": "{{ eq $json.status \"active\" }}"
}`,
		Inputs:  "any item",
		Outputs: "true branch or false branch",
	},
	{
		Type:     "core.switch",
		Category: "core",
		Short:    "Multi-branch switch on a value",
		Config: `{
  "field":  "$json.platform",
  "cases":  ["instagram", "linkedin"],
  "default": "other"
}`,
		Inputs:  "any item",
		Outputs: "one output per case + default",
	},
	{
		Type:     "core.split_in_batches",
		Category: "core",
		Short:    "Split a large stream into fixed-size batches",
		Config:   `{ "batch_size": 10 }`,
		Inputs:   "stream of items",
		Outputs:  "items in groups of batch_size",
	},
	{
		Type:     "core.remove_duplicates",
		Category: "core",
		Short:    "Deduplicate items by a field",
		Config:   `{ "field": "url" }`,
		Inputs:   "items possibly containing duplicates",
		Outputs:  "items with duplicates removed",
	},
	{
		Type:     "core.sort",
		Category: "core",
		Short:    "Sort items by a field",
		Config:   `{ "field": "pubDate", "order": "desc" }`,
		Inputs:   "unsorted items",
		Outputs:  "sorted items",
	},
	{
		Type:     "core.limit",
		Category: "core",
		Short:    "Keep only the first N items",
		Config:   `{ "limit": 5 }`,
		Inputs:   "any items",
		Outputs:  "first N items",
	},
	{
		Type:     "core.wait",
		Category: "core",
		Short:    "Pause execution for a fixed duration",
		Config:   `{ "seconds": 5 }`,
		Inputs:   "any items (passed through unchanged)",
		Outputs:  "same items after waiting",
	},
	{
		Type:     "core.stop_error",
		Category: "core",
		Short:    "Abort workflow with an error message",
		Config:   `{ "message": "{{ $json.error_detail }}" }`,
		Inputs:   "any item",
		Outputs:  "never (throws error)",
	},
	{
		Type:     "core.code",
		Category: "core",
		Short:    "Run custom JavaScript logic on items",
		Config: `{
  "code": "return items.map(i => ({ ...i.json, extra: 'hello' }))"
}`,
		Inputs:  "any items",
		Outputs: "items returned by the code",
	},
	{
		Type:     "core.compare_datasets",
		Category: "core",
		Short:    "Compare two datasets and emit differences",
		Config:   `{ "key": "id" }`,
		Inputs:   "two item streams",
		Outputs:  "added, removed, changed items",
	},

	// ── HTTP / Network ────────────────────────────────────────────────────────
	{
		Type:     "http.request",
		Category: "http",
		Short:    "Make an HTTP request (GET, POST, PUT, DELETE, …)",
		Config: `{
  "method":  "POST",
  "url":     "https://api.example.com/data",
  "headers": { "Content-Type": "application/json" },
  "body":    "{{ json $json.payload }}"
}`,
		Inputs:  "any item (fields usable in config templates)",
		Outputs: "status_code, body (parsed), headers",
	},
	{
		Type:     "http.ftp",
		Category: "http",
		Short:    "Upload / download files via FTP",
		Config: `{
  "host":     "ftp.example.com",
  "port":     21,
  "username": "user",
  "password": "pass",
  "operation": "upload",
  "remote_path": "/pub/file.csv",
  "local_path":  "/tmp/file.csv"
}`,
		Inputs:  "none for download; local file path for upload",
		Outputs: "success, path, size_bytes",
	},
	{
		Type:     "http.ssh",
		Category: "http",
		Short:    "Execute commands on a remote host via SSH",
		Config: `{
  "host":     "server.example.com",
  "port":     22,
  "username": "admin",
  "password": "secret",
  "command":  "ls -la /var/log"
}`,
		Inputs:  "none",
		Outputs: "stdout, stderr, exit_code",
	},

	// ── Data ──────────────────────────────────────────────────────────────────
	{
		Type:     "data.spreadsheet",
		Category: "data",
		Short:    "Read / write CSV or Excel files",
		Config: `{
  "operation": "read",
  "file_path": "/tmp/data.csv",
  "has_header": true
}`,
		Inputs:  "file path or binary data",
		Outputs: "one item per row with column fields",
	},
	{
		Type:     "data.html",
		Category: "data",
		Short:    "Parse HTML — extract text, links, or specific elements",
		Config:   `{ "operation": "extract_text", "selector": "h1" }`,
		Inputs:   "html string field",
		Outputs:  "extracted text / elements",
	},
	{
		Type:     "data.xml",
		Category: "data",
		Short:    "Parse or generate XML",
		Config:   `{ "operation": "parse", "field": "raw_xml" }`,
		Inputs:   "xml string field",
		Outputs:  "parsed JSON structure",
	},
	{
		Type:     "data.markdown",
		Category: "data",
		Short:    "Convert Markdown to HTML or plain text",
		Config:   `{ "operation": "to_html", "field": "body" }`,
		Inputs:   "markdown string field",
		Outputs:  "html or text string",
	},
	{
		Type:     "data.crypto",
		Category: "data",
		Short:    "Hash, encode, or encrypt values",
		Config:   `{ "operation": "hash", "algorithm": "sha256", "field": "password" }`,
		Inputs:   "any item",
		Outputs:  "hashed / encoded value",
	},
	{
		Type:     "data.compression",
		Category: "data",
		Short:    "GZip or Zip compress / decompress data",
		Config:   `{ "operation": "compress", "format": "gzip" }`,
		Inputs:   "binary or string data",
		Outputs:  "compressed binary",
	},
	{
		Type:     "data.datetime",
		Category: "data",
		Short:    "Parse, format, or calculate date/time values",
		Config:   `{ "operation": "format", "field": "pubDate", "format": "2006-01-02" }`,
		Inputs:   "any item",
		Outputs:  "formatted date string or epoch int",
	},
	{
		Type:     "data.write_binary_file",
		Category: "data",
		Short:    "Write binary data (images, PDFs) to disk",
		Config:   `{ "file_path": "/tmp/{{ $json.filename }}" }`,
		Inputs:   "item with binary field",
		Outputs:  "path, size_bytes",
	},

	// ── Databases ─────────────────────────────────────────────────────────────
	{
		Type:     "db.postgres",
		Category: "db",
		Short:    "Query or write to a PostgreSQL database",
		Config: `{
  "credential_id": "my-pg",
  "operation":     "query",
  "query":         "SELECT * FROM users WHERE active = true LIMIT 10"
}`,
		Inputs:  "none for SELECT; item fields usable as bind params",
		Outputs: "one item per row",
	},
	{
		Type:     "db.mysql",
		Category: "db",
		Short:    "Query or write to a MySQL / MariaDB database",
		Config:   `{ "credential_id": "my-mysql", "operation": "execute", "query": "INSERT INTO logs(msg) VALUES (?)", "params": ["{{ $json.message }}"] }`,
		Inputs:   "item fields as bind params",
		Outputs:  "affected_rows, last_insert_id",
	},
	{
		Type:     "db.mongodb",
		Category: "db",
		Short:    "Read or write documents in MongoDB",
		Config:   `{ "credential_id": "my-mongo", "database": "mydb", "collection": "events", "operation": "find", "filter": { "type": "news" } }`,
		Inputs:   "filter / document from config",
		Outputs:  "one item per document",
	},
	{
		Type:     "db.redis",
		Category: "db",
		Short:    "Get / set keys in Redis",
		Config:   `{ "credential_id": "my-redis", "operation": "set", "key": "cache:{{ $json.id }}", "value": "{{ json $json }}", "ttl": 3600 }`,
		Inputs:   "any item",
		Outputs:  "value (for get), ok (for set)",
	},

	// ── Comms ─────────────────────────────────────────────────────────────────
	{
		Type:     "comm.email_send",
		Category: "comm",
		Short:    "Send an email via SMTP or a provider",
		Config: `{
  "credential_id": "gmail-smtp",
  "to":      "{{ $json.email }}",
  "subject": "Daily news digest",
  "body":    "{{ $json.summary }}"
}`,
		Inputs:  "item with email fields (or hardcoded in config)",
		Outputs: "message_id, success",
	},
	{
		Type:     "comm.email_read",
		Category: "comm",
		Short:    "Fetch emails from an IMAP mailbox",
		Config:   `{ "credential_id": "my-imap", "folder": "INBOX", "unseen_only": true, "limit": 10 }`,
		Inputs:   "none",
		Outputs:  "one item per email: from, subject, body, date",
	},
	{
		Type:     "comm.slack",
		Category: "comm",
		Short:    "Send a Slack message to a channel or user",
		Config:   `{ "credential_id": "slack-bot", "channel": "#news", "text": "{{ $json.summary }}" }`,
		Inputs:   "item with message content",
		Outputs:  "ts (timestamp), channel",
	},
	{
		Type:     "comm.telegram",
		Category: "comm",
		Short:    "Send a Telegram message via Bot API",
		Config:   `{ "credential_id": "tg-bot", "chat_id": "123456789", "text": "{{ $json.message }}" }`,
		Inputs:   "item with message content",
		Outputs:  "message_id",
	},
	{
		Type:     "comm.discord",
		Category: "comm",
		Short:    "Post a message to a Discord channel via webhook",
		Config:   `{ "webhook_url": "https://discord.com/api/webhooks/...", "content": "{{ $json.text }}" }`,
		Inputs:   "item with message content",
		Outputs:  "success",
	},
	{
		Type:     "comm.whatsapp",
		Category: "comm",
		Short:    "Send a WhatsApp message via Twilio or Meta API",
		Config:   `{ "credential_id": "twilio-wa", "to": "+491234567890", "body": "{{ $json.message }}" }`,
		Inputs:   "item with message content",
		Outputs:  "message_sid, status",
	},
	{
		Type:     "comm.twilio",
		Category: "comm",
		Short:    "Send an SMS via Twilio",
		Config:   `{ "credential_id": "twilio", "to": "+491234567890", "body": "{{ $json.sms_text }}" }`,
		Inputs:   "item with SMS content",
		Outputs:  "message_sid, status",
	},

	// ── Services ──────────────────────────────────────────────────────────────
	{
		Type:     "service.google_sheets",
		Category: "service",
		Short:    "Read or write Google Sheets rows",
		Config: `{
  "credential_id":  "google-oauth",
  "spreadsheet_id": "1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgVE2upms",
  "sheet":          "Sheet1",
  "operation":      "append",
  "values":         [["{{ $json.title }}", "{{ $json.link }}"]]
}`,
		Inputs:  "item with row data",
		Outputs: "updated_range, updates",
	},
	{
		Type:     "service.gmail",
		Category: "service",
		Short:    "Send or read Gmail messages via Gmail API",
		Config:   `{ "credential_id": "gmail-oauth", "operation": "send", "to": "user@example.com", "subject": "Report", "body": "{{ $json.report }}" }`,
		Inputs:   "item with message fields",
		Outputs:  "message_id, thread_id",
	},
	{
		Type:     "service.google_drive",
		Category: "service",
		Short:    "Upload or download files in Google Drive",
		Config:   `{ "credential_id": "google-oauth", "operation": "upload", "file_path": "/tmp/report.pdf", "folder_id": "1ABC..." }`,
		Inputs:   "local file path",
		Outputs:  "file_id, web_view_link",
	},
	{
		Type:     "service.github",
		Category: "service",
		Short:    "Create issues, PRs, or fetch repo data from GitHub",
		Config:   `{ "credential_id": "github-token", "operation": "create_issue", "owner": "myorg", "repo": "myrepo", "title": "{{ $json.title }}", "body": "{{ $json.body }}" }`,
		Inputs:   "item with issue/PR data",
		Outputs:  "issue_number, html_url",
	},
	{
		Type:     "service.notion",
		Category: "service",
		Short:    "Read or write Notion pages and databases",
		Config:   `{ "credential_id": "notion-token", "operation": "create_page", "database_id": "abc123", "properties": { "Name": "{{ $json.title }}" } }`,
		Inputs:   "item with page properties",
		Outputs:  "page_id, url",
	},
	{
		Type:     "service.airtable",
		Category: "service",
		Short:    "Read or write Airtable records",
		Config:   `{ "credential_id": "airtable-key", "base_id": "appXXXX", "table": "Leads", "operation": "create", "fields": { "Name": "{{ $json.name }}", "URL": "{{ $json.url }}" } }`,
		Inputs:   "item with record fields",
		Outputs:  "record_id, created_time",
	},
	{
		Type:     "service.hubspot",
		Category: "service",
		Short:    "Manage HubSpot contacts, deals, or companies",
		Config:   `{ "credential_id": "hubspot-token", "operation": "create_contact", "email": "{{ $json.email }}", "firstname": "{{ $json.name }}" }`,
		Inputs:   "item with CRM data",
		Outputs:  "contact_id, portal_id",
	},
	{
		Type:     "service.salesforce",
		Category: "service",
		Short:    "Create or update Salesforce records",
		Config:   `{ "credential_id": "sf-oauth", "operation": "create", "sobject": "Lead", "fields": { "LastName": "{{ $json.name }}", "Email": "{{ $json.email }}" } }`,
		Inputs:   "item with record fields",
		Outputs:  "id, success",
	},
	{
		Type:     "service.linear",
		Category: "service",
		Short:    "Create or update Linear issues and projects",
		Config:   `{ "credential_id": "linear-key", "operation": "create_issue", "team_id": "TEAM-1", "title": "{{ $json.title }}", "description": "{{ $json.body }}" }`,
		Inputs:   "item with issue data",
		Outputs:  "issue_id, url",
	},
	{
		Type:     "service.jira",
		Category: "service",
		Short:    "Create or update Jira issues",
		Config:   `{ "credential_id": "jira-token", "operation": "create", "project": "PROJ", "issuetype": "Bug", "summary": "{{ $json.title }}" }`,
		Inputs:   "item with issue fields",
		Outputs:  "issue_key, url",
	},
	{
		Type:     "service.asana",
		Category: "service",
		Short:    "Create Asana tasks",
		Config:   `{ "credential_id": "asana-token", "project": "12345", "name": "{{ $json.title }}", "notes": "{{ $json.description }}" }`,
		Inputs:   "item with task data",
		Outputs:  "task_id, permalink_url",
	},
	{
		Type:     "service.shopify",
		Category: "service",
		Short:    "Manage Shopify orders, products, or customers",
		Config:   `{ "credential_id": "shopify-token", "operation": "list_orders", "status": "open", "limit": 50 }`,
		Inputs:   "filter parameters from config",
		Outputs:  "one item per order/product/customer",
	},
	{
		Type:     "service.stripe",
		Category: "service",
		Short:    "Fetch Stripe payments, customers, or subscriptions",
		Config:   `{ "credential_id": "stripe-key", "operation": "list_payments", "limit": 10 }`,
		Inputs:   "none",
		Outputs:  "one item per payment intent",
	},
	{
		Type:     "service.huggingface",
		Category: "service",
		Short:    "Call Hugging Face inference API for text/image models",
		Config:   `{ "credential_id": "hf-token", "model": "gpt2", "inputs": "{{ $json.prompt }}" }`,
		Inputs:   "item with prompt/input",
		Outputs:  "generated_text or other model output",
	},
	{
		Type:     "service.openrouter",
		Category: "service",
		Short:    "Call any LLM via OpenRouter unified API",
		Config:   `{ "credential_id": "openrouter-key", "model": "mistralai/mistral-7b-instruct", "prompt": "{{ $json.prompt }}" }`,
		Inputs:   "item with prompt",
		Outputs:  "response_text",
	},

	// ── AI ────────────────────────────────────────────────────────────────────
	{
		Type:     "ai.agent",
		Category: "ai",
		Short:    "Autonomous AI agent with tool access",
		Config:   `{ "credential_id": "my-ai", "system": "You are a helpful assistant.", "prompt": "{{ $json.task }}", "max_iterations": 5 }`,
		Inputs:   "item with task/prompt",
		Outputs:  "result, iterations_used",
	},
	{
		Type:     "ai.chat",
		Category: "ai",
		Short:    "Single-turn AI chat completion",
		Config:   `{ "credential_id": "my-ai", "system": "You are an assistant.", "prompt": "{{ $json.question }}" }`,
		Inputs:   "item with prompt",
		Outputs:  "response_text",
	},
	{
		Type:     "ai.classify",
		Category: "ai",
		Short:    "Classify text into categories using AI",
		Config:   `{ "credential_id": "my-ai", "field": "text", "categories": ["positive", "negative", "neutral"] }`,
		Inputs:   "item with text field",
		Outputs:  "category, confidence",
	},
	{
		Type:     "ai.extract",
		Category: "ai",
		Short:    "Extract structured data from text using AI",
		Config:   `{ "credential_id": "my-ai", "field": "raw_text", "schema": { "name": "string", "email": "string" } }`,
		Inputs:   "item with text field",
		Outputs:  "extracted fields matching schema",
	},
	{
		Type:     "ai.transform",
		Category: "ai",
		Short:    "Transform / rewrite text using AI",
		Config:   `{ "credential_id": "my-ai", "field": "text", "instruction": "Translate to Persian" }`,
		Inputs:   "item with text field",
		Outputs:  "transformed_text",
	},
	{
		Type:     "ai.read_page",
		Category: "ai",
		Short:    "Fetch a URL and extract its text content",
		Config:   `{ "url": "{{ $json.link }}", "selector": "article" }`,
		Inputs:   "item with URL",
		Outputs:  "page_text, title, url",
	},
	{
		Type:     "ai.extract_page",
		Category: "ai",
		Short:    "Crawl a URL and extract structured data using AI",
		Config:   `{ "credential_id": "my-ai", "url": "{{ $json.link }}", "schema": { "headline": "string", "date": "string" } }`,
		Inputs:   "item with URL",
		Outputs:  "extracted fields",
	},
	{
		Type:     "ai.embed",
		Category: "ai",
		Short:    "Generate vector embeddings for text",
		Config:   `{ "credential_id": "my-ai", "field": "text", "model": "text-embedding-3-small" }`,
		Inputs:   "item with text field",
		Outputs:  "embedding (float array), dimensions",
	},

	// ── Gemini (browser-automation) ───────────────────────────────────────────
	{
		Type:     "gemini.generate_text",
		Category: "gemini",
		Short:    "Send a prompt to Gemini and return generated text",
		Description: `NO API KEY REQUIRED. Uses browser automation — monoes opens gemini.google.com
in a real Chrome session, types the prompt, and reads the response back.

Setup (one-time): run  monoes login gemini  and log in with your Google account.
After that, credential_id is optional — if you have only one Gemini session it
is resolved automatically. Omit it entirely and monoes will find it.`,
		Config: `{
  "prompt":         "{{ $json.my_prompt }}",
  "maxWaitSeconds": 90
}

// credential_id is OPTIONAL. Only needed if you have multiple Gemini sessions:
// "credential_id": "social:gemini:gemini-user"`,
		Inputs:  "item with prompt field (or hardcoded prompt)",
		Outputs: "response_text",
		Notes:   "Keep prompts concise. Very long prompts may trigger Gemini's reasoning mode which does not return plain text.",
	},
	{
		Type:     "gemini.generate_image",
		Category: "gemini",
		Short:    "Generate an image via Gemini — NO API KEY, uses browser session",
		Description: `NO API KEY REQUIRED. Uses browser automation — monoes opens gemini.google.com
in a real Chrome session, submits the prompt, waits for the image to appear,
downloads it to disk, and returns the local file path.

Setup (one-time): run  monoes login gemini  and log in with your Google account.
credential_id is optional — omit it and monoes auto-resolves the saved session.

Two methods are available automatically (monoes tries both):
  1. Browser crawl  — logs into gemini.google.com and generates via the web UI (default, no key needed)
  2. API fallback   — used only if a Gemini API key is configured as a connection

Always prefer the browser crawl method. It requires only a Google login, no billing.`,
		Config: `{
  "prompt":         "editorial photo of a city skyline at sunset",
  "maxWaitSeconds": 120,
  "downloadDir":    "~/.monoes/downloads"
}

// credential_id is OPTIONAL — only set it if you have multiple Gemini sessions:
// "credential_id": "social:gemini:gemini-user"`,
		Inputs:  "item with prompt",
		Outputs: "images (array: {path, url, filename, size_bytes}), image_count",
		Notes:   "Use short English prompts (< 100 chars). Long or Persian prompts trigger the Nano Banana 2 reasoning model which cannot generate images.",
	},

	// ── Instagram ─────────────────────────────────────────────────────────────
	{
		Type:     "instagram.publish_post",
		Category: "instagram",
		Short:    "Upload a photo and publish it to Instagram",
		Config: `{
  "credential_id": "onetap",
  "media": [{"url": "{{ $json.media_path }}"}],
  "text": "{{ $json.caption }}"
}`,
		Inputs:  "item with media_path (local file path) and caption",
		Outputs: "post_url, post_id",
		Notes:   "media must be a native JSON array. Use core.set to build media_path as a plain string first.",
	},
	{
		Type:     "instagram.send_dms",
		Category: "instagram",
		Short:    "Send direct messages to a list of users",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.targets }}", "messageText": "Hi {{name}}!" }`,
		Inputs:   "item with targets array",
		Outputs:  "sent_count, failed_count",
	},
	{
		Type:     "instagram.like_posts",
		Category: "instagram",
		Short:    "Like a list of posts",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.posts }}" }`,
		Inputs:   "item with posts array [{url, platform}]",
		Outputs:  "liked_count",
	},
	{
		Type:     "instagram.comment_on_posts",
		Category: "instagram",
		Short:    "Comment on a list of posts",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.posts }}", "commentText": "Great!" }`,
		Inputs:   "item with posts array",
		Outputs:  "commented_count",
	},
	{
		Type:     "instagram.find_by_keyword",
		Category: "instagram",
		Short:    "Search Instagram for profiles or hashtags",
		Config:   `{ "credential_id": "onetap", "keyword": "{{ $json.search_term }}", "limit": 30 }`,
		Inputs:   "item with keyword",
		Outputs:  "one item per result: username, url, followers",
	},
	{
		Type:     "instagram.follow_users",
		Category: "instagram",
		Short:    "Follow a list of users",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.users }}" }`,
		Inputs:   "item with users array",
		Outputs:  "followed_count",
	},
	{
		Type:     "instagram.unfollow_users",
		Category: "instagram",
		Short:    "Unfollow a list of users",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.users }}" }`,
		Inputs:   "item with users array",
		Outputs:  "unfollowed_count",
	},
	{
		Type:     "instagram.scrape_profile_info",
		Category: "instagram",
		Short:    "Scrape public profile data from Instagram",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.profiles }}" }`,
		Inputs:   "item with profiles array [{url}]",
		Outputs:  "username, bio, followers, following, post_count",
	},
	{
		Type:     "instagram.export_followers",
		Category: "instagram",
		Short:    "Export follower list of an Instagram account",
		Config:   `{ "credential_id": "onetap", "target_url": "{{ $json.profile_url }}", "limit": 100 }`,
		Inputs:   "item with profile URL",
		Outputs:  "one item per follower: username, url",
	},
	{
		Type:     "instagram.list_user_posts",
		Category: "instagram",
		Short:    "List recent posts from an Instagram profile",
		Config:   `{ "credential_id": "onetap", "target_url": "{{ $json.profile_url }}", "limit": 12 }`,
		Inputs:   "item with profile URL",
		Outputs:  "one item per post: url, likes, comments, timestamp",
	},
	{
		Type:     "instagram.list_post_comments",
		Category: "instagram",
		Short:    "List comments on an Instagram post",
		Config:   `{ "credential_id": "onetap", "post_url": "{{ $json.url }}", "limit": 50 }`,
		Inputs:   "item with post URL",
		Outputs:  "one item per comment: username, text, timestamp",
	},
	{
		Type:     "instagram.extract_post_data",
		Category: "instagram",
		Short:    "Extract detailed data from a single Instagram post",
		Config:   `{ "credential_id": "onetap", "post_url": "{{ $json.url }}" }`,
		Inputs:   "item with post URL",
		Outputs:  "caption, likes, comments, media_url, timestamp",
	},
	{
		Type:     "instagram.like_comments_on_posts",
		Category: "instagram",
		Short:    "Like comments on a list of posts",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.posts }}" }`,
		Inputs:   "item with posts array",
		Outputs:  "liked_count",
	},
	{
		Type:     "instagram.reply_to_comments",
		Category: "instagram",
		Short:    "Reply to comments on a post",
		Config:   `{ "credential_id": "onetap", "post_url": "{{ $json.url }}", "replyText": "Thank you!" }`,
		Inputs:   "item with post URL",
		Outputs:  "replied_count",
	},
	{
		Type:     "instagram.engage_with_posts",
		Category: "instagram",
		Short:    "Like + comment on posts in one pass",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.posts }}", "commentText": "Nice!" }`,
		Inputs:   "item with posts array",
		Outputs:  "engaged_count",
	},
	{
		Type:     "instagram.engage_user_posts",
		Category: "instagram",
		Short:    "Engage with the recent posts of a specific user",
		Config:   `{ "credential_id": "onetap", "target_url": "{{ $json.profile_url }}", "post_limit": 3 }`,
		Inputs:   "item with profile URL",
		Outputs:  "engaged_count",
	},
	{
		Type:     "instagram.auto_reply_dms",
		Category: "instagram",
		Short:    "Auto-reply to unread DMs based on templates",
		Config:   `{ "credential_id": "onetap", "replyText": "Thanks for reaching out!" }`,
		Inputs:   "none (reads DM inbox directly)",
		Outputs:  "replied_count",
	},
	{
		Type:     "instagram.watch_stories",
		Category: "instagram",
		Short:    "Watch stories from a list of users",
		Config:   `{ "credential_id": "onetap", "selectedListItems": "{{ json $json.users }}" }`,
		Inputs:   "item with users array",
		Outputs:  "watched_count",
	},

	// ── LinkedIn ──────────────────────────────────────────────────────────────
	{
		Type:     "linkedin.publish_post",
		Category: "linkedin",
		Short:    "Publish a post on LinkedIn",
		Config:   `{ "credential_id": "li-onetap", "text": "{{ $json.post_text }}" }`,
		Inputs:   "item with post text",
		Outputs:  "post_url",
	},
	{
		Type:     "linkedin.send_dms",
		Category: "linkedin",
		Short:    "Send LinkedIn connection requests or messages",
		Config:   `{ "credential_id": "li-onetap", "selectedListItems": "{{ json $json.targets }}", "messageText": "Hi!" }`,
		Inputs:   "item with targets array",
		Outputs:  "sent_count",
	},
	{
		Type:     "linkedin.find_by_keyword",
		Category: "linkedin",
		Short:    "Search LinkedIn for people or companies",
		Config:   `{ "credential_id": "li-onetap", "keyword": "{{ $json.search }}", "limit": 20 }`,
		Inputs:   "item with keyword",
		Outputs:  "one item per result: name, url, headline",
	},
	{
		Type:     "linkedin.like_posts",
		Category: "linkedin",
		Short:    "Like LinkedIn posts",
		Config:   `{ "credential_id": "li-onetap", "selectedListItems": "{{ json $json.posts }}" }`,
		Inputs:   "item with posts array",
		Outputs:  "liked_count",
	},
	{
		Type:     "linkedin.comment_on_posts",
		Category: "linkedin",
		Short:    "Comment on LinkedIn posts",
		Config:   `{ "credential_id": "li-onetap", "selectedListItems": "{{ json $json.posts }}", "commentText": "{{ $json.comment }}" }`,
		Inputs:   "item with posts array and comment text",
		Outputs:  "commented_count",
	},
	{
		Type:     "linkedin.scrape_profile_info",
		Category: "linkedin",
		Short:    "Scrape LinkedIn profile data",
		Config:   `{ "credential_id": "li-onetap", "selectedListItems": "{{ json $json.profiles }}" }`,
		Inputs:   "item with profiles array",
		Outputs:  "name, headline, connections, about",
	},
	{
		Type:     "linkedin.export_followers",
		Category: "linkedin",
		Short:    "Export followers or connections from LinkedIn",
		Config:   `{ "credential_id": "li-onetap", "limit": 100 }`,
		Inputs:   "none",
		Outputs:  "one item per follower",
	},
	{
		Type:     "linkedin.engage_with_posts",
		Category: "linkedin",
		Short:    "Like + comment on LinkedIn posts in one pass",
		Config:   `{ "credential_id": "li-onetap", "selectedListItems": "{{ json $json.posts }}", "commentText": "Great insight!" }`,
		Inputs:   "item with posts array",
		Outputs:  "engaged_count",
	},
	{
		Type:     "linkedin.auto_reply_dms",
		Category: "linkedin",
		Short:    "Auto-reply to unread LinkedIn messages",
		Config:   `{ "credential_id": "li-onetap", "replyText": "Thanks for connecting!" }`,
		Inputs:   "none",
		Outputs:  "replied_count",
	},
	{
		Type:     "linkedin.list_post_comments",
		Category: "linkedin",
		Short:    "List comments on a LinkedIn post",
		Config:   `{ "credential_id": "li-onetap", "post_url": "{{ $json.url }}" }`,
		Inputs:   "item with post URL",
		Outputs:  "one item per comment",
	},
	{
		Type:     "linkedin.like_comments",
		Category: "linkedin",
		Short:    "Like comments on LinkedIn posts",
		Config:   `{ "credential_id": "li-onetap", "selectedListItems": "{{ json $json.posts }}" }`,
		Inputs:   "item with posts array",
		Outputs:  "liked_count",
	},
	{
		Type:     "linkedin.list_user_posts",
		Category: "linkedin",
		Short:    "List recent posts from a LinkedIn profile",
		Config:   `{ "credential_id": "li-onetap", "target_url": "{{ $json.profile_url }}", "limit": 10 }`,
		Inputs:   "item with profile URL",
		Outputs:  "one item per post",
	},

	// ── X (Twitter) ───────────────────────────────────────────────────────────
	{
		Type:     "x.publish_post",
		Category: "x",
		Short:    "Publish a tweet / post on X",
		Config:   `{ "credential_id": "x-onetap", "text": "{{ $json.tweet }}" }`,
		Inputs:   "item with text",
		Outputs:  "post_url",
	},
	{
		Type:     "x.send_dms",
		Category: "x",
		Short:    "Send X direct messages",
		Config:   `{ "credential_id": "x-onetap", "selectedListItems": "{{ json $json.targets }}", "messageText": "Hey!" }`,
		Inputs:   "item with targets array",
		Outputs:  "sent_count",
	},
	{
		Type:     "x.find_by_keyword",
		Category: "x",
		Short:    "Search X for users or posts",
		Config:   `{ "credential_id": "x-onetap", "keyword": "{{ $json.query }}", "limit": 20 }`,
		Inputs:   "item with keyword",
		Outputs:  "one item per result",
	},
	{
		Type:     "x.engage_with_posts",
		Category: "x",
		Short:    "Like + repost X posts",
		Config:   `{ "credential_id": "x-onetap", "selectedListItems": "{{ json $json.posts }}" }`,
		Inputs:   "item with posts array",
		Outputs:  "engaged_count",
	},
	{
		Type:     "x.scrape_profile_info",
		Category: "x",
		Short:    "Scrape public profile data from X",
		Config:   `{ "credential_id": "x-onetap", "selectedListItems": "{{ json $json.profiles }}" }`,
		Inputs:   "item with profiles array",
		Outputs:  "username, bio, followers, following",
	},
	{
		Type:     "x.export_followers",
		Category: "x",
		Short:    "Export followers from an X account",
		Config:   `{ "credential_id": "x-onetap", "target_url": "{{ $json.profile_url }}", "limit": 100 }`,
		Inputs:   "item with profile URL",
		Outputs:  "one item per follower",
	},
	{
		Type:     "x.auto_reply_dms",
		Category: "x",
		Short:    "Auto-reply to unread X DMs",
		Config:   `{ "credential_id": "x-onetap", "replyText": "Thanks!" }`,
		Inputs:   "none",
		Outputs:  "replied_count",
	},

	// ── TikTok ────────────────────────────────────────────────────────────────
	{
		Type:     "tiktok.publish_post",
		Category: "tiktok",
		Short:    "Upload and publish a video to TikTok",
		Config:   `{ "credential_id": "tiktok-onetap", "media_path": "{{ $json.video_path }}", "text": "{{ $json.caption }}" }`,
		Inputs:   "item with video path and caption",
		Outputs:  "post_url",
	},
	{
		Type:     "tiktok.send_dms",
		Category: "tiktok",
		Short:    "Send TikTok direct messages",
		Config:   `{ "credential_id": "tiktok-onetap", "selectedListItems": "{{ json $json.targets }}", "messageText": "Hey!" }`,
		Inputs:   "item with targets array",
		Outputs:  "sent_count",
	},
	{
		Type:     "tiktok.find_by_keyword",
		Category: "tiktok",
		Short:    "Search TikTok for users or videos",
		Config:   `{ "credential_id": "tiktok-onetap", "keyword": "{{ $json.query }}", "limit": 20 }`,
		Inputs:   "item with keyword",
		Outputs:  "one item per result",
	},
	{
		Type:     "tiktok.like_video",
		Category: "tiktok",
		Short:    "Like TikTok videos",
		Config:   `{ "credential_id": "tiktok-onetap", "selectedListItems": "{{ json $json.videos }}" }`,
		Inputs:   "item with videos array",
		Outputs:  "liked_count",
	},
	{
		Type:     "tiktok.comment_on_video",
		Category: "tiktok",
		Short:    "Comment on TikTok videos",
		Config:   `{ "credential_id": "tiktok-onetap", "selectedListItems": "{{ json $json.videos }}", "commentText": "Great!" }`,
		Inputs:   "item with videos array",
		Outputs:  "commented_count",
	},
	{
		Type:     "tiktok.engage_with_posts",
		Category: "tiktok",
		Short:    "Like + comment TikTok videos in one pass",
		Config:   `{ "credential_id": "tiktok-onetap", "selectedListItems": "{{ json $json.videos }}", "commentText": "Amazing!" }`,
		Inputs:   "item with videos array",
		Outputs:  "engaged_count",
	},
	{
		Type:     "tiktok.follow_user",
		Category: "tiktok",
		Short:    "Follow TikTok users",
		Config:   `{ "credential_id": "tiktok-onetap", "selectedListItems": "{{ json $json.users }}" }`,
		Inputs:   "item with users array",
		Outputs:  "followed_count",
	},
	{
		Type:     "tiktok.export_followers",
		Category: "tiktok",
		Short:    "Export TikTok followers",
		Config:   `{ "credential_id": "tiktok-onetap", "target_url": "{{ $json.profile_url }}", "limit": 100 }`,
		Inputs:   "item with profile URL",
		Outputs:  "one item per follower",
	},
	{
		Type:     "tiktok.scrape_profile_info",
		Category: "tiktok",
		Short:    "Scrape TikTok profile data",
		Config:   `{ "credential_id": "tiktok-onetap", "selectedListItems": "{{ json $json.profiles }}" }`,
		Inputs:   "item with profiles array",
		Outputs:  "username, bio, followers, videos",
	},
	{
		Type:     "tiktok.list_user_videos",
		Category: "tiktok",
		Short:    "List videos from a TikTok profile",
		Config:   `{ "credential_id": "tiktok-onetap", "target_url": "{{ $json.profile_url }}", "limit": 20 }`,
		Inputs:   "item with profile URL",
		Outputs:  "one item per video",
	},
	{
		Type:     "tiktok.list_video_comments",
		Category: "tiktok",
		Short:    "List comments on a TikTok video",
		Config:   `{ "credential_id": "tiktok-onetap", "video_url": "{{ $json.url }}", "limit": 50 }`,
		Inputs:   "item with video URL",
		Outputs:  "one item per comment",
	},
	{
		Type:     "tiktok.like_comment",
		Category: "tiktok",
		Short:    "Like comments on TikTok videos",
		Config:   `{ "credential_id": "tiktok-onetap", "selectedListItems": "{{ json $json.videos }}" }`,
		Inputs:   "item with videos array",
		Outputs:  "liked_count",
	},
	{
		Type:     "tiktok.auto_reply_dms",
		Category: "tiktok",
		Short:    "Auto-reply to TikTok DMs",
		Config:   `{ "credential_id": "tiktok-onetap", "replyText": "Thanks!" }`,
		Inputs:   "none",
		Outputs:  "replied_count",
	},
	{
		Type:     "tiktok.share_video",
		Category: "tiktok",
		Short:    "Share a TikTok video",
		Config:   `{ "credential_id": "tiktok-onetap", "video_url": "{{ $json.url }}" }`,
		Inputs:   "item with video URL",
		Outputs:  "success",
	},
	{
		Type:     "tiktok.duet_video",
		Category: "tiktok",
		Short:    "Create a duet with a TikTok video",
		Config:   `{ "credential_id": "tiktok-onetap", "video_url": "{{ $json.url }}", "local_video": "/tmp/my.mp4" }`,
		Inputs:   "item with video URL",
		Outputs:  "post_url",
	},
	{
		Type:     "tiktok.stitch_video",
		Category: "tiktok",
		Short:    "Stitch a TikTok video",
		Config:   `{ "credential_id": "tiktok-onetap", "video_url": "{{ $json.url }}", "local_video": "/tmp/my.mp4" }`,
		Inputs:   "item with video URL",
		Outputs:  "post_url",
	},

	// ── People ────────────────────────────────────────────────────────────────
	{
		Type:     "people.save",
		Category: "people",
		Short:    "Save scraped profiles to the people database",
		Config:   `{}`,
		Inputs:   "items with: url, platform, username, display_name, bio",
		Outputs:  "saved_count",
	},
}

// findNodeDoc returns nil when not found.
func findNodeDoc(typ string) *nodeDoc {
	for i := range nodeDocs {
		if strings.EqualFold(nodeDocs[i].Type, typ) {
			return &nodeDocs[i]
		}
	}
	return nil
}

// ── commands catalogue ────────────────────────────────────────────────────────

type cmdDoc struct {
	Name  string
	Short string
	Usage string
	Flags string
	Examples []string
}

var cliDocs = []cmdDoc{
	{
		Name:  "login",
		Short: "Log in to a social platform (opens browser for cookie capture)",
		Usage: "monoes login <platform>",
		Examples: []string{
			"monoes login instagram",
			"monoes login linkedin",
		},
	},
	{
		Name:  "logout",
		Short: "Remove stored session for a platform",
		Usage: "monoes logout <platform>",
		Examples: []string{"monoes logout instagram"},
	},
	{
		Name:  "run",
		Short: "Run a single action by its ID",
		Usage: "monoes run <action-id> [flags]",
		Flags: `  --platform string   Platform override
  --list string       People list ID
  --text string       Message or comment text
  --verbose           Show step-by-step output`,
		Examples: []string{
			"monoes run POST_LIKING --platform instagram --list my-list",
			"monoes run BULK_MESSAGING --platform instagram --text 'Hi {{name}}!'",
		},
	},
	{
		Name:  "search",
		Short: "Search for profiles or posts on a platform",
		Usage: "monoes search <platform> <keyword> [flags]",
		Flags: `  --limit int     Max results (default 50)
  --output string Output file path`,
		Examples: []string{
			"monoes search instagram 'coffee shop'",
			"monoes search linkedin 'software engineer Berlin'",
		},
	},
	{
		Name:  "message",
		Short: "Send bulk direct messages",
		Usage: "monoes message <platform> [flags]",
		Flags: `  --list string   People list ID
  --text string   Message template (supports {{name}}, {{username}})
  --delay int     Delay between messages in seconds (default 5)`,
		Examples: []string{
			"monoes message instagram --list leads --text 'Hi {{name}}!'",
		},
	},
	{
		Name:  "comment",
		Short: "Post comments on a list of posts",
		Usage: "monoes comment <platform> [flags]",
		Flags: `  --list string   People/post list ID
  --text string   Comment text`,
		Examples: []string{
			"monoes comment instagram --list posts --text 'Great content!'",
		},
	},
	{
		Name:  "action",
		Short: "Manage action definitions",
		Usage: "monoes action <subcommand>",
		Flags: `  list          List all available actions
  get <id>      Show action JSON definition
  create        Interactive action creator`,
		Examples: []string{
			"monoes action list",
			"monoes action get POST_LIKING",
		},
	},
	{
		Name:  "people",
		Short: "Manage people / contact lists",
		Usage: "monoes people <subcommand>",
		Flags: `  list                  List all people
  add                   Add a person
  import <file>         Import from CSV/JSON
  export <list-id>      Export list to file
  remove <id>           Remove a person`,
		Examples: []string{
			"monoes people list",
			"monoes people import leads.csv",
		},
	},
	{
		Name:  "list",
		Short: "Manage named lists for targeting",
		Usage: "monoes list <subcommand>",
		Flags: `  create <name>   Create a new list
  delete <id>     Delete a list
  show <id>       Show list contents`,
		Examples: []string{
			"monoes list create my-leads",
		},
	},
	{
		Name:  "template",
		Short: "Manage message/comment templates",
		Usage: "monoes template <subcommand>",
		Flags: `  list          List all templates
  create        Create a template
  delete <id>   Delete a template`,
		Examples: []string{
			"monoes template list",
		},
	},
	{
		Name:  "config",
		Short: "Manage XPath/selector config entries",
		Usage: "monoes config <subcommand>",
		Flags: `  list              List all config entries
  set <key> <val>   Set a config value
  get <key>         Get a config value
  delete <key>      Delete a config entry`,
		Examples: []string{
			"monoes config list",
			`monoes config set INSTAGRAM_POST_LIKING.like_button "//button[@aria-label='Like']"`,
		},
	},
	{
		Name:  "schedule",
		Short: "Manage and run scheduled automation tasks",
		Usage: "monoes schedule <subcommand>",
		Flags: `  list          List all schedules
  add           Add a schedule
  remove <id>   Remove a schedule
  run           Start the scheduler daemon`,
		Examples: []string{
			"monoes schedule run",
		},
	},
	{
		Name:  "export",
		Short: "Export data to CSV or JSON",
		Usage: "monoes export [flags]",
		Flags: `  --format string   csv or json (default json)
  --output string   Output file path`,
		Examples: []string{
			"monoes export --format csv --output results.csv",
		},
	},
	{
		Name:  "status",
		Short: "Show session status and connected platforms",
		Usage: "monoes status",
		Examples: []string{"monoes status"},
	},
	{
		Name:  "workflow",
		Short: "Manage and run multi-node automation workflows",
		Usage: "monoes workflow <subcommand>",
		Flags: `  list          List all workflows
  get <id>      Show workflow definition
  run <id>      Execute a workflow (--verbose to stream logs)
  create        Create a new workflow
  delete <id>   Delete a workflow`,
		Examples: []string{
			"monoes workflow list",
			"monoes workflow run german-news-persian-instagram-v1 --verbose",
			"monoes workflow get my-workflow",
		},
	},
	{
		Name:  "node",
		Short: "Run individual workflow nodes for testing",
		Usage: "monoes node <subcommand>",
		Flags: `  list           List all available node types
  run <type>     Run a node with --config JSON and optional --input JSON`,
		Examples: []string{
			"monoes node list",
			`monoes node run system.rss_read --config '{"url":"https://rss.dw.com/rdf/rss-en-all","limit":5}'`,
			`# Gemini text — NO API KEY needed, uses browser session (run 'monoes login gemini' first)`,
			`monoes node run gemini.generate_text --config '{"prompt":"Say hello in Persian"}'`,
			`# Gemini image — NO API KEY needed, downloads to ~/.monoes/downloads/`,
			`monoes node run gemini.generate_image --config '{"prompt":"sunset over a city skyline","downloadDir":"~/.monoes/downloads"}'`,
		},
	},
	{
		Name:  "connect",
		Short: "Manage AI and external service connections",
		Usage: "monoes connect <subcommand>",
		Flags: `  list          List all connections
  test <id>     Test a connection
  remove <id>   Remove a connection`,
		Examples: []string{
			"monoes connect list",
			"monoes connect test gemini-user",
		},
	},
	{
		Name:  "version",
		Short: "Print version and build information",
		Usage: "monoes version [--json]",
		Examples: []string{
			"monoes version",
			"monoes version --json",
		},
	},
	{
		Name:  "ref",
		Short: "Browse built-in reference documentation",
		Usage: "monoes ref <subcommand>",
		Flags: `  commands              All CLI commands
  nodes                 All node types (grouped by category)
  node <type>           Detailed docs for one node type
  workflow              Workflow JSON format
  expressions           Template expression reference
  examples              Common workflow patterns`,
		Examples: []string{
			"monoes ref commands",
			"monoes ref nodes",
			"monoes ref node gemini.generate_text",
			"monoes ref workflow",
			"monoes ref expressions",
			"monoes ref examples",
		},
	},
}

// ── newRefCmd ─────────────────────────────────────────────────────────────────

func newRefCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "ref",
		Short: "Built-in reference documentation for all commands and node types",
		Long: `Browse comprehensive offline documentation for monoes.

Subcommands:
  commands              All CLI commands with flags and examples
  nodes                 All node types grouped by category
  node <type>           Detailed docs for a specific node type
  workflow              Workflow JSON structure and connection format
  expressions           Template expression syntax and built-in functions
  examples              Common workflow patterns and use cases`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("monoes ref — built-in reference")
			fmt.Println()
			fmt.Println("Subcommands:")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "  commands\tAll CLI commands with flags and examples")
			fmt.Fprintln(w, "  nodes\tAll node types grouped by category")
			fmt.Fprintln(w, "  node <type>\tDetailed docs for a specific node type")
			fmt.Fprintln(w, "  workflow\tWorkflow JSON structure and connection format")
			fmt.Fprintln(w, "  expressions\tTemplate expression syntax and built-in functions")
			fmt.Fprintln(w, "  examples\tCommon workflow patterns and use cases")
			w.Flush()
			fmt.Println()
			fmt.Println("Example:  monoes ref node gemini.generate_text")
			return nil
		},
	}

	root.AddCommand(
		refCommandsCmd(),
		refNodesCmd(),
		refNodeCmd(),
		refWorkflowCmd(),
		refExpressionsCmd(),
		refExamplesCmd(),
	)
	return root
}

// ── ref commands ──────────────────────────────────────────────────────────────

func refCommandsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List all CLI commands with flags and examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("╔══════════════════════════════════════════════════════╗")
			fmt.Println("║             monoes — CLI command reference           ║")
			fmt.Println("╚══════════════════════════════════════════════════════╝")
			fmt.Println()
			for _, d := range cliDocs {
				fmt.Printf("  %-22s  %s\n", d.Name, d.Short)
				fmt.Printf("  Usage: %s\n", d.Usage)
				if d.Flags != "" {
					fmt.Println("  Flags:")
					for _, line := range strings.Split(d.Flags, "\n") {
						fmt.Println(" " + line)
					}
				}
				if len(d.Examples) > 0 {
					fmt.Println("  Examples:")
					for _, ex := range d.Examples {
						fmt.Printf("    $ %s\n", ex)
					}
				}
				fmt.Println(strings.Repeat("─", 60))
			}
		},
	}
}

// ── ref nodes ─────────────────────────────────────────────────────────────────

func refNodesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "nodes",
		Short: "List all node types grouped by category",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("╔══════════════════════════════════════════════════════════════╗")
			fmt.Println("║                  monoes — node type reference                ║")
			fmt.Println("╚══════════════════════════════════════════════════════════════╝")
			fmt.Println()
			fmt.Printf("  %-38s  %s\n", "Type", "Description")
			fmt.Println("  " + strings.Repeat("─", 70))

			categories := []string{"trigger", "system", "core", "http", "data", "db", "comm", "service", "ai", "gemini", "instagram", "linkedin", "x", "tiktok", "people"}
			byCategory := make(map[string][]nodeDoc)
			for _, n := range nodeDocs {
				byCategory[n.Category] = append(byCategory[n.Category], n)
			}

			for _, cat := range categories {
				nodes := byCategory[cat]
				if len(nodes) == 0 {
					continue
				}
				fmt.Printf("\n  ── %s %s\n", strings.ToUpper(cat), strings.Repeat("─", max(0, 60-len(cat))))
				for _, n := range nodes {
					fmt.Printf("  %-38s  %s\n", n.Type, n.Short)
				}
			}
			fmt.Println()
			fmt.Println("  Run: monoes ref node <type>   for full details on any node.")
		},
	}
}

// ── ref node <type> ───────────────────────────────────────────────────────────

func refNodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "node <type>",
		Short: "Show detailed documentation for a specific node type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ := args[0]
			d := findNodeDoc(typ)
			if d == nil {
				return fmt.Errorf("unknown node type %q — run 'monoes ref nodes' to see all types", typ)
			}

			sep := strings.Repeat("═", 62)
			fmt.Println(sep)
			fmt.Printf("  NODE: %s\n", d.Type)
			fmt.Printf("  Category: %s\n", d.Category)
			fmt.Println(sep)
			fmt.Printf("\n  %s\n", d.Short)
			if d.Description != "" {
				fmt.Println()
				for _, line := range strings.Split(d.Description, "\n") {
					fmt.Printf("  %s\n", line)
				}
			}
			if d.Config != "" {
				fmt.Println("\n  Config example:")
				for _, line := range strings.Split(d.Config, "\n") {
					fmt.Printf("    %s\n", line)
				}
			}
			if d.Inputs != "" {
				fmt.Printf("\n  Inputs:   %s\n", d.Inputs)
			}
			if d.Outputs != "" {
				fmt.Printf("  Outputs:  %s\n", d.Outputs)
			}
			if d.Notes != "" {
				fmt.Println("\n  Notes:")
				for _, line := range strings.Split(d.Notes, "\n") {
					fmt.Printf("    %s\n", line)
				}
			}
			fmt.Println()
			return nil
		},
	}
}

// ── ref workflow ──────────────────────────────────────────────────────────────

func refWorkflowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "workflow",
		Short: "Workflow JSON structure and connection format",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(`
╔══════════════════════════════════════════════════════════════╗
║                monoes — workflow format reference            ║
╚══════════════════════════════════════════════════════════════╝

FILE LOCATION
  ~/.monoes/workflows/<workflow-id>.json
  The filename MUST match the workflow's "id" field exactly.

SCHEMA
  {
    "id":          "my-workflow-v1",       // must match filename
    "name":        "My Workflow",
    "description": "What it does",
    "version":     1,
    "is_active":   true,
    "nodes":       [...],
    "connections": [...]
  }

NODE OBJECT
  {
    "id":       "n1",                    // unique within workflow
    "type":     "system.rss_read",       // node type (monoes ref nodes)
    "name":     "DW Feed",               // display name
    "position": { "x": 100, "y": 300 }, // canvas position (visual only)
    "config":   { ... }                  // node-specific config
  }

CONNECTION OBJECT
  {
    "id":            "c1",
    "source":        "n1",      // source node id
    "source_handle": "main",    // output handle (usually "main")
    "target":        "n2",      // target node id
    "target_handle": "main"     // input handle (usually "main")
  }

TRIGGER NODE
  trigger.schedule uses 6-field cron (sec min hour day month weekday):
  "cron": "0 0 9 * * *"   // every day at 09:00
  Run scheduler with: monoes schedule run

PARALLEL BRANCHES
  Connect one source to multiple targets — all run concurrently.
  Rejoin branches with core.merge.

EXAMPLE — minimal two-node workflow:
  {
    "id": "hello-v1",
    "name": "Hello",
    "version": 1,
    "is_active": true,
    "nodes": [
      { "id":"n1", "type":"trigger.schedule", "name":"Daily",
        "position":{"x":100,"y":100},
        "config":{ "cron":"0 0 9 * * *" } },
      { "id":"n2", "type":"system.execute_command", "name":"Say hi",
        "position":{"x":350,"y":100},
        "config":{ "command":"echo hello" } }
    ],
    "connections": [
      { "id":"c1","source":"n1","source_handle":"main","target":"n2","target_handle":"main" }
    ]
  }

Run: monoes workflow run hello-v1

`)
		},
	}
}

// ── ref expressions ───────────────────────────────────────────────────────────

func refExpressionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "expressions",
		Short: "Template expression syntax and built-in functions",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(`
╔══════════════════════════════════════════════════════════════╗
║              monoes — expression / template reference        ║
╚══════════════════════════════════════════════════════════════╝

SYNTAX
  Config string values may contain Go template expressions: {{ ... }}
  Strings with no {{ }} are returned as-is (no parsing overhead).

BUILT-IN VARIABLES
  $json                       current item's JSON map
  $json.field_name            access a field on the current item
  $node["NodeName"].json.f    access a previous node's first output item
  $workflow.id                workflow ID string
  $execution.id               execution run ID string
  $env.MY_VAR                 OS environment variable

ENCODING
  json <value>                marshal Go value to JSON string
  jsonParse <string>          parse JSON string to Go value

TYPE CONVERSIONS
  toString <v>                convert any value to string
  toInt <v>                   convert to int64
  toFloat <v>                 convert to float64
  toBool <v>                  convert to bool

STRING OPERATIONS
  upper <s>                   uppercase
  lower <s>                   lowercase
  trim <s>                    trim whitespace
  trimLeft <cutset> <s>       trim left chars
  trimRight <cutset> <s>      trim right chars
  split <sep> <s>             split string → []string
  join <sep> <parts>          join []string → string
  replace <old> <new> <s>     replace all occurrences

MAP HELPERS
  hasKey <map> <key>          returns bool
  default <def> <v>           return def if v is nil or empty string

COLLECTION HELPERS
  len <v>                     length of string, array, or map
  index <arr> <n>             get element at index n (0-based)
                              works with []interface{}, []map[string]interface{},
                              and map[string]interface{}

TIME
  now                         current UTC time as RFC3339 string

ARITHMETIC  (all operate on float64)
  add <a> <b>
  sub <a> <b>
  mul <a> <b>
  div <a> <b>                 returns error on division by zero

EXAMPLES
  {{ $json.title }}                          — field access
  {{ upper $json.title }}                    — function call
  {{ index $json.titles 0 }}                 — first element of an array
  {{ replace "foo" "bar" $json.text }}       — string replace
  {{ json $json.items }}                     — marshal to JSON
  {{ default "n/a" $json.optional_field }}   — default value
  {{ $node["RSS Feed"].json.title }}         — previous node output
  {{ $env.SLACK_WEBHOOK }}                   — env variable
  {{ add $json.count 1.0 }}                  — arithmetic

TIPS
  - Config values that evaluate to a JSON array or object are automatically
    parsed back to native types so downstream nodes receive the correct type.
  - Use core.set to precompute values before passing them to API nodes.
  - The "replace" function is useful for reformatting Gemini output:
      {{ replace "▬▬▬▬▬▬▬" "\n\n▬▬▬▬▬▬▬\n\n" $json.response_text }}

`)
		},
	}
}

// ── ref examples ──────────────────────────────────────────────────────────────

func refExamplesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "examples",
		Short: "Common workflow patterns and use cases",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(`
╔══════════════════════════════════════════════════════════════╗
║               monoes — common workflow examples              ║
╚══════════════════════════════════════════════════════════════╝

──────────────────────────────────────────────────────────────
1. DAILY NEWS → PERSIAN TRANSLATION → INSTAGRAM POST
──────────────────────────────────────────────────────────────
Nodes (in order):
  trigger.schedule      cron: "0 0 9 * * *"
  system.rss_read       url: DW/Spiegel/Zeit feeds  (3 parallel nodes)
  core.merge            combine all feed items
  core.aggregate        collect titles + descriptions into arrays
  core.set              build Gemini prompt string
  gemini.generate_text  translate/summarize to Persian
  core.set              build short English image prompt
  gemini.generate_image generate news collage image
  core.set              extract media_path + build caption
  instagram.publish_post upload image + caption

Key tips:
  - Image prompt must be SHORT English text (< 100 chars)
  - Use replace to inject \n\n around separators in Gemini output
  - media in instagram.publish_post must be a native JSON array

──────────────────────────────────────────────────────────────
2. BULK INSTAGRAM DM CAMPAIGN
──────────────────────────────────────────────────────────────
  monoes login instagram
  monoes people import leads.csv
  monoes message instagram --list leads --text "Hi {{name}}! ..."

Or as a workflow:
  trigger.schedule → people.lookup → instagram.send_dms

──────────────────────────────────────────────────────────────
3. INSTAGRAM ENGAGEMENT LOOP
──────────────────────────────────────────────────────────────
  monoes login instagram
  monoes run ENGAGE_WITH_POSTS --platform instagram --list target-posts

Or workflow:
  trigger.schedule → instagram.find_by_keyword → instagram.engage_with_posts

──────────────────────────────────────────────────────────────
4. RSS → SLACK DIGEST
──────────────────────────────────────────────────────────────
  trigger.schedule   →  system.rss_read  →  core.limit (5)
    →  core.aggregate (titles into array)
    →  core.set (build Slack message)
    →  comm.slack

──────────────────────────────────────────────────────────────
5. WEB SCRAPE → GOOGLE SHEETS
──────────────────────────────────────────────────────────────
  trigger.schedule  →  http.request  →  ai.extract_page
    →  service.google_sheets (append rows)

──────────────────────────────────────────────────────────────
6. TEST A SINGLE NODE QUICKLY
──────────────────────────────────────────────────────────────
  monoes node run system.rss_read \
    --config '{"url":"https://rss.dw.com/rdf/rss-en-all","limit":3}'

  # Gemini nodes use BROWSER AUTOMATION — no API key required.
  # Run 'monoes login gemini' once to authenticate, then:
  monoes node run gemini.generate_text \
    --config '{"prompt":"Say hello in Persian"}'

  monoes node run gemini.generate_image \
    --config '{"prompt":"sunset over a mountain lake","downloadDir":"~/.monoes/downloads"}'

──────────────────────────────────────────────────────────────
7. TROUBLESHOOTING CHECKLIST
──────────────────────────────────────────────────────────────
  "workflow not found"
    → file must be named exactly <workflow-id>.json in ~/.monoes/workflows/

  "invalid cron spec"
    → use 6-field cron: "0 0 9 * * *"  (sec min hour day month weekday)

  "I need a Gemini API key"
    → WRONG. monoes uses BROWSER AUTOMATION, not the API. No key needed.
    → Run: monoes login gemini   (one-time, opens Chrome for Google login)
    → Then use gemini.generate_image with just "prompt" — no credential_id required.

  Gemini images not generating
    → use short English prompts (< 100 chars); long/Persian prompts trigger reasoning mode
    → check session is alive: monoes login status

  Instagram posting as comment instead of new post
    → media must be a native JSON array: [{"url": "{{ $json.media_path }}"}]
    → ensure media_path is a plain string field set by core.set first

  Extension port conflict
    → only one monoes process at a time (CLI and Wails share port 9222)

`)
		},
	}
}

// max returns the larger of two ints (Go 1.21 builtin not always available).
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
