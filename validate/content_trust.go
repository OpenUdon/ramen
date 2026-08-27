package validate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	browsertrust "github.com/OpenUdon/browsertools/contenttrust"
	browserprofile "github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/ramen/internal/browsercontract"
	"github.com/OpenUdon/ramen/project"
	uwstrust "github.com/OpenUdon/uws/contenttrust"
	"github.com/OpenUdon/uws/uws1"
)

const contentTrustAnalysisUnavailableMessage = "operation resolver failed while describing the operation"

func contentTrustDiagnostics(ctx context.Context, doc *project.Document) ([]Diagnostic, error) {
	if doc == nil || doc.UWS == nil || doc.UWS.ContentTrust == nil {
		return nil, nil
	}
	report, err := analyzeContentTrust(ctx, doc)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return []Diagnostic{{
			Code: uwstrust.CodeResolverFailure, Severity: "warning", Path: "contentTrust", Message: contentTrustAnalysisUnavailableMessage,
		}}, nil
	}
	diagnostics := make([]Diagnostic, 0, len(report.Findings))
	for _, finding := range report.Findings {
		diagnostics = append(diagnostics, Diagnostic{
			Code: finding.Code, Severity: "warning", Path: finding.Path, Message: finding.Message,
		})
	}
	return diagnostics, nil
}

func analyzeContentTrust(ctx context.Context, doc *project.Document, additional ...uwstrust.Resolver) (*uwstrust.Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if doc == nil || doc.UWS == nil {
		return nil, fmt.Errorf("content-trust analysis requires a loaded project")
	}
	resolver, hasBrowserSources := newBrowserContentTrustResolver(ctx, doc)
	resolvers := make([]uwstrust.Resolver, 0, len(additional)+1)
	if hasBrowserSources {
		resolvers = append(resolvers, resolver)
	}
	resolvers = append(resolvers, additional...)
	return uwstrust.Analyze(ctx, doc.UWS, resolvers...)
}

type browserContentTrustResolver struct {
	delegate *browsertrust.Resolver
	browser  map[string]bool
	failures map[string]bool
}

func newBrowserContentTrustResolver(ctx context.Context, doc *project.Document) (*browserContentTrustResolver, bool) {
	result := &browserContentTrustResolver{browser: map[string]bool{}, failures: map[string]bool{}}
	profiles := map[string]*browserprofile.Profile{}
	for _, source := range doc.UWS.SourceDescriptions {
		if source == nil || source.EffectiveType() != uws1.SourceDescriptionTypeBrowserProfile {
			continue
		}
		name := strings.TrimSpace(source.Name)
		result.browser[name] = true
		if err := ctx.Err(); err != nil {
			result.failures[name] = true
			continue
		}
		profile, err := browsercontract.LoadContentTrustProfile(doc.Dir, source.URL)
		if err != nil {
			result.failures[name] = true
			continue
		}
		profiles[name] = profile
	}
	if len(result.browser) == 0 {
		return result, false
	}
	delegate, err := browsertrust.NewResolver(profiles)
	if err != nil {
		for name := range result.browser {
			result.failures[name] = true
		}
		return result, true
	}
	result.delegate = delegate
	return result, true
}

func (r *browserContentTrustResolver) ResolveOperation(ctx context.Context, doc *uws1.Document, operation *uws1.Operation) (bool, uwstrust.OperationContract, error) {
	if err := ctx.Err(); err != nil {
		return false, uwstrust.OperationContract{}, err
	}
	if operation == nil || r == nil || !r.browser[strings.TrimSpace(operation.SourceDescription)] {
		return false, uwstrust.OperationContract{}, nil
	}
	if r.delegate == nil || r.failures[strings.TrimSpace(operation.SourceDescription)] {
		return true, uwstrust.OperationContract{}, fmt.Errorf("browser content-trust profile is unavailable")
	}
	return r.delegate.ResolveOperation(ctx, doc, operation)
}
