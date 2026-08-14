// Package icot implements Ramen's dependency-aware desired-state authoring
// interview. Product-neutral graph and round mechanics stay in Authoring;
// this package owns only Ramen decisions and artifacts.
package icot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/OpenUdon/apitools"
	"github.com/OpenUdon/authoring/interview"
	"github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/ramen/project"
)

const (
	SessionVersion    = "ramen.icot-session.v2"
	AnswersVersion    = "ramen.icot-answers.v2"
	ReportVersion     = "ramen.icot-report.v2"
	TranscriptVersion = "ramen.icot-transcript.v2"
)

var (
	inlineSecretPattern      = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|authorization)\s*[:=]\s*["']?[^\s,;"']{4,}`)
	unsafePlaceholderPattern = regexp.MustCompile(`(?i)(^|[^[:alnum:]_])(todo|tbd|fixme|change[ _-]?me|replace[ _-]?me|fill[ _-]?me)([^[:alnum:]_]|$)|<[[:alnum:]_][^<>]{0,63}>`)
	sha256Pattern            = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Boundary is the confirmed desired-state authoring boundary. It is not an
// execution authorization.
type Boundary struct {
	Outcome         string   `json:"outcome,omitempty"`
	ActiveTitle     string   `json:"active_title,omitempty"`
	ActorTrigger    string   `json:"actor_trigger,omitempty"`
	SuccessEvidence []string `json:"success_evidence,omitempty"`
	NonGoals        []string `json:"non_goals,omitempty"`
	MutationScope   string   `json:"mutation_scope,omitempty"`
}

// CandidateWorkflow is an unselected future direction. Candidates never get
// source files, operations, mappings, or implementation detail.
type CandidateWorkflow struct {
	Title            string `json:"title"`
	Outcome          string `json:"outcome"`
	DeferralReason   string `json:"deferral_reason"`
	PromotionTrigger string `json:"promotion_trigger"`
}

// SourcePlan records validated source evidence and its proposed package
// target. Materialization happens only after proposal approval.
type SourcePlan struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	SourceFamily   string   `json:"source_family,omitempty"`
	Title          string   `json:"title,omitempty"`
	OperationCount int      `json:"operation_count,omitempty"`
	Score          int      `json:"score,omitempty"`
	Path           string   `json:"path"`
	SHA256         string   `json:"sha256"`
	Provenance     string   `json:"provenance,omitempty"`
	TargetPath     string   `json:"target_path"`
	DuplicatePaths []string `json:"duplicate_paths,omitempty"`
	Content        []byte   `json:"content,omitempty"`
}

// ActiveIntent is the Ramen-owned desired-state portion of the session.
type ActiveIntent struct {
	ProjectName         string             `json:"project_name,omitempty"`
	SelectedSourceIDs   []string           `json:"selected_source_ids,omitempty"`
	SelectedOperationID string             `json:"selected_operation_id,omitempty"`
	Resources           []project.Resource `json:"resources,omitempty"`
	FallbackBehavior    string             `json:"fallback_behavior,omitempty"`
	Verification        []string           `json:"verification,omitempty"`
}

// WorkflowStep is a proposal-only summary of one source-backed desired-state
// operation. Candidate workflows never receive steps.
type WorkflowStep struct {
	StepID          string `json:"step_id"`
	ResourceAddress string `json:"resource_address"`
	Role            string `json:"role"`
	Method          string `json:"method,omitempty"`
	SourceKind      string `json:"source_kind"`
	SourceID        string `json:"source_id"`
	OperationID     string `json:"operation_id"`
}

// Blocker is a visible condition that prevents safe progression.
type Blocker struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
	Deferrable  bool   `json:"deferrable,omitempty"`
}

// FileAction is one exact proposal action.
type FileAction struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

// Proposal is the complete review surface before any deliverable is written.
type Proposal struct {
	Outcome            string               `json:"outcome"`
	ActiveTitle        string               `json:"active_title,omitempty"`
	ActorTrigger       string               `json:"actor_trigger"`
	SuccessEvidence    []string             `json:"success_evidence,omitempty"`
	NonGoals           []string             `json:"non_goals,omitempty"`
	ProjectName        string               `json:"project_name"`
	CandidateWorkflows []CandidateWorkflow  `json:"candidate_workflows,omitempty"`
	Sources            []SourcePlan         `json:"sources,omitempty"`
	Steps              []WorkflowStep       `json:"steps,omitempty"`
	Resources          []project.Resource   `json:"resources,omitempty"`
	MutationScope      string               `json:"mutation_scope"`
	FallbackBehavior   string               `json:"fallback_behavior,omitempty"`
	Verification       []string             `json:"verification,omitempty"`
	Deferrals          []interview.Deferral `json:"deferrals,omitempty"`
	FileActions        []FileAction         `json:"file_actions,omitempty"`
	Complete           bool                 `json:"complete"`
}

// Session is the durable Ramen iCoT v2 contract.
type Session struct {
	Version            string                              `json:"version"`
	Interview          interview.State                     `json:"interview"`
	Boundary           Boundary                            `json:"boundary"`
	Intent             ActiveIntent                        `json:"active_intent"`
	Context            promptcontext.Context               `json:"prompt_context,omitempty"`
	Discovery          apitools.LocalSourceDiscoveryReport `json:"discovery,omitempty"`
	SourcePlans        []SourcePlan                        `json:"source_plans,omitempty"`
	CandidateWorkflows []CandidateWorkflow                 `json:"candidate_workflows,omitempty"`
	Blockers           []Blocker                           `json:"blockers,omitempty"`
	NetworkPolicy      string                              `json:"network_policy"`
	Approval           string                              `json:"approval,omitempty"`
	OutDir             string                              `json:"out_dir,omitempty"`
	Force              bool                                `json:"force,omitempty"`
	Metadata           map[string]string                   `json:"metadata,omitempty"`
	nodeIndex          map[string]int                      `json:"-"`
}

// AnswersFile is a versioned replay input. A saved v2 session may be supplied
// separately through --resume.
type AnswersFile struct {
	Version string `json:"version"`
	Input   string `json:"input,omitempty"`
}

func NormalizeSession(session Session) Session {
	session.Version = strings.TrimSpace(session.Version)
	if session.Version == "" {
		session.Version = SessionVersion
	}
	session.Interview = interview.Normalize(session.Interview)
	session.Boundary.Outcome = strings.TrimSpace(session.Boundary.Outcome)
	session.Boundary.ActiveTitle = strings.TrimSpace(session.Boundary.ActiveTitle)
	session.Boundary.ActorTrigger = strings.TrimSpace(session.Boundary.ActorTrigger)
	session.Boundary.SuccessEvidence = normalizeStrings(session.Boundary.SuccessEvidence)
	session.Boundary.NonGoals = normalizeStrings(session.Boundary.NonGoals)
	session.Boundary.MutationScope = strings.ToLower(strings.TrimSpace(session.Boundary.MutationScope))
	session.Intent.ProjectName = strings.TrimSpace(session.Intent.ProjectName)
	session.Intent.SelectedSourceIDs = normalizeStrings(session.Intent.SelectedSourceIDs)
	session.Intent.SelectedOperationID = strings.TrimSpace(session.Intent.SelectedOperationID)
	session.Intent.FallbackBehavior = strings.TrimSpace(session.Intent.FallbackBehavior)
	session.Intent.Verification = normalizeStrings(session.Intent.Verification)
	session.SourcePlans = normalizeSourcePlans(session.SourcePlans)
	session.CandidateWorkflows = normalizeCandidates(session.CandidateWorkflows)
	session.Blockers = normalizeBlockers(session.Blockers)
	session.NetworkPolicy = strings.ToLower(strings.TrimSpace(session.NetworkPolicy))
	if session.NetworkPolicy == "" {
		session.NetworkPolicy = "ask"
	}
	session.Approval = strings.ToLower(strings.TrimSpace(session.Approval))
	outDir := strings.TrimSpace(session.OutDir)
	if outDir == "" {
		session.OutDir = ""
	} else {
		session.OutDir = filepath.Clean(outDir)
	}
	if len(session.Metadata) == 0 {
		session.Metadata = nil
	}
	return session
}

func ValidateSession(session Session) error {
	session = NormalizeSession(session)
	if session.Version != SessionVersion {
		return fmt.Errorf("unsupported Ramen iCoT session version %q; want %q; v1 inputs are not compatible", session.Version, SessionVersion)
	}
	if err := interview.Validate(session.Interview); err != nil {
		return err
	}
	if containsInlineSecret(session.Boundary.Outcome) || containsInlineSecret(session.Boundary.ActorTrigger) || slices.ContainsFunc(session.Boundary.SuccessEvidence, containsInlineSecret) || slices.ContainsFunc(session.Boundary.NonGoals, containsInlineSecret) {
		return fmt.Errorf("Ramen iCoT session contains an inline secret-like value; use an environment credential binding instead")
	}
	if containsUnsafePlaceholder(session.Boundary.Outcome) || containsUnsafePlaceholder(session.Boundary.ActorTrigger) || slices.ContainsFunc(session.Boundary.SuccessEvidence, containsUnsafePlaceholder) || slices.ContainsFunc(session.Boundary.NonGoals, containsUnsafePlaceholder) || containsUnsafePlaceholder(session.Intent.ProjectName) {
		return fmt.Errorf("Ramen iCoT session contains an unresolved placeholder; replace it with a concrete value or a ${var.name} binding")
	}
	seenCandidates := map[string]bool{}
	for _, candidate := range session.CandidateWorkflows {
		if candidate.Title == "" || candidate.Outcome == "" || candidate.DeferralReason == "" || candidate.PromotionTrigger == "" {
			return fmt.Errorf("Ramen iCoT candidate workflows require title, outcome, deferral reason, and promotion trigger")
		}
		candidateKey := strings.ToLower(candidate.Title)
		if seenCandidates[candidateKey] {
			return fmt.Errorf("Ramen iCoT candidate workflow title %q is duplicated", candidate.Title)
		}
		seenCandidates[candidateKey] = true
		if containsInlineSecret(candidate.Title) || containsInlineSecret(candidate.Outcome) {
			return fmt.Errorf("Ramen iCoT candidate workflow contains an inline secret-like value; use an environment credential binding instead")
		}
		if containsUnsafePlaceholder(candidate.Title) || containsUnsafePlaceholder(candidate.Outcome) {
			return fmt.Errorf("Ramen iCoT candidate workflow contains an unresolved placeholder; replace it with a concrete value or a ${var.name} binding")
		}
	}
	for _, answer := range session.Interview.Answers {
		if containsInlineSecret(answer.Value) {
			return fmt.Errorf("Ramen iCoT answer for %q contains an inline secret-like value; use an environment credential binding instead", answer.NodeID)
		}
		if containsUnsafePlaceholder(answer.Value) {
			return fmt.Errorf("Ramen iCoT answer for %q contains an unresolved placeholder; replace it with a concrete value or a ${var.name} binding", answer.NodeID)
		}
	}
	if containsInlineSecret(session.Intent.FallbackBehavior) || containsUnsafePlaceholder(session.Intent.FallbackBehavior) {
		return fmt.Errorf("Ramen iCoT fallback behavior contains an unsafe inline value")
	}
	if session.Boundary.MutationScope == "approved-for-authoring" && !mutationConfirmed(session) {
		return fmt.Errorf("Ramen iCoT mutation posture lacks linked durable user-decision evidence")
	}
	if session.Approval != "" && !approvalConfirmed(session) {
		return fmt.Errorf("Ramen iCoT proposal approval lacks linked durable user-decision evidence")
	}
	switch session.NetworkPolicy {
	case "never", "ask", "allow":
	default:
		return fmt.Errorf("network policy must be never, ask, or allow")
	}
	seenIDs := map[string]SourcePlan{}
	seenTargets := map[string]SourcePlan{}
	for _, source := range session.SourcePlans {
		if source.ID == "" || source.Kind == "" || source.SHA256 == "" || source.TargetPath == "" || source.Path == "" && len(source.Content) == 0 {
			return fmt.Errorf("source plans require id, kind, path or embedded content, sha256, and target_path")
		}
		if !validSourceKind(source.Kind) {
			return fmt.Errorf("source %s:%s uses unsupported source kind", source.Kind, source.ID)
		}
		if !sha256Pattern.MatchString(source.SHA256) {
			return fmt.Errorf("source %s:%s sha256 must be a 64-character lowercase hexadecimal digest", source.Kind, source.ID)
		}
		if len(source.Content) > 0 {
			if int64(len(source.Content)) > apitools.DefaultMaxBytes {
				return fmt.Errorf("source %s:%s embedded content exceeds the %d-byte limit", source.Kind, source.ID, apitools.DefaultMaxBytes)
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(source.Content))
			if !strings.EqualFold(digest, source.SHA256) {
				return fmt.Errorf("source %s:%s embedded content digest does not match sha256", source.Kind, source.ID)
			}
		}
		if prior, ok := seenIDs[source.ID]; ok {
			return fmt.Errorf("duplicate source id %q for %s and %s; source ids must be unique within an active workflow", source.ID, prior.Path, source.Path)
		}
		seenIDs[source.ID] = source
		if err := validateRelativeSourceTarget(source.TargetPath); err != nil {
			return fmt.Errorf("source %s:%s: %w", source.Kind, source.ID, err)
		}
		if prior, ok := seenTargets[source.TargetPath]; ok && prior.SHA256 != source.SHA256 {
			return fmt.Errorf("source target collision at %q between %s and %s", source.TargetPath, prior.Path, source.Path)
		}
		seenTargets[source.TargetPath] = source
	}
	return nil
}

func containsInlineSecret(value string) bool {
	return inlineSecretPattern.MatchString(value)
}

func containsUnsafePlaceholder(value string) bool {
	return unsafePlaceholderPattern.MatchString(value)
}

func validateRelativeSourceTarget(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return fmt.Errorf("target path %q must stay inside the output directory", path)
	}
	if !strings.HasPrefix(filepath.ToSlash(path), "sources/") {
		return fmt.Errorf("target path %q must stay under the sources directory", path)
	}
	return nil
}

func validSourceKind(kind string) bool {
	switch kind {
	case apitools.APISourceKindOpenAPI, apitools.APISourceKindGoogleDiscovery, apitools.APISourceKindAWSSmithy,
		apitools.APISourceKindAsyncAPI, apitools.APISourceKindGraphQL, apitools.APISourceKindOpenRPC,
		apitools.APISourceKindGRPCProtobuf, apitools.APISourceKindOData:
		return true
	default:
		return false
	}
}

func SaveSession(path string, session Session) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	session = NormalizeSession(session)
	if err := ValidateSession(session); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := preparePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".icot-session-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func preparePrivateDirectory(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		path = "."
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("iCoT state directory %q is not a non-symlink directory", path)
	}
	return nil
}

func LoadSession(path string) (Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}
	var version struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return Session{}, fmt.Errorf("parse Ramen iCoT session: %w", err)
	}
	if strings.TrimSpace(version.Version) != SessionVersion {
		return Session{}, fmt.Errorf("unsupported Ramen iCoT session version %q; want %q; v1 inputs are not compatible", version.Version, SessionVersion)
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, fmt.Errorf("parse Ramen iCoT session: %w", err)
	}
	session = NormalizeSession(session)
	if err := ValidateSession(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// PrepareResume reopens previously deferred technical decisions and proposal
// approval while retaining their historical evidence. A resumed agent run can
// therefore report the exact reopened frontier without silently promoting a
// draft.
func PrepareResume(session *Session) {
	if session == nil || session.Approval != "save-draft" {
		return
	}
	for _, deferral := range session.Interview.Deferrals {
		setNodeStatus(&session.Interview, deferral.NodeID, interview.StatusOpen)
	}
	if nodeStatus(session.Interview, nodeVerification) == interview.StatusInapplicable {
		setNodeStatus(&session.Interview, nodeVerification, interview.StatusOpen)
	}
	setNodeStatus(&session.Interview, nodeProposal, interview.StatusOpen)
	session.Interview.Deferrals = nil
	session.Approval = ""
	Normalize(session)
}

func normalizeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func normalizeSourcePlans(values []SourcePlan) []SourcePlan {
	out := append([]SourcePlan(nil), values...)
	for i := range out {
		out[i].ID = strings.TrimSpace(out[i].ID)
		out[i].Kind = strings.TrimSpace(out[i].Kind)
		out[i].SourceFamily = strings.TrimSpace(out[i].SourceFamily)
		out[i].Title = strings.TrimSpace(out[i].Title)
		out[i].Path = strings.TrimSpace(out[i].Path)
		if !strings.Contains(out[i].Path, "://") {
			out[i].Path = filepath.Clean(out[i].Path)
		}
		out[i].SHA256 = strings.ToLower(strings.TrimSpace(out[i].SHA256))
		out[i].Provenance = strings.TrimSpace(out[i].Provenance)
		out[i].TargetPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(out[i].TargetPath)))
		out[i].DuplicatePaths = normalizeStrings(out[i].DuplicatePaths)
	}
	slices.SortStableFunc(out, func(a, b SourcePlan) int {
		if a.ID != b.ID {
			return strings.Compare(a.ID, b.ID)
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}

func normalizeCandidates(values []CandidateWorkflow) []CandidateWorkflow {
	out := append([]CandidateWorkflow(nil), values...)
	for i := range out {
		out[i].Title = strings.TrimSpace(out[i].Title)
		out[i].Outcome = strings.TrimSpace(out[i].Outcome)
		out[i].DeferralReason = strings.TrimSpace(out[i].DeferralReason)
		out[i].PromotionTrigger = strings.TrimSpace(out[i].PromotionTrigger)
	}
	slices.SortStableFunc(out, func(a, b CandidateWorkflow) int { return strings.Compare(a.Title, b.Title) })
	return out
}

func normalizeBlockers(values []Blocker) []Blocker {
	out := append([]Blocker(nil), values...)
	for i := range out {
		out[i].Code = strings.TrimSpace(out[i].Code)
		out[i].Message = strings.TrimSpace(out[i].Message)
		out[i].Remediation = strings.TrimSpace(out[i].Remediation)
	}
	slices.SortStableFunc(out, func(a, b Blocker) int { return strings.Compare(a.Code, b.Code) })
	return out
}
