package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weka/portcli/internal/config"
)

type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	http         *http.Client

	// authMu guards token. It is held across the authentication round-trip on
	// purpose: a burst of concurrent requests — what a TUI produces on every
	// view switch — should cost one authentication, not one per request.
	// Readers hold it only long enough to copy a string.
	authMu sync.Mutex
	token  string
}

func New(cfg *config.Config) *Client {
	return &Client{
		baseURL:      cfg.BaseURL,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

// bearer returns a usable token, authenticating on first use. Any existing
// token will do, which is what passing an empty stale token asks for.
func (c *Client) bearer(ctx context.Context) (string, error) {
	return c.tokenOtherThan(ctx, "")
}

// refresh replaces a token the API rejected.
func (c *Client) refresh(ctx context.Context, rejected string) (string, error) {
	return c.tokenOtherThan(ctx, rejected)
}

// tokenOtherThan returns the cached token unless it is stale, authenticating
// otherwise. Comparing against the token the caller actually sent is what
// makes N simultaneous 401s cost one authentication between them: whichever
// goroutine gets the lock first replaces the token, and the rest see a value
// that is no longer the one they were rejected for.
func (c *Client) tokenOtherThan(ctx context.Context, stale string) (string, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.token != "" && c.token != stale {
		return c.token, nil
	}
	return c.authLocked(ctx)
}

// authLocked performs the client-credentials exchange. The caller holds authMu.
func (c *Client) authLocked(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"clientId":     c.clientID,
		"clientSecret": c.clientSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/auth/access_token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("auth failed (status %d): %s", resp.StatusCode, data)
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse auth response: %w", err)
	}
	c.token = result.AccessToken
	return c.token, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	// Marshal once. The request is replayed if the token turns out to be
	// expired, and an io.Reader cannot be rewound — reusing one would send an
	// empty body on the second attempt, which the API accepts.
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}

	token, err := c.bearer(ctx)
	if err != nil {
		return nil, err
	}

	// Port access tokens expire (~1.7h) and nothing announces it. A one-shot
	// CLI run never notices; a session left open does. Treat one 401 as "the
	// token aged out", swap it and replay. A second 401 means the credentials
	// themselves are wrong, and is reported as-is.
	for attempt := 0; ; attempt++ {
		data, status, err := c.send(ctx, method, path, payload, token)
		if err != nil {
			return nil, err
		}
		if status == http.StatusUnauthorized && attempt == 0 {
			if token, err = c.refresh(ctx, token); err != nil {
				return nil, err
			}
			continue
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("API error (status %d): %s", status, data)
		}
		return data, nil
	}
}

// send performs one attempt, returning the body and status rather than an
// error for a non-2xx, so doRequest can decide whether to retry.
func (c *Client) send(ctx context.Context, method, path string, payload []byte, token string) ([]byte, int, error) {
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return data, resp.StatusCode, nil
}

// RunSummary represents a single run entry returned by the list runs endpoint.
type RunSummary struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"createdAt"`
	EndedAt   *string `json:"endedAt"`
	Action    struct {
		Identifier string `json:"identifier"`
		Title      string `json:"title"`
	} `json:"action"`
	Blueprint struct {
		Identifier string `json:"identifier"`
	} `json:"blueprint"`
}

// ListActionRuns fetches action runs, optionally filtered by entity, blueprint, and limit.
func (c *Client) ListActionRuns(ctx context.Context, entity, blueprint string, limit int) ([]RunSummary, error) {
	params := url.Values{}
	if entity != "" {
		params.Set("entity", entity)
	}
	if blueprint != "" {
		params.Set("blueprint", blueprint)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	path := "/v1/actions/runs"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Runs []RunSummary `json:"runs"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse runs response: %w", err)
	}
	return result.Runs, nil
}

// ActionRun represents the result of triggering an action.
type ActionRun struct {
	OK  bool `json:"ok"`
	Run struct {
		ID         string          `json:"id"`
		Status     string          `json:"status"`
		Action     json.RawMessage `json:"action"`
		EndedAt    *string         `json:"endedAt"`
		Source     string          `json:"source"`
		Properties map[string]any  `json:"properties"`
		Link       json.RawMessage `json:"link"`
		CreatedAt  string          `json:"createdAt"`
		UpdatedAt  string          `json:"updatedAt"`
		CreatedBy  string          `json:"createdBy"`
		Blueprint  struct {
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
		} `json:"blueprint"`
	} `json:"run"`
}

// EntityLink represents a link to an entity from a run.
type EntityLink struct {
	Blueprint  string `json:"blueprint"`
	Identifier string `json:"identifier"`
}

// GetLinkedEntities parses the link field from a run into entity links.
func (ar *ActionRun) GetLinkedEntities() []EntityLink {
	var links []EntityLink
	_ = json.Unmarshal(ar.Run.Link, &links)
	return links
}

// ResolvedLink pairs a run's entity link with the entity it points at, or with
// the error from fetching it.
type ResolvedLink struct {
	Link   EntityLink
	Entity *Entity
	Err    error
}

// ResolveLinks fetches every entity a run links to. Failures ride along per
// link rather than aborting, so one unreadable link does not hide the others.
func (c *Client) ResolveLinks(ctx context.Context, run *ActionRun) []ResolvedLink {
	links := run.GetLinkedEntities()
	out := make([]ResolvedLink, 0, len(links))
	for _, link := range links {
		entity, err := c.GetEntity(ctx, link.Blueprint, link.Identifier)
		out = append(out, ResolvedLink{Link: link, Entity: entity, Err: err})
	}
	return out
}

// RunLog represents a log entry from an action run.
type RunLog struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

// GetRunLogs fetches logs for an action run.
func (c *Client) GetRunLogs(ctx context.Context, runID string) ([]RunLog, error) {
	return c.GetRunLogsFrom(ctx, runID, 0, 0)
}

// GetRunLogsFrom fetches logs for an action run starting at offset. Passing the
// number of lines already seen turns a repeated poll into an incremental tail
// rather than a full refetch. offset and limit are omitted when non-positive.
func (c *Client) GetRunLogsFrom(ctx context.Context, runID string, offset, limit int) ([]RunLog, error) {
	params := url.Values{}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	path := "/v1/actions/runs/" + url.PathEscape(runID) + "/logs"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		RunLogs []RunLog `json:"runLogs"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse logs response: %w", err)
	}
	return result.RunLogs, nil
}

// Blueprint represents a Port blueprint.
type Blueprint struct {
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// BlueprintProperty represents a single property in a blueprint schema.
type BlueprintProperty struct {
	Type  string `json:"type"`
	Title string `json:"title"`
}

// BlueprintDetail represents a Port blueprint with its schema properties.
type BlueprintDetail struct {
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	Schema     struct {
		Properties map[string]BlueprintProperty `json:"properties"`
		Required   []string                     `json:"required"`
	} `json:"schema"`
}

// GetBlueprint fetches a single blueprint by identifier.
func (c *Client) GetBlueprint(ctx context.Context, identifier string) (*BlueprintDetail, error) {
	data, err := c.doRequest(ctx, "GET", "/v1/blueprints/"+url.PathEscape(identifier), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Blueprint BlueprintDetail `json:"blueprint"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse blueprint response: %w", err)
	}
	return &result.Blueprint, nil
}

// ListBlueprints fetches all blueprints.
func (c *Client) ListBlueprints(ctx context.Context) ([]Blueprint, error) {
	data, err := c.doRequest(ctx, "GET", "/v1/blueprints", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Blueprints []Blueprint `json:"blueprints"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse blueprints response: %w", err)
	}
	return result.Blueprints, nil
}

// Entity represents a Port catalog entity.
type Entity struct {
	OK     bool `json:"ok"`
	Entity struct {
		Identifier string         `json:"identifier"`
		Title      string         `json:"title"`
		Blueprint  string         `json:"blueprint"`
		Properties map[string]any `json:"properties"`
		Relations  map[string]any `json:"relations"`
		CreatedAt  string         `json:"createdAt"`
		UpdatedAt  string         `json:"updatedAt"`
		CreatedBy  string         `json:"createdBy"`
	} `json:"entity"`
}

// IsNotFound reports whether err is a 404 from the API. doRequest carries the
// status in the formatted message, so the check lives beside the formatting
// rather than leaving callers to match on the string themselves.
func IsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "(status 404)")
}

// IsUnauthorized reports whether err came from rejected credentials — either
// the token exchange itself failing, or a request still 401ing after the
// automatic re-authentication. Nothing will work until the credentials change,
// which is worth telling a user plainly rather than retrying.
func IsUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "(status 401)") || strings.Contains(msg, "auth failed (status")
}

// ErrPollTimeout reports that PollEntity gave up waiting. Callers format the
// user-facing message themselves, since only they know what they waited for.
var ErrPollTimeout = errors.New("timed out polling entity")

// PollEntity fetches an entity until check is satisfied or timeout expires.
// A failed fetch is handed to check rather than ending the poll: callers are
// usually waiting for an entity to appear, so "not found" is the expected
// start state. Only check can stop early, by returning true or an error.
func (c *Client) PollEntity(
	ctx context.Context,
	blueprint, identifier string,
	timeout, interval time.Duration,
	check func(*Entity, error) (bool, error),
) (*Entity, error) {
	deadline := time.Now().Add(timeout)
	for {
		entity, fetchErr := c.GetEntity(ctx, blueprint, identifier)
		done, err := check(entity, fetchErr)
		if err != nil {
			return nil, err
		}
		if done {
			return entity, nil
		}
		if time.Now().After(deadline) {
			return nil, ErrPollTimeout
		}
		// Sleeping on the context rather than the clock is the difference
		// between a cancellation landing now and landing one interval from now.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// EntitySummary is a representation of an entity used for listing.
type EntitySummary struct {
	Identifier string         `json:"identifier"`
	Title      string         `json:"title"`
	Properties map[string]any `json:"properties"`
	CreatedAt  string         `json:"createdAt"`
	CreatedBy  string         `json:"createdBy"`
}

// SearchEntities lists all entities for a given blueprint.
func (c *Client) SearchEntities(ctx context.Context, blueprint string) ([]EntitySummary, error) {
	data, err := c.doRequest(ctx, "GET", "/v1/blueprints/"+url.PathEscape(blueprint)+"/entities", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Entities []EntitySummary `json:"entities"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse entities response: %w", err)
	}
	return result.Entities, nil
}

// metaFields are an entity's top-level fields, which the search API addresses
// with a "$" prefix. It treats an unprefixed "identifier" as a regular
// property and quietly matches nothing, so a filter on one of these returns
// zero results for entities that plainly exist unless it is translated.
var metaFields = map[string]bool{
	"identifier": true,
	"title":      true,
	"blueprint":  true,
	"team":       true,
	"createdAt":  true,
	"updatedAt":  true,
	"createdBy":  true,
	"updatedBy":  true,
}

// searchProperty translates a filter field into the property name the search
// API expects, leaving ordinary properties and already-prefixed names alone.
func searchProperty(field string) string {
	if strings.HasPrefix(field, "$") {
		return field
	}
	if metaFields[field] {
		return "$" + field
	}
	return field
}

// SearchEntitiesWithFilter lists entities for a blueprint filtered by a property value.
func (c *Client) SearchEntitiesWithFilter(ctx context.Context, blueprint, field, value string) ([]EntitySummary, error) {
	body := map[string]any{
		"rules": []map[string]any{
			{
				"property": "$blueprint",
				"operator": "=",
				"value":    blueprint,
			},
			{
				"property": searchProperty(field),
				"operator": "=",
				"value":    value,
			},
		},
		"combinator": "and",
	}
	data, err := c.doRequest(ctx, "POST", "/v1/entities/search", body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Entities []EntitySummary `json:"entities"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}
	return result.Entities, nil
}

// UpdateEntityProperties updates properties on an entity using PATCH.
func (c *Client) UpdateEntityProperties(ctx context.Context, blueprint, identifier string, properties map[string]any) error {
	body := map[string]any{
		"properties": properties,
	}
	_, err := c.doRequest(ctx, "PATCH", "/v1/blueprints/"+url.PathEscape(blueprint)+"/entities/"+url.PathEscape(identifier), body)
	return err
}

// DeleteEntity deletes a catalog entity by blueprint and identifier.
func (c *Client) DeleteEntity(ctx context.Context, blueprint, identifier string) error {
	path := fmt.Sprintf("/v1/blueprints/%s/entities/%s", url.PathEscape(blueprint), url.PathEscape(identifier))
	_, err := c.doRequest(ctx, "DELETE", path, nil)
	return err
}

// GetEntity fetches a catalog entity by blueprint and identifier.
func (c *Client) GetEntity(ctx context.Context, blueprint, identifier string) (*Entity, error) {
	data, err := c.doRequest(ctx, "GET", "/v1/blueprints/"+url.PathEscape(blueprint)+"/entities/"+url.PathEscape(identifier), nil)
	if err != nil {
		return nil, err
	}

	var entity Entity
	if err := json.Unmarshal(data, &entity); err != nil {
		return nil, fmt.Errorf("failed to parse entity response: %w", err)
	}
	return &entity, nil
}

// ExecuteAction triggers a self-service action and returns the run.
// If runAs is non-empty, the action is executed on behalf of that user email.
// If entity is non-empty, the action targets an existing entity (day-2 action).
func (c *Client) ExecuteAction(ctx context.Context, actionID string, properties map[string]any, runAs, entity, identifier string) (*ActionRun, error) {
	props := make(map[string]any, len(properties)+1)
	for k, v := range properties {
		props[k] = v
	}
	if identifier != "" {
		props["identifier"] = identifier
	}

	body := map[string]any{
		"properties": props,
	}
	if entity != "" {
		body["entity"] = entity
	}

	path := "/v1/actions/" + actionID + "/runs"
	if runAs != "" {
		path += "?run_as=" + url.QueryEscape(runAs)
	}

	data, err := c.doRequest(ctx, "POST", path, body)
	if err != nil {
		return nil, err
	}

	var run ActionRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("failed to parse run response: %w", err)
	}
	return &run, nil
}

// GetActionRun fetches the current state of an action run.
func (c *Client) GetActionRun(ctx context.Context, runID string) (*ActionRun, error) {
	data, err := c.doRequest(ctx, "GET", "/v1/actions/runs/"+runID, nil)
	if err != nil {
		return nil, err
	}

	var run ActionRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("failed to parse run response: %w", err)
	}
	return &run, nil
}

// WaitForRun polls until the run completes or the timeout is reached.
func (c *Client) WaitForRun(ctx context.Context, runID string, pollInterval, timeout time.Duration) (*ActionRun, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run, err := c.GetActionRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		if run.Run.Status != "IN_PROGRESS" {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return nil, fmt.Errorf("timed out waiting for run %s after %s", runID, timeout)
}

// ActionInput describes a single user input of a self-service action.
//
// Enum is deliberately raw. Port sends it either as a list of choices or as a
// {"jqQuery": "…"} object it evaluates server-side, and decoding the object
// form into a concrete Go type fails the whole action — which silently
// disabled UPSERT_ENTITY verification, since verifyUpsert treats any GetAction
// error as "not an upsert". Read it through EnumValues.
//
// The json tag stays bare: a nil json.RawMessage marshals to null exactly as
// the previous []any did, so `action get --json` is unchanged.
type ActionInput struct {
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Default     any             `json:"default"`
	Enum        json.RawMessage `json:"enum"`
}

// EnumValues returns the input's choices, distinguishing the three shapes Port
// uses: no enum at all (nil, false), a fixed list (values, false), and a query
// Port evaluates server-side whose results we cannot know (nil, true). A
// dynamic enum means the caller must accept free text.
func (i ActionInput) EnumValues() (values []any, dynamic bool) {
	if isJSONNull(i.Enum) {
		return nil, false
	}
	if json.Unmarshal(i.Enum, &values) != nil {
		return nil, true
	}
	return values, false
}

// isJSONNull reports whether raw JSON is absent or the literal null — decoding
// `"enum": null` yields the bytes "null", not a nil slice.
func isJSONNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || string(trimmed) == "null"
}

// ActionDetail represents a Port self-service action and its input schema.
//
// Fields may be added freely: `action get --json` marshals a curated map of
// selected values, not this struct, so its output does not widen when this
// does. ActionInput is the opposite case — that one *is* marshalled directly.
type ActionDetail struct {
	Identifier       string `json:"identifier"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Publish          bool   `json:"publish"`
	RequiredApproval bool   `json:"requiredApproval"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
	Trigger          struct {
		BlueprintIdentifier string `json:"blueprintIdentifier"`
		Operation           string `json:"operation"`
		UserInputs          struct {
			Properties map[string]ActionInput `json:"properties"`
			Required   []string               `json:"required"`
			Order      []string               `json:"order"`
		} `json:"userInputs"`
	} `json:"trigger"`
	InvocationMethod struct {
		Type                string `json:"type"`
		BlueprintIdentifier string `json:"blueprintIdentifier"`
		Mapping             struct {
			Identifier string `json:"identifier"`
			// Values are templates, but a mapping may nest objects or arrays,
			// so decode loosely rather than failing the whole action fetch.
			Properties map[string]any `json:"properties"`
		} `json:"mapping"`
	} `json:"invocationMethod"`
}

// GetAction fetches a self-service action and its input schema by identifier.
func (c *Client) GetAction(ctx context.Context, identifier string) (*ActionDetail, error) {
	data, err := c.doRequest(ctx, "GET", "/v1/actions/"+url.PathEscape(identifier), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Action ActionDetail `json:"action"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse action response: %w", err)
	}
	return &result.Action, nil
}

// ListActions fetches every self-service action with its full schema. Port
// returns complete action objects here — trigger inputs and invocation mapping
// included — so one call answers what would otherwise be a GetAction per row.
//
// version=v2 is explicit rather than defaulted: the trigger shape differs
// between versions, and pinning it keeps a server-side default change from
// silently reshaping what we decode.
func (c *Client) ListActions(ctx context.Context) ([]ActionDetail, error) {
	data, err := c.doRequest(ctx, "GET", "/v1/actions?trigger_type=self-service&version=v2", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Actions []ActionDetail `json:"actions"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse actions response: %w", err)
	}
	return result.Actions, nil
}
