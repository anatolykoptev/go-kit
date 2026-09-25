package wowa

import "encoding/json"

// FetchRequest is the POST /api/v1/fetch body (proxied to ox-browser /fetch).
// Method defaults to GET, or POST when Body is set. TimeoutSecs maps to the
// wire field "timeout" — the canonical name on the ox side; it bounds the
// WHOLE server-side call (retry loop + solver escalation + rate-limit wait).
type FetchRequest struct {
	URL         string            `json:"url"`
	Method      string            `json:"method,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	TimeoutSecs int               `json:"timeout,omitempty"`
}

// FetchResponse is the /api/v1/fetch response. Status is the UPSTREAM page
// status — a 404 page is a legitimate fetch result, not a client error.
// Error carries the remote failure string on transport/deadline failures
// (those also arrive with HTTP 502/504, handled before decode).
type FetchResponse struct {
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	CFDetected bool              `json:"cf_detected"`
	CFType     string            `json:"cf_type,omitempty"`
	ElapsedMs  int64             `json:"elapsed_ms"`
	Error      string            `json:"error,omitempty"`
}

// RenderRequest is the POST /api/v1/render body (go-wowa stealth Chrome).
// Wait selects the page-ready strategy: "load" (server default),
// "domcontentloaded", "networkidle". TimeoutSecs is clamped server-side to
// [1, 60]; unset means the server's 20s default.
type RenderRequest struct {
	URL         string `json:"url"`
	TimeoutSecs int    `json:"timeout_secs,omitempty"`
	Wait        string `json:"wait,omitempty"`
	Proxy       string `json:"proxy,omitempty"`
}

// RenderResponse is the /api/v1/render response. go-wowa answers HTTP 200
// even on render failure — Error non-empty means the render failed.
type RenderResponse struct {
	URL       string `json:"url"`
	HTML      string `json:"html"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	ElapsedMs int64  `json:"elapsed_ms"`
	Error     string `json:"error,omitempty"`
}

// ReadRequest is the POST /api/v1/read body (proxied to ox-browser /read —
// readability-style content extraction). Format defaults to "text" on the
// server; MaxLength caps the returned content; TimeoutSecs maps to the
// wire field "timeout".
type ReadRequest struct {
	URL         string `json:"url"`
	Format      string `json:"format,omitempty"`
	MaxLength   int    `json:"max_length,omitempty"`
	TimeoutSecs int    `json:"timeout,omitempty"`
}

// ReadResponse is the /api/v1/read response. The server answers 502 with
// this same body when Error is set — do() surfaces that as RemoteError
// before decode. ExtractionNote is a bounded token naming an extraction
// fallback (e.g. "extraction_rejected_low_text_ratio") on an otherwise
// clean read — check it to tell a gate fallback from a clean extraction.
type ReadResponse struct {
	Title          string            `json:"title"`
	Content        string            `json:"content"`
	Author         string            `json:"author"`
	Excerpt        string            `json:"excerpt"`
	URL            string            `json:"url"`
	Format         string            `json:"format"`
	Length         int               `json:"length"`
	Method         string            `json:"method"`
	ElapsedMs      int64             `json:"elapsed_ms"`
	JSONLD         []json.RawMessage `json:"json_ld,omitempty"`
	OGImage        string            `json:"og_image,omitempty"`
	PublishedAt    string            `json:"published_at,omitempty"`
	ModifiedAt     string            `json:"modified_at,omitempty"`
	Section        string            `json:"section,omitempty"`
	SiteName       string            `json:"site_name,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Language       string            `json:"language,omitempty"`
	Error          string            `json:"error,omitempty"`
	ExtractionNote string            `json:"extraction_note,omitempty"`
}

// InteractRequest is the POST /api/v1/chrome/interact body. Session names a
// persistent tab across calls (empty = ephemeral); Mode is "default"
// (persistent profile), "private", or "proxy". TimeoutSecs defaults to 30
// server-side. Mirrors go-browser.InteractRequest (plus go-wowa's
// auto_bypass/max_retries extensions) — keep in sync.
type InteractRequest struct {
	URL         string   `json:"url"`
	Actions     []Action `json:"actions"`
	PreActions  []Action `json:"pre_actions,omitempty"` // run after page creation, before navigation
	TimeoutSecs int      `json:"timeout_secs,omitempty"`
	Session     string   `json:"session,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Proxy       string   `json:"proxy,omitempty"`
	NoStealth   bool     `json:"no_stealth,omitempty"`
	StealthMode bool     `json:"stealth_mode,omitempty"`
	DeadlineMs  int64    `json:"deadline_ms,omitempty"` // absolute unix-ms deadline for the whole call
	AutoBypass  bool     `json:"auto_bypass,omitempty"`
	MaxRetries  int      `json:"max_retries,omitempty"`
}

// Action is one Chrome interaction step. Type selects the verb (navigate,
// click, type_text, evaluate, wait_for, snapshot, screenshot, sleep, press,
// set_cookies, scroll, ...); the remaining fields are per-verb parameters.
// Mirrors go-browser.Action — keep in sync.
type Action struct {
	Type          string      `json:"type"`
	Selector      string      `json:"selector,omitempty"`
	Text          string      `json:"text,omitempty"`
	Script        string      `json:"script,omitempty"`
	JS            string      `json:"js,omitempty"` // alias for Script
	Key           string      `json:"key,omitempty"`
	URL           string      `json:"url,omitempty"`
	Humanize      bool        `json:"humanize,omitempty"`
	WaitMs        int         `json:"wait_ms,omitempty"`
	TimeoutMs     int         `json:"timeout_ms,omitempty"`
	Format        string      `json:"format,omitempty"`
	Cookies       []Cookie    `json:"cookies,omitempty"`
	DeltaX        float64     `json:"delta_x,omitempty"`
	DeltaY        float64     `json:"delta_y,omitempty"`
	Accept        *bool       `json:"accept,omitempty"`
	TextGone      string      `json:"text_gone,omitempty"`
	Button        string      `json:"button,omitempty"`
	DoubleClick   bool        `json:"double_click,omitempty"`
	Modifiers     []string    `json:"modifiers,omitempty"`
	Values        []string    `json:"values,omitempty"`
	Depth         int         `json:"depth,omitempty"`
	Filter        string      `json:"filter,omitempty"`
	Level         string      `json:"level,omitempty"`
	URLContains   string      `json:"url_contains,omitempty"`
	Width         int         `json:"width,omitempty"`
	Height        int         `json:"height,omitempty"`
	OutputPath    string      `json:"output_path,omitempty"`
	Quality       int         `json:"quality,omitempty"`
	Slowly        bool        `json:"slowly,omitempty"`
	Submit        bool        `json:"submit,omitempty"`
	Fields        []FormField `json:"fields,omitempty"`
	Cookie        string      `json:"cookie,omitempty"`
	Limit         int         `json:"limit,omitempty"`
	StorageType   string      `json:"storage_type,omitempty"`
	Goal          string      `json:"goal,omitempty"`
	FrameSelector string      `json:"frame_selector,omitempty"`
	SkipOnError   bool        `json:"skip_on_error,omitempty"`
	QuietMs       int         `json:"quiet_ms,omitempty"`
	MaxWaitMs     int         `json:"max_wait_ms,omitempty"`
	IgnoreHosts   []string    `json:"ignore_hosts,omitempty"`
}

// Cookie is one cookie for the set_cookies action (go-browser CookieInput).
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
}

// FormField is one field of the fill_form batch action.
type FormField struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
	Type     string `json:"type,omitempty"` // textbox (default), checkbox, combobox
}

// ActionResult is the outcome of one executed action. Data holds the
// action's return value as raw JSON (e.g. the evaluate expression's value).
type ActionResult struct {
	Action    string          `json:"action"`
	Ok        bool            `json:"ok"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
	ErrorCode string          `json:"error_code,omitempty"`
}

// InteractResponse is the /api/v1/chrome/interact response. Status is "ok"
// or "error"; on error, ErrorCode is the first failing action's
// classification. The protection/intelligence fields arrive on go-wowa's
// enriched envelope and are passed through unparsed — decode on demand.
type InteractResponse struct {
	URL                string          `json:"url"`
	Status             string          `json:"status"`
	Actions            []ActionResult  `json:"actions"`
	SessionID          string          `json:"session_id,omitempty"`
	Error              string          `json:"error,omitempty"`
	ErrorCode          string          `json:"error_code,omitempty"`
	ElapsedMs          int64           `json:"elapsed_ms"`
	ProtectionDetected json.RawMessage `json:"protection_detected,omitempty"`
	Intelligence       json.RawMessage `json:"intelligence,omitempty"`
	PostState          json.RawMessage `json:"post_state,omitempty"`
}

// ExtractRequest is the POST /api/v1/extract body — LLM structured
// extraction over a fetched page. URL and Prompt are required; Schema is an
// optional JSON Schema the output must conform to; MaxChars truncates page
// content (0 = server default cap). No wire timeout field: the extract
// pipeline carries its own internal budget.
type ExtractRequest struct {
	URL      string          `json:"url"`
	Prompt   string          `json:"prompt"`
	Schema   json.RawMessage `json:"schema,omitempty"`
	MaxChars int             `json:"max_chars,omitempty"`
}

// ExtractResponse is the /api/v1/extract response. Data is the extracted
// JSON verbatim (decode per Schema); SourceMethod names the fetch path;
// Attempts counts fetch+extract rounds; Chunks >0 means multi-chunk mode.
type ExtractResponse struct {
	Data         json.RawMessage `json:"data"`
	SourceMethod string          `json:"source_method"`
	CharsUsed    int             `json:"chars_used"`
	Attempts     int             `json:"attempts"`
	Chunks       int             `json:"chunks,omitempty"`
}
