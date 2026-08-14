package icot

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/authoring/interview"
	"github.com/OpenUdon/authoring/promptcontext"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
)

// DiscoveryOptions contains only explicit local roots and documents. Remote
// lookup remains a separate, approval-gated action.
type DiscoveryOptions struct {
	Goal              string
	Roots             []string
	Sources           []apitools.LocalSource
	MaxVisitedEntries int
	MaxCandidates     int
	MaxBytes          int64
}

// DiscoveryResult is the prompt-safe bridge from apitools to the Ramen
// interview.
type DiscoveryResult struct {
	Report   apitools.LocalSourceDiscoveryReport
	Plans    []SourcePlan
	Blockers []Blocker
	Context  promptcontext.Context
}

// ReplaceDiscovery refreshes all source-dependent interview state while
// preserving the confirmed boundary and project name.
func ReplaceDiscovery(session *Session, discovered DiscoveryResult) {
	if session == nil {
		return
	}
	preserveSourceDecision := nodeStatus(session.Interview, nodeSourceInput) == interview.StatusSettled &&
		(strings.TrimSpace(session.Metadata["pending_source_input"]) != "" || session.Metadata["pending_remote_lookup"] == "true")
	remove := func(id string) bool {
		return id == nodeSourceInput && !preserveSourceDecision || id == nodeSource || id == nodeDiscovery || id == nodeOperation || id == nodeSafety || id == nodeCredentials || id == nodeRuntime || id == nodeFallback || id == nodeVerification || id == nodeProposal || strings.HasPrefix(id, "mapping.")
	}
	session.Interview.Nodes = slices.DeleteFunc(session.Interview.Nodes, func(node interview.Node) bool { return remove(node.ID) })
	session.Interview.Answers = slices.DeleteFunc(session.Interview.Answers, func(answer interview.Answer) bool { return remove(answer.NodeID) })
	session.Interview.Evidence = slices.DeleteFunc(session.Interview.Evidence, func(evidence interview.Evidence) bool { return remove(evidence.NodeID) })
	session.Interview.Deferrals = slices.DeleteFunc(session.Interview.Deferrals, func(deferral interview.Deferral) bool { return remove(deferral.NodeID) })
	session.Discovery = discovered.Report
	session.SourcePlans = append([]SourcePlan(nil), discovered.Plans...)
	session.Blockers = append([]Blocker(nil), discovered.Blockers...)
	session.Context = discovered.Context
	session.Intent.SelectedSourceIDs = nil
	session.Intent.SelectedOperationID = ""
	session.Intent.Resources = nil
	session.Intent.FallbackBehavior = ""
	session.Intent.Verification = nil
	session.Boundary.MutationScope = ""
	session.Approval = ""
	Normalize(session)
}

func DiscoverLocalSources(ctx context.Context, opts DiscoveryOptions) (DiscoveryResult, error) {
	report, err := apitools.DiscoverLocalSources(ctx, apitools.LocalSourceDiscoveryOptions{
		Roots: opts.Roots, Sources: opts.Sources, Query: opts.Goal,
		MaxVisitedEntries: opts.MaxVisitedEntries, MaxCandidates: opts.MaxCandidates, MaxBytes: opts.MaxBytes,
	})
	if err != nil {
		return DiscoveryResult{Report: report}, err
	}
	plans := sourcePlans(report.Candidates)
	blockers := discoveryBlockers(report, opts.Sources)
	inputs := make([]ramenauthoring.APISourceInput, 0, len(plans))
	for _, plan := range plans {
		inputs = append(inputs, ramenauthoring.APISourceInput{Kind: plan.Kind, ID: plan.ID, Path: plan.Path})
	}
	var promptContext promptcontext.Context
	if len(inputs) > 0 {
		promptContext, err = ramenauthoring.PromptContextFromAPISources(ctx, opts.Goal, inputs)
		if err != nil {
			return DiscoveryResult{Report: report, Plans: plans, Blockers: blockers}, err
		}
	}
	return DiscoveryResult{Report: report, Plans: plans, Blockers: blockers, Context: promptContext}, nil
}

func sourcePlans(candidates []apitools.LocalSourceCandidate) []SourcePlan {
	plans := make([]SourcePlan, 0, len(candidates))
	for _, candidate := range candidates {
		id := slug(firstNonEmpty(candidate.ID, candidate.Title, strings.TrimSuffix(filepath.Base(candidate.Path), filepath.Ext(candidate.Path)), candidate.Kind))
		if id == "" {
			id = "api"
		}
		ext := strings.ToLower(filepath.Ext(candidate.Path))
		if ext == "" {
			ext = defaultSourceExtension(candidate.Kind)
		}
		plans = append(plans, SourcePlan{
			ID: id, Kind: candidate.Kind, SourceFamily: candidate.SourceFamily,
			Title: candidate.Title, OperationCount: candidate.OperationCount, Score: candidate.Score,
			Path: candidate.Path, SHA256: candidate.SHA256, Provenance: candidate.Provenance,
			TargetPath:     filepath.ToSlash(filepath.Join("sources", candidate.Kind, id+ext)),
			DuplicatePaths: append([]string(nil), candidate.DuplicatePaths...),
		})
	}
	return normalizeSourcePlans(plans)
}

func discoveryBlockers(report apitools.LocalSourceDiscoveryReport, explicit []apitools.LocalSource) []Blocker {
	var blockers []Blocker
	if report.Truncated {
		blockers = append(blockers, Blocker{
			Code: "ramen.icot.discovery_truncated", Message: "Local API source discovery reached a configured bound.",
			Remediation: "Narrow --source-root or explicitly increase the discovery bound.", Deferrable: true,
		})
	}
	for _, ambiguous := range report.Ambiguous {
		blockers = append(blockers, Blocker{
			Code: "ramen.icot.discovery_ambiguous", Message: ambiguous.Message,
			Remediation: firstNonEmpty(ambiguous.Remediation, "Declare the document with --api-source KIND:ID=PATH."), Deferrable: true,
		})
	}
	explicitPaths := map[string]bool{}
	for _, source := range explicit {
		if absolute, err := filepath.Abs(source.Path); err == nil {
			explicitPaths[filepath.Clean(absolute)] = true
		}
	}
	for _, rejected := range report.Rejected {
		if rejected.Code == "file.too_large" {
			blockers = append(blockers, Blocker{
				Code: "ramen.icot.discovery_oversized", Message: rejected.Message,
				Remediation: firstNonEmpty(rejected.Remediation, "Narrow --source-root or explicitly increase --source-max-bytes after review."), Deferrable: true,
			})
		} else if explicitPaths[filepath.Clean(rejected.Path)] {
			blockers = append(blockers, Blocker{
				Code: "ramen.icot.discovery_rejected", Message: rejected.Message,
				Remediation: rejected.Remediation, Deferrable: true,
			})
		}
	}
	if len(report.Candidates) == 0 && len(blockers) == 0 {
		blockers = append(blockers, Blocker{
			Code: "ramen.icot.discovery_empty", Message: "No validated API source candidates were found.",
			Remediation: "Provide --api-source KIND:ID=PATH or narrow --source-root to API documents.", Deferrable: true,
		})
	}
	slices.SortStableFunc(blockers, func(a, b Blocker) int {
		if a.Code != b.Code {
			return strings.Compare(a.Code, b.Code)
		}
		return strings.Compare(a.Message, b.Message)
	})
	return blockers
}

func defaultSourceExtension(kind string) string {
	switch kind {
	case apitools.APISourceKindGraphQL:
		return ".graphql"
	case apitools.APISourceKindGRPCProtobuf:
		return ".proto"
	case apitools.APISourceKindOData:
		return ".xml"
	default:
		return ".json"
	}
}
