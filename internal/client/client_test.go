package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/weka/portcli/internal/config"
)

// newTestClient points a client at ts and pre-seeds a token so tests exercise
// the endpoint under test rather than the auth handshake.
func newTestClient(ts *httptest.Server) *Client {
	c := New(&config.Config{BaseURL: ts.URL, ClientID: "id", ClientSecret: "secret"})
	c.token = "test-token"
	return c
}

// Port sends an action input's enum either as a list of choices or as a
// {"jqQuery": …} object it evaluates server-side. Decoding the object form into
// a concrete Go type used to fail the entire action fetch, which also disabled
// UPSERT_ENTITY verification, because verifyUpsert reads any GetAction error as
// "not an upsert".
func TestGetActionDecodesDynamicEnum(t *testing.T) {
	const body = `{"ok":true,"action":{
		"identifier":"deploy","title":"Deploy",
		"trigger":{"operation":"CREATE","blueprintIdentifier":"service","userInputs":{
			"properties":{
				"cloud":{"type":"string","enum":["aws","gcp"]},
				"template":{"type":"string","enum":{"jqQuery":".entity.properties.templates"}},
				"name":{"type":"string"},
				"nulled":{"type":"string","enum":null}
			},
			"required":["cloud"],"order":["cloud","template","name","nulled"]}},
		"invocationMethod":{"type":"UPSERT_ENTITY","blueprintIdentifier":"service"}}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/v1/actions/deploy" {
			t.Errorf("path = %q, want /v1/actions/deploy", got)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	action, err := newTestClient(ts).GetAction("deploy")
	if err != nil {
		t.Fatalf("GetAction returned %v; a jq-driven enum must not fail the whole action", err)
	}

	props := action.Trigger.UserInputs.Properties
	if action.InvocationMethod.Type != "UPSERT_ENTITY" {
		t.Errorf("InvocationMethod.Type = %q, want UPSERT_ENTITY", action.InvocationMethod.Type)
	}

	t.Run("static enum", func(t *testing.T) {
		vals, dynamic := props["cloud"].EnumValues()
		if dynamic {
			t.Error("dynamic = true for a fixed list")
		}
		if len(vals) != 2 || vals[0] != "aws" || vals[1] != "gcp" {
			t.Errorf("values = %v, want [aws gcp]", vals)
		}
	})

	t.Run("jq-driven enum", func(t *testing.T) {
		vals, dynamic := props["template"].EnumValues()
		if !dynamic {
			t.Error("dynamic = false; server-side values are not knowable")
		}
		if vals != nil {
			t.Errorf("values = %v, want nil for a dynamic enum", vals)
		}
	})

	// An absent enum and an explicit null are both "no choices", not dynamic —
	// `"enum": null` decodes to the four bytes "null", not a nil slice.
	for _, name := range []string{"name", "nulled"} {
		t.Run("no enum/"+name, func(t *testing.T) {
			vals, dynamic := props[name].EnumValues()
			if dynamic {
				t.Error("dynamic = true, want false")
			}
			if len(vals) != 0 {
				t.Errorf("values = %v, want none", vals)
			}
		})
	}
}

// `action get --json` marshals ActionInput directly, so the Enum field's Go
// type is part of the CLI's output contract. A nil json.RawMessage must still
// marshal to null exactly as the previous []any did.
func TestActionInputJSONRoundTripsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input ActionInput
		want  string
	}{
		{"absent enum", ActionInput{Type: "string"}, `{"type":"string","title":"","description":"","default":null,"enum":null}`},
		{"static enum", ActionInput{Type: "string", Enum: json.RawMessage(`["a","b"]`)}, `{"type":"string","title":"","description":"","default":null,"enum":["a","b"]}`},
		{"jq enum", ActionInput{Type: "string", Enum: json.RawMessage(`{"jqQuery":".x"}`)}, `{"type":"string","title":"","description":"","default":null,"enum":{"jqQuery":".x"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestListActionsDecodesFullObjects(t *testing.T) {
	const body = `{"ok":true,"actions":[
		{"identifier":"a1","title":"One","trigger":{"operation":"CREATE","blueprintIdentifier":"svc",
			"userInputs":{"properties":{"x":{"type":"string","enum":{"jqQuery":".y"}}},"required":["x"],"order":["x"]}},
		 "invocationMethod":{"type":"UPSERT_ENTITY","blueprintIdentifier":"svc","mapping":{"identifier":"{{ .inputs.x }}"}}},
		{"identifier":"a2","title":"Two","trigger":{"operation":"DAY-2","blueprintIdentifier":"svc",
			"userInputs":{"properties":{},"required":[],"order":[]}},
		 "invocationMethod":{"type":"WEBHOOK"}}]}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("version"); got != "v2" {
			t.Errorf("version = %q, want v2", got)
		}
		if got := r.URL.Query().Get("trigger_type"); got != "self-service" {
			t.Errorf("trigger_type = %q, want self-service", got)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	actions, err := newTestClient(ts).ListActions()
	if err != nil {
		t.Fatalf("ListActions: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(actions))
	}

	// The whole point of using the list endpoint is that a row already carries
	// everything the run form and upsert verification need.
	if got := actions[0].InvocationMethod.Mapping.Identifier; got != "{{ .inputs.x }}" {
		t.Errorf("mapping identifier = %q, want the input template", got)
	}
	if got := actions[0].Trigger.UserInputs.Order; len(got) != 1 || got[0] != "x" {
		t.Errorf("order = %v, want [x]", got)
	}
	if _, dynamic := actions[0].Trigger.UserInputs.Properties["x"].EnumValues(); !dynamic {
		t.Error("a jq enum in the list payload must decode, not fail the batch")
	}
}

func TestGetRunLogsFromBuildsOffsetQuery(t *testing.T) {
	for _, tc := range []struct {
		name          string
		offset, limit int
		wantQuery     string
	}{
		{"no paging", 0, 0, ""},
		{"offset only", 12, 0, "offset=12"},
		{"offset and limit", 12, 50, "limit=50&offset=12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.RawQuery; got != tc.wantQuery {
					t.Errorf("query = %q, want %q", got, tc.wantQuery)
				}
				if got := r.URL.Path; got != "/v1/actions/runs/r_1/logs" {
					t.Errorf("path = %q", got)
				}
				_, _ = w.Write([]byte(`{"ok":true,"runLogs":[{"id":"l1","message":"hi"}]}`))
			}))
			defer ts.Close()

			logs, err := newTestClient(ts).GetRunLogsFrom("r_1", tc.offset, tc.limit)
			if err != nil {
				t.Fatalf("GetRunLogsFrom: %v", err)
			}
			if len(logs) != 1 || logs[0].Message != "hi" {
				t.Errorf("logs = %v", logs)
			}
		})
	}
}
