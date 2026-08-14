package icot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/authoring/promptcontext"
)

func TestDiscoverRemoteSourcesUsesOneBoundedAPIsGuruLookup(t *testing.T) {
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/list.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"quasar.example": map[string]any{
					"preferred": "1.0.0",
					"versions": map[string]any{"1.0.0": map[string]any{
						"swaggerUrl": "http://" + r.Host + "/openapi.json",
						"info":       map[string]any{"title": "Quasar Widget API"},
					}},
				},
			})
		case "/openapi.json":
			_, _ = w.Write([]byte(`{"openapi":"3.0.0","info":{"title":"Quasar Widget API","version":"1"},"paths":{"/widgets":{"get":{"operationId":"listWidgets","responses":{"200":{"description":"ok"}}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := DiscoverRemoteSources(context.Background(), RemoteLookupOptions{
		Query: "quasar widgets", Client: &apitools.Client{APIsGuruListURL: server.URL + "/list.json", AllowUnsafeHosts: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests["/list.json"] != 1 {
		t.Fatalf("APIs.guru lookups = %d, want 1", requests["/list.json"])
	}
	if len(result.Blockers) != 0 || len(result.Plans) != 1 || len(result.Context.Operations) != 1 {
		t.Fatalf("remote discovery = %#v", result)
	}
	plan := result.Plans[0]
	if plan.Provenance == "" || len(plan.Content) == 0 || plan.Path != server.URL+"/openapi.json" {
		t.Fatalf("remote plan = %#v", plan)
	}
	if len(result.Plans) > RemoteLookupMaxCandidates {
		t.Fatalf("candidate count = %d", len(result.Plans))
	}
}

func TestDiscoverRemoteSourcesRejectsUnsafeHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	result, err := DiscoverRemoteSources(context.Background(), RemoteLookupOptions{
		Query: "quasar widgets", Client: &apitools.Client{APIsGuruListURL: server.URL + "/list.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Blockers) != 1 {
		t.Fatalf("unsafe lookup = %#v", result)
	}
	message := strings.ToLower(result.Blockers[0].Message)
	if !strings.Contains(message, "unsafe") && !strings.Contains(message, "private") {
		t.Fatalf("unsafe lookup = %#v", result)
	}
}

func TestDiscoverRemoteSourcesTimeoutIsDeferableBlocker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	result, err := DiscoverRemoteSources(context.Background(), RemoteLookupOptions{
		Query: "quasar widgets", Timeout: 20 * time.Millisecond,
		Client: &apitools.Client{APIsGuruListURL: server.URL + "/list.json", AllowUnsafeHosts: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Blockers) != 1 || result.Blockers[0].Code != "ramen.icot.remote_lookup_timeout" || !result.Blockers[0].Deferrable {
		t.Fatalf("timeout lookup = %#v", result)
	}
}

func TestNetworkAskFrontierRequiresExplicitRemoteApproval(t *testing.T) {
	session := SeedSession("Manage quasar widgets", "", t.TempDir(), "ask", nil, nil, promptcontext.Context{Version: promptcontext.Version})
	frontier, err := PlanFrontier(session)
	if err != nil {
		t.Fatal(err)
	}
	for _, question := range frontier {
		if question.ID == nodeSourceInput {
			if !question.Forced || !strings.Contains(question.Prompt, "answer remote") {
				t.Fatalf("source question = %#v", question)
			}
			return
		}
	}
	t.Fatalf("source approval missing from frontier: %#v", frontier)
}
