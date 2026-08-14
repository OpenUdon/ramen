package icot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/apitools/catalog"
	"github.com/OpenUdon/authoring/promptcontext"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
)

const (
	RemoteLookupTimeout       = 8 * time.Second
	RemoteLookupMaxCandidates = 3
)

// RemoteLookupOptions keeps remote source discovery separate from local
// scanning. Client and Catalog are injectable so all tests can remain local.
type RemoteLookupOptions struct {
	Query   string
	Client  *apitools.Client
	Catalog catalog.Catalog
	Timeout time.Duration
}

type remoteReference struct {
	ID         string
	Title      string
	URL        string
	Provenance string
	Score      int
}

// DiscoverRemoteSources performs one bounded APIs.guru search alongside
// matching curated apitools catalog references. It downloads at most three
// OpenAPI documents under one eight-second deadline, validates them through
// apitools, and retains their bytes only in resumable interview state.
func DiscoverRemoteSources(ctx context.Context, opts RemoteLookupOptions) (DiscoveryResult, error) {
	query := strings.TrimSpace(opts.Query)
	if len(query) < 2 {
		return DiscoveryResult{Blockers: []Blocker{remoteLookupBlocker("ramen.icot.remote_lookup_empty", "The active outcome is too short to search safely.")}}, nil
	}
	timeout := opts.Timeout
	if timeout <= 0 || timeout > RemoteLookupTimeout {
		timeout = RemoteLookupTimeout
	}
	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := opts.Client
	if client == nil {
		client = &apitools.Client{}
	}
	clientCopy := *client
	clientCopy.Timeout = timeout
	clientCopy.MaxBytes = apitools.DefaultMaxBytes
	client = &clientCopy

	cat := opts.Catalog
	if len(cat.Providers) == 0 {
		cat = catalog.BuiltInCatalog()
	}
	curated := curatedRemoteReferences(query, cat)
	search, searchErr := client.Search(lookupCtx, apitools.SearchOptions{
		Query: query, Limit: RemoteLookupMaxCandidates, Source: apitools.SourceAPIsGuru, CacheMode: apitools.CacheModeBypass,
	})
	references := mergeRemoteReferences(curated, search.Results)
	if len(references) == 0 {
		if err := lookupCtx.Err(); err != nil {
			if ctx.Err() != nil {
				return DiscoveryResult{}, ctx.Err()
			}
			return DiscoveryResult{Blockers: []Blocker{remoteLookupBlocker("ramen.icot.remote_lookup_timeout", "Remote source lookup exceeded its eight-second deadline.")}}, nil
		}
		message := "No curated catalog or APIs.guru source matched the active outcome."
		if searchErr != nil {
			message = "Remote source lookup failed: " + searchErr.Error()
		}
		return DiscoveryResult{Blockers: []Blocker{remoteLookupBlocker("ramen.icot.remote_lookup_empty", message)}}, nil
	}

	tempDir, err := os.MkdirTemp("", "ramen-icot-remote-*")
	if err != nil {
		return DiscoveryResult{}, err
	}
	defer os.RemoveAll(tempDir)

	var plans []SourcePlan
	var inputs []ramenauthoring.APISourceInput
	var failures []string
	seenIDs := map[string]int{}
	for _, reference := range references {
		if err := lookupCtx.Err(); err != nil {
			if ctx.Err() != nil {
				return DiscoveryResult{}, ctx.Err()
			}
			failures = append(failures, "lookup deadline reached")
			break
		}
		imported, importErr := client.Import(lookupCtx, apitools.ImportOptions{
			URL: reference.URL, Dir: tempDir, Name: firstNonEmpty(reference.ID, "remote-api"), CacheMode: apitools.CacheModeBypass,
		})
		if importErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", reference.URL, importErr))
			continue
		}
		content, readErr := os.ReadFile(imported.Path)
		if readErr != nil {
			return DiscoveryResult{}, readErr
		}
		id := slug(firstNonEmpty(reference.ID, imported.Name, imported.Title, "remote-api"))
		seenIDs[id]++
		if seenIDs[id] > 1 {
			id = fmt.Sprintf("%s-%d", id, seenIDs[id])
		}
		ext := strings.ToLower(filepath.Ext(imported.Path))
		if ext == "" {
			ext = ".json"
		}
		plans = append(plans, SourcePlan{
			ID: id, Kind: apitools.APISourceKindOpenAPI, SourceFamily: apitools.APISourceKindOpenAPI,
			Title: firstNonEmpty(imported.Title, reference.Title), OperationCount: imported.Metadata.OperationCount,
			Score: reference.Score, Path: imported.URL, SHA256: imported.SHA256, Provenance: reference.Provenance,
			TargetPath: filepath.ToSlash(filepath.Join("sources", apitools.APISourceKindOpenAPI, id+ext)), Content: content,
		})
		inputs = append(inputs, ramenauthoring.APISourceInput{Kind: apitools.APISourceKindOpenAPI, ID: id, Path: imported.Path})
	}
	if len(plans) == 0 {
		code := "ramen.icot.remote_lookup_empty"
		message := "Remote lookup returned no validated OpenAPI source."
		if lookupCtx.Err() != nil {
			code = "ramen.icot.remote_lookup_timeout"
			message = "Remote source lookup exceeded its eight-second deadline."
		} else if len(failures) > 0 {
			message += " " + strings.Join(failures, "; ")
		}
		return DiscoveryResult{Blockers: []Blocker{remoteLookupBlocker(code, message)}}, nil
	}
	promptContext, err := ramenauthoring.PromptContextFromAPISources(lookupCtx, query, inputs)
	if err != nil {
		return DiscoveryResult{Plans: plans, Blockers: []Blocker{remoteLookupBlocker("ramen.icot.remote_lookup_invalid", err.Error())}}, nil
	}
	result := DiscoveryResult{Plans: normalizeSourcePlans(plans), Context: promptcontext.Normalize(promptContext)}
	if len(failures) > 0 || searchErr != nil || lookupCtx.Err() != nil {
		message := strings.Join(failures, "; ")
		if message == "" && searchErr != nil {
			message = searchErr.Error()
		}
		if message == "" {
			message = "the remote lookup deadline was reached"
		}
		result.Blockers = []Blocker{remoteLookupBlocker("ramen.icot.remote_lookup_incomplete", "Remote lookup was incomplete: "+message)}
	}
	return result, nil
}

func curatedRemoteReferences(query string, cat catalog.Catalog) []remoteReference {
	tokens := meaningfulTokens(query)
	var refs []remoteReference
	for _, provider := range cat.ListProviders() {
		text := strings.ToLower(strings.Join(append([]string{provider.ID, provider.DisplayName, provider.Category, provider.WorkflowRelevance}, append(provider.Aliases, provider.SourceHints...)...), " "))
		score := 0
		for _, token := range tokens {
			if strings.Contains(text, token) {
				score += 10
			}
		}
		if score == 0 {
			continue
		}
		for _, ref := range provider.SpecReferencesByKind(catalog.SpecKindOpenAPI) {
			refs = append(refs, remoteReference{
				ID: provider.ID + "-" + ref.ID, Title: provider.DisplayName, URL: ref.URL,
				Provenance: "apitools-catalog:" + provider.ID + "/" + ref.ID, Score: score + 100,
			})
		}
	}
	slices.SortStableFunc(refs, func(a, b remoteReference) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(refs) > 2 {
		refs = refs[:2]
	}
	return refs
}

func mergeRemoteReferences(curated []remoteReference, guru []apitools.Result) []remoteReference {
	out := append([]remoteReference(nil), curated...)
	seen := map[string]bool{}
	for _, ref := range out {
		seen[strings.TrimSpace(ref.URL)] = true
	}
	for _, result := range guru {
		if len(out) >= RemoteLookupMaxCandidates {
			break
		}
		url := strings.TrimSpace(result.SpecURL)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, remoteReference{
			ID: result.ID, Title: result.Title, URL: url, Provenance: "apis-guru:" + result.ID, Score: result.Score,
		})
	}
	return out
}

func meaningfulTokens(value string) []string {
	ignored := map[string]bool{"create": true, "read": true, "update": true, "delete": true, "manage": true, "list": true, "show": true, "with": true, "from": true, "into": true, "that": true, "this": true, "workflow": true}
	var out []string
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r < 'a' || r > 'z' }) {
		if len(token) >= 3 && !ignored[token] && !slices.Contains(out, token) {
			out = append(out, token)
		}
	}
	return out
}

func remoteLookupBlocker(code, message string) Blocker {
	return Blocker{
		Code: code, Message: message, Deferrable: true,
		Remediation: "Provide --api-source KIND:ID=PATH or --source-root PATH, or defer source selection with an owner and unblock condition.",
	}
}
