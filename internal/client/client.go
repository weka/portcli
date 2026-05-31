package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/weka/portcli/internal/config"
)

type Client struct {
	baseURL    string
	clientID   string
	clientSecret string
	token      string
	http       *http.Client
}

func New(cfg *config.Config) *Client {
	return &Client{
		baseURL:      cfg.BaseURL,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) authenticate() error {
	body, _ := json.Marshal(map[string]string{
		"clientId":     c.clientID,
		"clientSecret": c.clientSecret,
	})
	resp, err := c.http.Post(c.baseURL+"/v1/auth/access_token", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed (status %d): %s", resp.StatusCode, data)
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}
	c.token = result.AccessToken
	return nil
}

func (c *Client) doRequest(method, path string, body any) ([]byte, error) {
	if c.token == "" {
		if err := c.authenticate(); err != nil {
			return nil, err
		}
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, respBody)
	}
	return respBody, nil
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
	Blueprint string `json:"blueprint"`
	Identifier string `json:"identifier"`
}

// GetLinkedEntities parses the link field from a run into entity links.
func (ar *ActionRun) GetLinkedEntities() []EntityLink {
	var links []EntityLink
	_ = json.Unmarshal(ar.Run.Link, &links)
	return links
}

// RunLog represents a log entry from an action run.
type RunLog struct {
	ID        string `json:"id"`
	RunID     string `json:"runId"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

// GetRunLogs fetches logs for an action run.
func (c *Client) GetRunLogs(runID string) ([]RunLog, error) {
	data, err := c.doRequest("GET", "/v1/actions/runs/"+runID+"/logs", nil)
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
	} `json:"schema"`
}

// GetBlueprint fetches a single blueprint by identifier.
func (c *Client) GetBlueprint(identifier string) (*BlueprintDetail, error) {
	data, err := c.doRequest("GET", "/v1/blueprints/"+url.PathEscape(identifier), nil)
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
func (c *Client) ListBlueprints() ([]Blueprint, error) {
	data, err := c.doRequest("GET", "/v1/blueprints", nil)
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

// EntitySummary is a minimal representation of an entity used for listing.
type EntitySummary struct {
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
}

// SearchEntities lists all entities for a given blueprint.
func (c *Client) SearchEntities(blueprint string) ([]EntitySummary, error) {
	data, err := c.doRequest("GET", "/v1/blueprints/"+url.PathEscape(blueprint)+"/entities", nil)
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

// SearchEntitiesWithFilter lists entities for a blueprint filtered by a property value.
func (c *Client) SearchEntitiesWithFilter(blueprint, field, value string) ([]EntitySummary, error) {
	body := map[string]any{
		"rules": []map[string]any{
			{
				"property": "$blueprint",
				"operator": "=",
				"value":    blueprint,
			},
			{
				"property": field,
				"operator": "=",
				"value":    value,
			},
		},
		"combinator": "and",
	}
	data, err := c.doRequest("POST", "/v1/entities/search", body)
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

// UpdateEntityProperty updates a single property on an entity using PATCH.
func (c *Client) UpdateEntityProperty(blueprint, identifier, field string, value any) error {
	body := map[string]any{
		"properties": map[string]any{
			field: value,
		},
	}
	_, err := c.doRequest("PATCH", "/v1/blueprints/"+url.PathEscape(blueprint)+"/entities/"+url.PathEscape(identifier), body)
	return err
}

// DeleteEntity deletes a catalog entity by blueprint and identifier.
func (c *Client) DeleteEntity(blueprint, identifier string) error {
	path := fmt.Sprintf("/v1/blueprints/%s/entities/%s", url.PathEscape(blueprint), url.PathEscape(identifier))
	_, err := c.doRequest("DELETE", path, nil)
	return err
}

// GetEntity fetches a catalog entity by blueprint and identifier.
func (c *Client) GetEntity(blueprint, identifier string) (*Entity, error) {
	data, err := c.doRequest("GET", "/v1/blueprints/"+url.PathEscape(blueprint)+"/entities/"+url.PathEscape(identifier), nil)
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
func (c *Client) ExecuteAction(actionID string, properties map[string]any, runAs, entity, identifier string) (*ActionRun, error) {
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

	data, err := c.doRequest("POST", path, body)
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
func (c *Client) GetActionRun(runID string) (*ActionRun, error) {
	data, err := c.doRequest("GET", "/v1/actions/runs/"+runID, nil)
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
func (c *Client) WaitForRun(runID string, pollInterval, timeout time.Duration) (*ActionRun, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run, err := c.GetActionRun(runID)
		if err != nil {
			return nil, err
		}
		if run.Run.Status != "IN_PROGRESS" {
			return run, nil
		}
		time.Sleep(pollInterval)
	}
	return nil, fmt.Errorf("timed out waiting for run %s after %s", runID, timeout)
}
