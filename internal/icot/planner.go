package icot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	sharedicot "github.com/OpenUdon/authoring/icot"
	"github.com/OpenUdon/authoring/interview"
	"github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/authoring/readiness"
	"github.com/OpenUdon/authoring/session"
	ramenauthoring "github.com/OpenUdon/ramen/authoring"
	"github.com/OpenUdon/ramen/project"
)

const (
	nodeActiveWorkflow = "boundary.active_workflow"
	nodeOutcome        = "boundary.outcome"
	nodeActorTrigger   = "boundary.actor_trigger"
	nodeSuccess        = "boundary.success_evidence"
	nodeNonGoals       = "boundary.non_goals"
	nodeProjectName    = "project.name"
	nodeSourceInput    = "source.input"
	nodeSource         = "source.selection"
	nodeDiscovery      = "source.discovery_blocker"
	nodeOperation      = "operation.seed"
	nodeSafety         = "safety.mutation_posture"
	nodeCredentials    = "security.credentials"
	nodeRuntime        = "runtime.behavior"
	nodeFallback       = "runtime.fallback"
	nodeVerification   = "output.verification"
	nodeProposal       = "proposal.approval"
)

// SeedSession constructs a fresh v2 session from already-inspected local
// evidence. It performs no writes.
func SeedSession(goal, projectName, outDir, networkPolicy string, discoveryPlans []SourcePlan, discoveryBlockers []Blocker, ctx promptcontext.Context) Session {
	s := Session{
		Version:       SessionVersion,
		Interview:     interview.State{Version: interview.Version},
		Intent:        ActiveIntent{ProjectName: strings.TrimSpace(projectName)},
		Context:       promptcontext.Normalize(ctx),
		SourcePlans:   append([]SourcePlan(nil), discoveryPlans...),
		Blockers:      append([]Blocker(nil), discoveryBlockers...),
		NetworkPolicy: networkPolicy,
		OutDir:        outDir,
	}
	if containsInlineSecret(goal) {
		s.Blockers = append(s.Blockers, Blocker{
			Code: "ramen.icot.inline_secret", Message: "The requested outcome contains an inline secret-like value.",
			Remediation: "Remove the value and use an environment credential binding.", Deferrable: false,
		})
	}
	choices := candidateWorkflows(goal)
	if len(choices) > 1 {
		s.CandidateWorkflows = choices
	} else {
		s.Boundary.Outcome = strings.TrimSpace(goal)
		s.Boundary.ActiveTitle = humanTitle(goal)
	}
	Normalize(&s)
	return s
}

// Normalize rebuilds conditional decisions from current product state while
// retaining all durable answers and evidence.
func Normalize(s *Session) {
	if s == nil {
		return
	}
	*s = NormalizeSession(*s)
	s.nodeIndex = make(map[string]int, len(s.Interview.Nodes))
	for i := range s.Interview.Nodes {
		s.nodeIndex[s.Interview.Nodes[i].ID] = i
	}
	ensureBaseNodes(s)
	if s.Intent.SelectedOperationID != "" {
		if op, ok := selectedOperation(*s); ok {
			resource := ramenauthoring.APILifecycleResource(s.Context, op, s.Boundary.Outcome, s.Intent.ProjectName)
			applyPersistedSecuritySelections(s, &resource)
			if s.Intent.FallbackBehavior != "" {
				if resource.Metadata == nil {
					resource.Metadata = map[string]any{}
				}
				resource.Metadata["fallback_behavior"] = s.Intent.FallbackBehavior
			}
			s.Intent.Resources = []project.Resource{resource}
			ensureTechnicalNodes(s)
		}
	}
	ensureProposalNodes(s)
	s.Interview = interview.Normalize(s.Interview)
}

func ensureBaseNodes(s *Session) {
	if len(s.CandidateWorkflows) > 0 && s.Boundary.Outcome == "" {
		choices := make([]string, 0, len(s.CandidateWorkflows))
		for _, candidate := range s.CandidateWorkflows {
			choices = append(choices, candidate.Title)
		}
		ensureNode(s, interview.Node{
			ID: nodeActiveWorkflow, Title: "Active desired-state workflow",
			Prompt: "Choose the one active desired-state workflow: " + strings.Join(choices, ", "),
			Status: interview.StatusOpen, Priority: 100, Required: true,
			Rationale: "Only one coherent workflow receives source and mapping detail; the rest remain unnumbered candidates.",
		})
		setNodeForced(&s.Interview, nodeActiveWorkflow, true)
	} else {
		ensureNodeWithValue(s, interview.Node{
			ID: nodeOutcome, Title: "Desired-state outcome", Prompt: "What desired-state outcome should this project deliver?",
			Status: interview.StatusOpen, Priority: 100, Required: true,
			Rationale: "The outcome bounds source and operation selection.",
		}, s.Boundary.Outcome, "operator goal")
	}
	outcomeDependency := nodeOutcome
	if len(s.CandidateWorkflows) > 0 && s.Boundary.Outcome == "" || nodeStatus(s.Interview, nodeActiveWorkflow) == interview.StatusSettled {
		outcomeDependency = nodeActiveWorkflow
	}
	if s.Boundary.Outcome != "" && nodeStatus(s.Interview, nodeOutcome) == "" && outcomeDependency != nodeActiveWorkflow {
		outcomeDependency = nodeOutcome
	}
	ensureNodeWithValue(s, interview.Node{
		ID: nodeProjectName, Title: "Project name", Prompt: "Choose the generated Ramen project name.",
		Status: interview.StatusOpen, Dependencies: []string{outcomeDependency}, Priority: 90,
		Recommendation: slug(firstNonEmpty(s.Intent.ProjectName, s.Boundary.ActiveTitle, s.Boundary.Outcome, "ramen-project")),
		Rationale:      "The project name determines stable resource and file identifiers.",
	}, s.Intent.ProjectName, "explicit project name")
	ensureNodeWithValue(s, interview.Node{
		ID: nodeActorTrigger, Title: "Actor and trigger", Prompt: "Confirm who requests reconciliation and what triggers it.",
		Status: interview.StatusOpen, Dependencies: []string{outcomeDependency}, Priority: 92,
		Recommendation: "operator or automation requests a Ramen reconciliation",
		Rationale:      "The actor and trigger distinguish authoring intent from execution authorization.",
	}, s.Boundary.ActorTrigger, "confirmed workflow boundary")
	ensureNodeWithValue(s, interview.Node{
		ID: nodeSuccess, Title: "Success evidence", Prompt: "Confirm the evidence that proves the desired state was achieved.",
		Status: interview.StatusOpen, Dependencies: []string{outcomeDependency}, Priority: 91,
		Recommendation: "native validation passes,approved plan converges,verification read confirms desired state",
		Rationale:      "Explicit evidence makes completion reviewable without assuming a successful API request equals convergence.",
	}, strings.Join(s.Boundary.SuccessEvidence, ","), "confirmed workflow boundary")
	ensureNodeWithValue(s, interview.Node{
		ID: nodeNonGoals, Title: "Non-goals", Prompt: "Confirm what this authoring session must not do.",
		Status: interview.StatusOpen, Dependencies: []string{outcomeDependency}, Priority: 89,
		Recommendation: "execute live operations,store inline secrets,bypass mutation approval",
		Rationale:      "Non-goals keep authoring, credentials, and trusted execution boundaries explicit.",
	}, strings.Join(s.Boundary.NonGoals, ","), "confirmed workflow boundary")
	if len(s.Blockers) > 0 {
		if nodeStatus(s.Interview, nodeSourceInput) == interview.StatusOpen {
			setNodeStatus(&s.Interview, nodeSourceInput, interview.StatusInapplicable)
		}
		messages := make([]string, 0, len(s.Blockers))
		deferrable := true
		for _, blocker := range s.Blockers {
			messages = append(messages, blocker.Code+": "+blocker.Message)
			deferrable = deferrable && blocker.Deferrable
		}
		blockerPrompt := "Resolve the authoring blocker"
		if deferrable {
			blockerPrompt += " or record a structured deferral"
		}
		ensureNode(s, interview.Node{
			ID: nodeDiscovery, Title: "Authoring blocker",
			Prompt: blockerPrompt + ": " + strings.Join(messages, "; "),
			Status: interview.StatusOpen, Dependencies: []string{outcomeDependency}, Priority: 95,
			Required: true, Deferrable: deferrable,
			Rationale: "Ambiguous or truncated discovery cannot safely authorize selection from a partial result. To defer, use defer:owner|impact|unblock condition|next action.",
		})
		return
	}
	if nodeStatus(s.Interview, nodeDiscovery) == interview.StatusOpen {
		setNodeStatus(&s.Interview, nodeDiscovery, interview.StatusInapplicable)
	}
	if len(s.SourcePlans) == 0 {
		prompt := "Provide a local API source as KIND:ID=PATH."
		rationale := "Only explicit local files or directories are inspected; remote lookup is disabled."
		if s.NetworkPolicy == "ask" {
			prompt = "Provide a local API source as KIND:ID=PATH, or answer remote to approve one bounded catalog/APIs.guru lookup."
			rationale = "Remote lookup requires this explicit approval and is limited to curated apitools references plus one APIs.guru search."
		} else if s.NetworkPolicy == "allow" {
			prompt = "Provide a local API source as KIND:ID=PATH, or answer remote to use the explicitly allowed bounded lookup."
			rationale = "The operator explicitly allowed bounded remote lookup."
		}
		ensureNode(s, interview.Node{
			ID: nodeSourceInput, Title: "Local API source",
			Prompt: prompt, Status: interview.StatusOpen,
			Dependencies: []string{outcomeDependency}, Priority: 85, Required: true, Deferrable: true,
			Rationale: rationale,
		})
		setNodeForced(&s.Interview, nodeSourceInput, true)
		return
	}
	sourceIDs := make([]string, 0, len(s.SourcePlans))
	for _, source := range s.SourcePlans {
		sourceIDs = append(sourceIDs, source.ID)
	}
	recommendation := ""
	forced := len(sourceIDs) != 1
	if len(sourceIDs) == 1 {
		recommendation = sourceIDs[0]
	}
	sourceDependencies := []string{outcomeDependency}
	if nodeStatus(s.Interview, nodeSourceInput) == interview.StatusSettled {
		sourceDependencies = []string{nodeSourceInput}
	}
	ensureNodeWithValue(s, interview.Node{
		ID: nodeSource, Title: "API source", Prompt: sourcePrompt(sourceIDs), Status: interview.StatusOpen,
		Dependencies: sourceDependencies, Priority: 80, Required: true, Deferrable: true,
		Recommendation: recommendation, Rationale: "Source validation and digest evidence must precede operation selection.",
	}, strings.Join(s.Intent.SelectedSourceIDs, ","), "selected local source")
	setNodeForced(&s.Interview, nodeSource, forced)
	if len(s.Intent.SelectedSourceIDs) > 0 {
		operations := availableOperations(*s)
		ids := operationIDs(operations)
		recommended, confidence := recommendOperation(s.Boundary.Outcome, operations)
		rationale := "The selected operation anchors conservative same-source lifecycle sibling inference."
		if suggested := strings.TrimSpace(s.Metadata["llm_suggested_operation_id"]); slices.Contains(ids, suggested) {
			recommended = suggested
			rationale += " Model assistance recommended this listed operation; the local source inventory remains authoritative."
		}
		ensureNodeWithValue(s, interview.Node{
			ID: nodeOperation, Title: "Seed operation", Prompt: operationPrompt(ids), Status: interview.StatusOpen,
			Dependencies: []string{nodeSource}, Priority: 70, Required: true, Deferrable: true,
			Recommendation: recommended, Rationale: rationale,
		}, s.Intent.SelectedOperationID, "selected operation")
		setNodeForced(&s.Interview, nodeOperation, len(ids) != 1 && confidence < 2)
	}
}

func ensureTechnicalNodes(s *Session) {
	if len(s.Intent.Resources) == 0 {
		return
	}
	resource := s.Intent.Resources[0]
	mutating := resourceMutates(resource)
	if mutating {
		ensureNode(s, interview.Node{
			ID: nodeSafety, Title: "Mutation posture",
			Prompt: "Confirm whether this project may describe mutating API actions (answer approve or read-only).",
			Status: interview.StatusOpen, Dependencies: []string{nodeOperation}, Priority: 100,
			Required: true, Rationale: "Authoring does not execute the actions, but mutation intent must be explicit before a runnable project is emitted.",
		})
		if s.Boundary.MutationScope != "" && !mutationConfirmed(*s) {
			setSessionNodeStatus(s, nodeSafety, interview.StatusOpen)
		}
		setNodeForced(&s.Interview, nodeSafety, true)
	} else {
		s.Boundary.MutationScope = "read-only"
		ensureNodeWithValue(s, interview.Node{
			ID: nodeSafety, Title: "Mutation posture", Prompt: "Confirm read-only project posture.", Status: interview.StatusOpen,
			Dependencies: []string{nodeOperation}, Priority: 100, Required: true, Recommendation: "read-only",
			Rationale: "The selected lifecycle contains no mutating operation.",
		}, s.Boundary.MutationScope, "observed read-only operation set")
	}
	technicalDependency := []string{nodeOperation}
	var securityDependencies []string
	roles := make([]string, 0, len(resource.Operations))
	for role := range resource.Operations {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	for _, roleName := range roles {
		role := resource.Operations[roleName]
		if len(role.CredentialBindingAlternatives) <= 1 {
			continue
		}
		id := securityAlternativeNodeID(resource.Address, roleName)
		ensureNode(s, interview.Node{
			ID: id, Title: "Security alternative",
			Prompt: "Choose one security alternative for " + roleName + ": " + strings.Join(credentialAlternativeChoices(role.CredentialBindingAlternatives), "; "),
			Status: interview.StatusOpen, Dependencies: technicalDependency, Priority: 65, Required: true, Deferrable: true,
			Rationale: "Alternative security sets are OR choices; bindings inside one set are required together.",
		})
		setNodeForced(&s.Interview, id, true)
		securityDependencies = append(securityDependencies, id)
	}
	technicalDependency = append(technicalDependency, securityDependencies...)
	for _, path := range resource.Schema {
		kind := "desired input"
		if path.Identity {
			kind = "identity"
		} else if path.Computed || path.ReadOnly {
			kind = "computed output"
		} else if path.ReplaceOnChange || path.Immutable || path.CreateOnly {
			kind = "replacement-sensitive input"
		}
		id := technicalNodeID("schema", path.Path)
		ensureNode(s, interview.Node{
			ID: id, Title: "Schema classification", Prompt: fmt.Sprintf("Confirm schema path %q as %s.", path.Path, kind),
			Status: interview.StatusOpen, Dependencies: technicalDependency, Priority: 50, Required: true, Deferrable: true,
			Recommendation: "accept", Rationale: "Ramen diff and identity behavior depends on this classification.",
		})
	}
	for _, binding := range resource.RequestBindings {
		key := strings.Join([]string{binding.OperationRole, binding.OperationID, binding.Path, binding.RequestPath, binding.Location}, "|")
		id := technicalNodeID("request", key)
		ensureNode(s, interview.Node{
			ID: id, Title: "Request mapping",
			Prompt: fmt.Sprintf("Confirm %s request mapping %q -> %s:%q.", binding.OperationRole, binding.Path, firstNonEmpty(binding.Location, "body"), binding.RequestPath),
			Status: interview.StatusOpen, Dependencies: technicalDependency, Priority: 45, Required: true, Deferrable: true,
			Recommendation: "accept", Rationale: "Request mappings determine the exact approved API input projection.",
		})
	}
	for _, binding := range resource.ResponseBindings {
		key := strings.Join([]string{binding.OperationRole, binding.OperationID, binding.ResponsePath, binding.StatePath}, "|")
		id := technicalNodeID("response", key)
		ensureNode(s, interview.Node{
			ID: id, Title: "Response mapping",
			Prompt: fmt.Sprintf("Confirm %s response mapping %q -> %q.", binding.OperationRole, binding.ResponsePath, binding.StatePath),
			Status: interview.StatusOpen, Dependencies: technicalDependency, Priority: 40, Required: true, Deferrable: true,
			Recommendation: "accept", Rationale: "Response projection controls identity, computed state, and drift evidence.",
		})
	}
	if len(resource.CredentialBindings) > 0 {
		ensureNode(s, interview.Node{
			ID: nodeCredentials, Title: "Credential bindings",
			Prompt: "Confirm environment-resolved credential bindings: " + strings.Join(resource.CredentialBindings, ", "),
			Status: interview.StatusOpen, Dependencies: technicalDependency, Priority: 60, Required: true,
			Recommendation: "environment", Rationale: "Only credential names enter the project; secret values remain outside authoring and state.",
		})
	}
	if mutating || resource.RuntimeHints != nil {
		ensureNode(s, interview.Node{
			ID: nodeRuntime, Title: "Runtime behavior", Prompt: "Confirm the source-derived retry, waiter, and settle posture.",
			Status: interview.StatusOpen, Dependencies: technicalDependency, Priority: 35, Required: true, Deferrable: true,
			Recommendation: "accept", Rationale: "Runtime hints must be reviewed before they influence convergence or cleanup.",
		})
	}
	ensureNodeWithValue(s, interview.Node{
		ID: nodeFallback, Title: "Fallback behavior",
		Prompt: "Confirm behavior when the source operation or convergence check fails.",
		Status: interview.StatusOpen, Dependencies: technicalDependency, Priority: 30, Required: true, Deferrable: true,
		Recommendation: "fail closed and retain the last confirmed state",
		Rationale:      "Failure behavior must be explicit before runtime hints or partial responses can influence desired state.",
	}, s.Intent.FallbackBehavior, "confirmed fallback policy")
}

func ensureProposalNodes(s *Session) {
	deferred := len(s.Interview.Deferrals) > 0
	if s.Intent.SelectedOperationID == "" && !deferred {
		return
	}
	if s.Intent.SelectedOperationID == "" {
		ensureNode(s, interview.Node{
			ID: nodeSafety, Title: "Authoring side-effect posture",
			Prompt: "Confirm whether this incomplete project may ultimately describe mutations (answer approve or read-only).",
			Status: interview.StatusOpen, Dependencies: []string{nodeOutcomeDependency(*s)}, Priority: 100, Required: true,
			Rationale: "Source and operation detail may be deferred, but the authoring side-effect boundary may not be deferred.",
		})
		setNodeForced(&s.Interview, nodeSafety, true)
	}
	if nodeStatus(s.Interview, nodeSafety) != interview.StatusSettled {
		return
	}
	var technical []string
	for _, node := range s.Interview.Nodes {
		if strings.HasPrefix(node.ID, "mapping.") || node.ID == nodeCredentials || node.ID == nodeRuntime || node.ID == nodeFallback {
			technical = append(technical, node.ID)
		}
	}
	if deferred {
		if nodeStatus(s.Interview, nodeVerification) == interview.StatusOpen {
			setNodeStatus(&s.Interview, nodeVerification, interview.StatusInapplicable)
		}
	} else {
		ensureNodeWithValue(s, interview.Node{
			ID: nodeVerification, Title: "Verification", Prompt: "Confirm pre-write verification gates.",
			Status: interview.StatusOpen, Dependencies: technical, Priority: 20, Required: true, Deferrable: true,
			Recommendation: "validate,graph,plan", Rationale: "A native project should validate and produce deterministic graph/plan evidence before promotion.",
		}, strings.Join(s.Intent.Verification, ","), "selected verification gates")
	}
	dependencies := []string{nodeProjectName, nodeActorTrigger, nodeSuccess, nodeNonGoals, nodeSafety}
	if nodeStatus(s.Interview, nodeSource) == interview.StatusSettled {
		dependencies = append(dependencies, nodeSource)
	}
	if nodeStatus(s.Interview, nodeOperation) == interview.StatusSettled {
		dependencies = append(dependencies, nodeOperation)
	}
	if !deferred {
		dependencies = append(dependencies, nodeVerification)
	}
	ensureNodeWithValue(s, interview.Node{
		ID: nodeProposal, Title: "Proposal approval",
		Prompt: "Approve the complete file proposal (approve) or write only the explicitly incomplete draft and resumable state (save-draft).",
		Status: interview.StatusOpen, Dependencies: dependencies, Priority: 10, Required: true,
		Rationale: "No deliverable or copied API source is written before this explicit approval; approval is never defaulted.",
	}, s.Approval, "operator proposal approval")
	setNodeForced(&s.Interview, nodeProposal, true)
}

// PlanFrontier returns the complete dependency-ready round.
func PlanFrontier(s Session) ([]readiness.Question, error) {
	return ramenInterviewBinding().Plan(&s, nil)
}

// ApplyRound atomically applies every answer in one displayed frontier.
func ApplyRound(s *Session, answers []sharedicot.RoundAnswer) error {
	if s == nil {
		return fmt.Errorf("Ramen iCoT session is required")
	}
	return ramenInterviewBinding().Apply(s, answers, nil)
}

func ramenInterviewBinding() sharedicot.InterviewBinding[Session, promptcontext.Context] {
	return sharedicot.InterviewBinding[Session, promptcontext.Context]{
		State: func(s *Session) *interview.State { return &s.Interview },
		Clone: cloneSession,
		Prepare: func(s *Session, _ []promptcontext.Context) error {
			Normalize(s)
			return interview.Validate(s.Interview)
		},
		Question: func(s Session, _ []promptcontext.Context, node interview.Node) readiness.Question {
			return readiness.Question{
				ID: node.ID, Prompt: node.Prompt, Slots: []string{node.ID}, Required: node.Required,
				Forced: nodeRequiredForced(s.Interview, node.ID), Recommendation: node.Recommendation,
				Priority: node.Priority, Rationale: node.Rationale, EvidenceRefs: append([]string(nil), node.EvidenceRefs...),
			}
		},
		Resolve: func(s *Session, _ []promptcontext.Context, node interview.Node, answer sharedicot.RoundAnswer) (interview.Resolution, error) {
			value := strings.TrimSpace(answer.Value)
			if value == "" {
				return interview.Resolution{}, fmt.Errorf("answer for %q is required", node.ID)
			}
			if containsInlineSecret(value) {
				return interview.Resolution{}, fmt.Errorf("answer for %q contains an inline secret-like value; use an environment credential binding", node.ID)
			}
			if containsUnsafePlaceholder(value) {
				return interview.Resolution{}, fmt.Errorf("answer for %q contains an unresolved placeholder; replace it with a concrete value or a ${var.name} binding", node.ID)
			}
			if (node.ID == nodeSafety || node.ID == nodeProposal) && (strings.TrimSpace(answer.Source) == "" || answer.Source == readiness.DefaultRecommendationSource || strings.EqualFold(strings.TrimSpace(answer.Source), "default")) {
				return interview.Resolution{}, fmt.Errorf("confirmation for %q must be an explicit user decision and cannot use a default", node.ID)
			}
			evidenceID := fmt.Sprintf("evidence-%06d-%s", s.Interview.Round+1, node.ID)
			if strings.HasPrefix(strings.ToLower(value), "defer:") {
				if !node.Deferrable {
					return interview.Resolution{}, fmt.Errorf("node %q cannot be deferred", node.ID)
				}
				deferral, err := parseDeferral(node.ID, value, len(s.Interview.Deferrals)+1)
				if err != nil {
					return interview.Resolution{}, err
				}
				deferral.ID = fmt.Sprintf("deferral-%06d-%s", s.Interview.Round+1, node.ID)
				evidence := interview.Evidence{ID: evidenceID, Kind: interview.EvidenceDeferral, NodeID: node.ID, Summary: deferral.Impact, Value: deferral.Impact, Source: "operator", Attributes: map[string]string{"owner": deferral.Owner, "unblock_condition": deferral.UnblockCondition}}
				return interview.Resolution{NodeID: node.ID, Deferral: &deferral, Evidence: []interview.Evidence{evidence}}, nil
			}
			if err := applyProductAnswer(s, node.ID, value); err != nil {
				return interview.Resolution{}, err
			}
			evidenceKind := interview.EvidenceUserDecision
			if answer.Source == readiness.DefaultRecommendationSource || answer.Source == "default" {
				evidenceKind = interview.EvidenceRecommendation
			}
			attributes := map[string]string(nil)
			if node.ID == nodeSafety {
				attributes = map[string]string{"requires_confirmation": "true", "classification": "side-effect-posture", "confidence": "confirmed"}
			} else if node.ID == nodeProposal {
				attributes = map[string]string{"requires_confirmation": "true", "classification": "proposal-approval", "confidence": "confirmed"}
			}
			evidence := interview.Evidence{ID: evidenceID, Kind: evidenceKind, NodeID: node.ID, Summary: value, Value: value, Source: answer.Source, Attributes: attributes}
			resolved := interview.Answer{ID: fmt.Sprintf("answer-%06d-%s", s.Interview.Round+1, node.ID), NodeID: node.ID, Value: value, Source: answer.Source, EvidenceRefs: []string{evidenceID}}
			return interview.Resolution{NodeID: node.ID, Answer: &resolved, Evidence: []interview.Evidence{evidence}}, nil
		},
		Normalize: Normalize,
		Validate:  ValidateSession,
	}
}

func CheckReadiness(s Session) []session.ReadinessIssue {
	Normalize(&s)
	if s.Approval != "" && nodeStatus(s.Interview, nodeProposal) == interview.StatusSettled && approvalConfirmed(s) {
		return nil
	}
	frontier, err := interview.Frontier(s.Interview)
	if err != nil {
		return []session.ReadinessIssue{{Code: "ramen.icot.interview_invalid", Severity: readiness.SeverityBlocking, Slot: "interview", Message: err.Error()}}
	}
	issues := make([]session.ReadinessIssue, 0, len(frontier))
	for _, node := range frontier {
		issues = append(issues, session.ReadinessIssue{Code: "ramen.icot." + strings.ReplaceAll(node.ID, ".", "_"), Severity: readiness.SeverityBlocking, Slot: node.ID, Message: node.Prompt, SuggestedAnswer: node.Recommendation})
	}
	if len(issues) == 0 {
		issues = append(issues, session.ReadinessIssue{Code: "ramen.icot.no_frontier", Severity: readiness.SeverityBlocking, Slot: "interview", Message: "Interview has unresolved decisions but no dependency-ready frontier."})
	}
	return issues
}

func Ready(s Session, issues []session.ReadinessIssue) bool {
	return len(issues) == 0 && approvalConfirmed(s)
}

func BuildProposal(s Session) Proposal {
	Normalize(&s)
	selected := selectedSourcePlans(s)
	proposal := Proposal{
		Outcome: s.Boundary.Outcome, ActiveTitle: s.Boundary.ActiveTitle, ProjectName: s.Intent.ProjectName,
		ActorTrigger:       s.Boundary.ActorTrigger,
		SuccessEvidence:    append([]string(nil), s.Boundary.SuccessEvidence...),
		NonGoals:           append([]string(nil), s.Boundary.NonGoals...),
		CandidateWorkflows: append([]CandidateWorkflow(nil), s.CandidateWorkflows...),
		Sources:            publicSourcePlans(selected), Steps: proposalWorkflowSteps(s.Intent.Resources), Resources: append([]project.Resource(nil), s.Intent.Resources...),
		MutationScope:    s.Boundary.MutationScope,
		FallbackBehavior: s.Intent.FallbackBehavior,
		Verification:     append([]string(nil), s.Intent.Verification...),
		Deferrals:        append([]interview.Deferral(nil), s.Interview.Deferrals...),
		Complete:         proposalIsComplete(s),
	}
	if proposal.Complete {
		proposal.FileActions = append(proposal.FileActions,
			FileAction{Action: "write", Path: filepath.Join(s.OutDir, project.DefaultFile)},
			FileAction{Action: "write", Path: filepath.Join(s.OutDir, "project.uws.hcl")},
		)
	} else if len(proposal.Deferrals) > 0 {
		proposal.FileActions = append(proposal.FileActions,
			FileAction{Action: "write-draft", Path: filepath.Join(s.OutDir, project.DraftFile)},
			FileAction{Action: "write-draft", Path: filepath.Join(s.OutDir, project.DraftHCL)},
		)
	}
	if len(proposal.FileActions) > 0 {
		for _, source := range selected {
			proposal.FileActions = append(proposal.FileActions, FileAction{Action: "copy", Path: filepath.Join(s.OutDir, filepath.FromSlash(source.TargetPath)), SHA256: source.SHA256})
		}
	}
	return proposal
}

func nodeOutcomeDependency(s Session) string {
	if nodeStatus(s.Interview, nodeActiveWorkflow) == interview.StatusSettled {
		return nodeActiveWorkflow
	}
	return nodeOutcome
}

func proposalIsComplete(s Session) bool {
	if len(s.Interview.Deferrals) > 0 || s.Boundary.Outcome == "" || s.Boundary.ActorTrigger == "" || len(s.Boundary.SuccessEvidence) == 0 || len(s.Boundary.NonGoals) == 0 || s.Intent.ProjectName == "" || len(s.Intent.SelectedSourceIDs) == 0 || s.Intent.SelectedOperationID == "" || len(s.Intent.Resources) == 0 || s.Boundary.MutationScope == "" || s.Intent.FallbackBehavior == "" || len(s.Intent.Verification) == 0 {
		return false
	}
	for _, node := range s.Interview.Nodes {
		if node.ID == nodeProposal {
			continue
		}
		if node.Status == interview.StatusOpen || node.Status == interview.StatusDeferred {
			return false
		}
	}
	return true
}

func applyProductAnswer(s *Session, nodeID, value string) error {
	switch nodeID {
	case nodeActiveWorkflow:
		index := slices.IndexFunc(s.CandidateWorkflows, func(candidate CandidateWorkflow) bool {
			return strings.EqualFold(candidate.Title, value) || strings.EqualFold(candidate.Outcome, value)
		})
		if index < 0 {
			if numeric, err := strconv.Atoi(value); err == nil && numeric > 0 && numeric <= len(s.CandidateWorkflows) {
				index = numeric - 1
			}
		}
		if index < 0 {
			return fmt.Errorf("active workflow %q is not one of the listed candidates", value)
		}
		selected := s.CandidateWorkflows[index]
		s.Boundary.Outcome = selected.Outcome
		s.Boundary.ActiveTitle = selected.Title
		s.CandidateWorkflows = append(s.CandidateWorkflows[:index:index], s.CandidateWorkflows[index+1:]...)
		for i := range s.CandidateWorkflows {
			s.CandidateWorkflows[i].DeferralReason = "another workflow was explicitly selected as active"
			s.CandidateWorkflows[i].PromotionTrigger = "the active workflow is complete or priorities change"
		}
	case nodeOutcome:
		s.Boundary.Outcome = value
		s.Boundary.ActiveTitle = humanTitle(value)
	case nodeProjectName:
		s.Intent.ProjectName = slug(value)
	case nodeActorTrigger:
		s.Boundary.ActorTrigger = value
	case nodeSuccess:
		s.Boundary.SuccessEvidence = splitCSV(value)
	case nodeNonGoals:
		s.Boundary.NonGoals = splitCSV(value)
	case nodeSourceInput:
		if s.Metadata == nil {
			s.Metadata = map[string]string{}
		}
		if strings.EqualFold(value, "remote") || strings.EqualFold(value, "approve-remote") {
			if s.NetworkPolicy == "never" {
				return fmt.Errorf("remote source lookup is disabled by the network policy")
			}
			s.Metadata["pending_remote_lookup"] = "true"
		} else {
			s.Metadata["pending_source_input"] = value
		}
	case nodeSource:
		ids := splitCSV(value)
		for _, id := range ids {
			if !slices.ContainsFunc(s.SourcePlans, func(source SourcePlan) bool { return source.ID == id }) {
				return fmt.Errorf("API source %q is not a validated candidate", id)
			}
		}
		if len(ids) == 0 {
			return fmt.Errorf("at least one API source is required")
		}
		if len(ids) != 1 {
			return fmt.Errorf("choose exactly one active API source; later workflows keep their sources deferred")
		}
		s.Intent.SelectedSourceIDs = ids
	case nodeOperation:
		if !slices.ContainsFunc(availableOperations(*s), func(op promptcontext.OperationCandidate) bool { return operationID(op) == value }) {
			return fmt.Errorf("operation %q is not available from the selected source", value)
		}
		s.Intent.SelectedOperationID = value
	case nodeSafety:
		switch strings.ToLower(value) {
		case "approve", "approved", "allow", "mutating", "mutation-approved":
			s.Boundary.MutationScope = "approved-for-authoring"
		case "read-only", "readonly", "deny", "no":
			if len(s.Intent.Resources) > 0 && resourceMutates(s.Intent.Resources[0]) {
				return fmt.Errorf("selected lifecycle contains mutations; choose a read-only operation or explicitly approve mutation authoring")
			}
			s.Boundary.MutationScope = "read-only"
		default:
			return fmt.Errorf("mutation posture must be approve or read-only")
		}
	case nodeVerification:
		values := splitCSV(value)
		for _, gate := range values {
			switch gate {
			case "validate", "graph", "plan":
			default:
				return fmt.Errorf("unknown verification gate %q", gate)
			}
		}
		s.Intent.Verification = values
	case nodeFallback:
		s.Intent.FallbackBehavior = value
	case nodeProposal:
		switch strings.ToLower(value) {
		case "approve", "approved", "write":
			if len(s.Interview.Deferrals) > 0 {
				return fmt.Errorf("incomplete proposal can only be saved as a draft")
			}
			s.Approval = "approve"
		case "save-draft", "draft", "defer":
			s.Approval = "save-draft"
		default:
			return fmt.Errorf("proposal decision must be approve or save-draft")
		}
	default:
		if strings.HasPrefix(nodeID, "mapping.security.") {
			return applySecurityAlternativeAnswer(s, nodeID, value)
		}
		if strings.HasPrefix(nodeID, "mapping.") || nodeID == nodeCredentials || nodeID == nodeRuntime {
			if !strings.EqualFold(value, "accept") && !strings.EqualFold(value, "environment") {
				return fmt.Errorf("decision %q must be accepted or explicitly deferred", nodeID)
			}
		}
	}
	return nil
}

func securityAlternativeNodeID(resourceAddress, role string) string {
	return technicalNodeID("security", resourceAddress+"|"+role)
}

func securityAlternativeMetadataKey(resourceAddress, role string) string {
	return "security_alternative." + resourceAddress + "." + role
}

func credentialAlternativeChoices(alternatives [][]string) []string {
	out := make([]string, 0, len(alternatives))
	for index, alternative := range alternatives {
		label := "anonymous"
		if len(alternative) > 0 {
			label = strings.Join(alternative, " + ")
		}
		out = append(out, fmt.Sprintf("%d=%s", index+1, label))
	}
	return out
}

func applySecurityAlternativeAnswer(s *Session, nodeID, value string) error {
	if s == nil || len(s.Intent.Resources) == 0 {
		return fmt.Errorf("security alternative %q has no active resource", nodeID)
	}
	resource := &s.Intent.Resources[0]
	roles := make([]string, 0, len(resource.Operations))
	for role := range resource.Operations {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	for _, roleName := range roles {
		role := resource.Operations[roleName]
		if securityAlternativeNodeID(resource.Address, roleName) != nodeID {
			continue
		}
		index := -1
		if numeric, err := strconv.Atoi(value); err == nil && numeric > 0 && numeric <= len(role.CredentialBindingAlternatives) {
			index = numeric - 1
		} else {
			matched := -1
			for candidate, alternative := range role.CredentialBindingAlternatives {
				label := "anonymous"
				if len(alternative) > 0 {
					label = strings.Join(alternative, " + ")
				}
				if strings.EqualFold(value, label) {
					if matched >= 0 {
						return fmt.Errorf("security alternative label %q is ambiguous; choose its numbered alternative", value)
					}
					matched = candidate
				}
			}
			index = matched
		}
		if index < 0 {
			return fmt.Errorf("security alternative %q is not one of the listed choices", value)
		}
		if s.Metadata == nil {
			s.Metadata = map[string]string{}
		}
		s.Metadata[securityAlternativeMetadataKey(resource.Address, roleName)] = strconv.Itoa(index + 1)
		return nil
	}
	return fmt.Errorf("security alternative node %q does not match an active operation role", nodeID)
}

func applyPersistedSecuritySelections(s *Session, resource *project.Resource) {
	if s == nil || resource == nil {
		return
	}
	var selected []string
	for roleName, role := range resource.Operations {
		value := strings.TrimSpace(s.Metadata[securityAlternativeMetadataKey(resource.Address, roleName)])
		if value != "" && len(role.CredentialBindingAlternatives) > 1 {
			if numeric, err := strconv.Atoi(value); err == nil && numeric > 0 && numeric <= len(role.CredentialBindingAlternatives) {
				role.CredentialBindings = append([]string(nil), role.CredentialBindingAlternatives[numeric-1]...)
				role.CredentialBindingAlternatives = nil
				resource.Operations[roleName] = role
			}
		}
		selected = append(selected, role.CredentialBindings...)
	}
	resource.CredentialBindings = normalizeStrings(selected)
	resource.CredentialBindingAlternatives = nil
}

func ensureNode(s *Session, desired interview.Node) {
	if i, ok := s.nodeIndex[desired.ID]; ok {
		status := s.Interview.Nodes[i].Status
		evidence := append([]string(nil), s.Interview.Nodes[i].EvidenceRefs...)
		desired.Status = status
		desired.EvidenceRefs = evidence
		s.Interview.Nodes[i] = desired
		return
	}
	if desired.Status == "" {
		desired.Status = interview.StatusOpen
	}
	s.Interview.Nodes = append(s.Interview.Nodes, desired)
	if s.nodeIndex == nil {
		s.nodeIndex = map[string]int{}
	}
	s.nodeIndex[desired.ID] = len(s.Interview.Nodes) - 1
}

func ensureNodeWithValue(s *Session, node interview.Node, value, source string) {
	ensureNode(s, node)
	index, ok := s.nodeIndex[node.ID]
	if strings.TrimSpace(value) == "" || !ok || s.Interview.Nodes[index].Status != interview.StatusOpen {
		return
	}
	setNodeStatus(&s.Interview, node.ID, interview.StatusSettled)
	answerID := fmt.Sprintf("answer-%06d", len(s.Interview.Answers)+1)
	evidenceID := addEvidence(s, node.ID, interview.EvidenceObservedFact, value, source, nil)
	s.Interview.Answers = append(s.Interview.Answers, interview.Answer{ID: answerID, NodeID: node.ID, Value: value, Source: source, EvidenceRefs: []string{evidenceID}})
}

func addEvidence(s *Session, nodeID, kind, summary, source string, attributes map[string]string) string {
	id := fmt.Sprintf("evidence-%06d", len(s.Interview.Evidence)+1)
	s.Interview.Evidence = append(s.Interview.Evidence, interview.Evidence{ID: id, Kind: kind, NodeID: nodeID, Summary: summary, Value: summary, Source: source, Attributes: attributes})
	if i, ok := s.nodeIndex[nodeID]; ok {
		s.Interview.Nodes[i].EvidenceRefs = append(s.Interview.Nodes[i].EvidenceRefs, id)
	}
	return id
}

func nodeStatus(state interview.State, id string) string {
	for _, node := range state.Nodes {
		if node.ID == id {
			return node.Status
		}
	}
	return ""
}

func setNodeStatus(state *interview.State, id, status string) {
	for i := range state.Nodes {
		if state.Nodes[i].ID == id {
			state.Nodes[i].Status = status
			return
		}
	}
}

func setSessionNodeStatus(s *Session, id, status string) {
	if i, ok := s.nodeIndex[id]; ok {
		s.Interview.Nodes[i].Status = status
		return
	}
	setNodeStatus(&s.Interview, id, status)
}

func setNodeForced(state *interview.State, id string, forced bool) {
	if state.Metadata == nil {
		state.Metadata = map[string]string{}
	}
	key := "forced." + id
	if forced {
		state.Metadata[key] = "true"
	} else {
		delete(state.Metadata, key)
	}
}

func nodeRequiredForced(state interview.State, id string) bool {
	return state.Metadata["forced."+id] == "true"
}

func technicalNodeID(kind, value string) string {
	sum := sha256.Sum256([]byte(value))
	return "mapping." + kind + "." + hex.EncodeToString(sum[:6])
}

func parseDeferral(nodeID, value string, index int) (interview.Deferral, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(strings.TrimSpace(value[len("defer:"):]), "|")
	if len(parts) != 4 {
		return interview.Deferral{}, fmt.Errorf("deferral for %q must use defer:owner|impact|unblock condition|next action", nodeID)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return interview.Deferral{}, fmt.Errorf("deferral for %q has an empty required field", nodeID)
		}
	}
	return interview.Deferral{ID: fmt.Sprintf("deferral-%06d", index), NodeID: nodeID, Owner: parts[0], Impact: parts[1], UnblockCondition: parts[2], SuggestedNextAction: parts[3]}, nil
}

func cloneSession(s Session) (Session, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return Session{}, err
	}
	var clone Session
	if err := json.Unmarshal(data, &clone); err != nil {
		return Session{}, err
	}
	return clone, nil
}

func availableOperations(s Session) []promptcontext.OperationCandidate {
	selected := map[string]bool{}
	for _, id := range s.Intent.SelectedSourceIDs {
		selected[id] = true
	}
	var out []promptcontext.OperationCandidate
	for _, operation := range s.Context.Operations {
		if len(selected) == 0 || selected[operation.SourceID] {
			out = append(out, operation)
		}
	}
	return out
}

func selectedOperation(s Session) (promptcontext.OperationCandidate, bool) {
	for _, operation := range availableOperations(s) {
		if operationID(operation) == s.Intent.SelectedOperationID {
			return operation, true
		}
	}
	return promptcontext.OperationCandidate{}, false
}

func operationID(operation promptcontext.OperationCandidate) string {
	return firstNonEmpty(operation.OperationID, operation.ID)
}

func operationIDs(operations []promptcontext.OperationCandidate) []string {
	ids := make([]string, 0, len(operations))
	for _, operation := range operations {
		if id := operationID(operation); id != "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func recommendOperation(goal string, operations []promptcontext.OperationCandidate) (string, int) {
	type ranked struct {
		id    string
		score int
	}
	var values []ranked
	for _, operation := range operations {
		values = append(values, ranked{id: operationID(operation), score: operationScore(goal, operation)})
	}
	slices.SortStableFunc(values, func(a, b ranked) int {
		if a.score != b.score {
			return b.score - a.score
		}
		return strings.Compare(a.id, b.id)
	})
	if len(values) == 0 {
		return "", 0
	}
	confidence := 1
	if len(values) == 1 || values[0].score > 0 && (len(values) == 1 || values[0].score-values[1].score >= 10) {
		confidence = 2
	}
	return values[0].id, confidence
}

func operationScore(goal string, operation promptcontext.OperationCandidate) int {
	goal = strings.ToLower(goal)
	text := strings.ToLower(strings.Join([]string{operation.ID, operation.OperationID, operation.Name, operation.Verb, operation.Path, operation.Summary, strings.Join(operation.Tags, " ")}, " "))
	score := 0
	for _, token := range strings.FieldsFunc(goal, func(r rune) bool { return r < 'a' || r > 'z' }) {
		if len(token) > 2 && strings.Contains(text, token) {
			score += 5
		}
	}
	for _, rule := range []struct {
		words  []string
		verbs  []string
		needle string
	}{{[]string{"create", "add", "manage"}, []string{"POST", "PUT"}, "create"}, {[]string{"update", "change"}, []string{"PUT", "PATCH"}, "update"}, {[]string{"delete", "remove"}, []string{"DELETE"}, "delete"}, {[]string{"read", "get", "list", "show"}, []string{"GET", "HEAD"}, "read"}} {
		if slices.ContainsFunc(rule.words, func(word string) bool { return strings.Contains(goal, word) }) && (slices.Contains(rule.verbs, strings.ToUpper(operation.Verb)) || strings.Contains(text, rule.needle)) {
			score += 20
		}
	}
	return score
}

func resourceMutates(resource project.Resource) bool {
	for role, operation := range resource.Operations {
		method := strings.ToUpper(strings.TrimSpace(operation.Method))
		if role != "read" && method != "GET" && method != "HEAD" {
			return true
		}
	}
	return false
}

func mutationConfirmed(s Session) bool {
	if s.Boundary.MutationScope != "approved-for-authoring" {
		return false
	}
	evidenceByID := make(map[string]interview.Evidence, len(s.Interview.Evidence))
	for _, evidence := range s.Interview.Evidence {
		evidenceByID[evidence.ID] = evidence
	}
	for _, answer := range s.Interview.Answers {
		if answer.NodeID != nodeSafety || strings.TrimSpace(answer.Source) == "" || answer.Source == readiness.DefaultRecommendationSource || strings.EqualFold(answer.Source, "default") || !approvedMutationAnswer(answer.Value) {
			continue
		}
		for _, evidenceID := range answer.EvidenceRefs {
			evidence := evidenceByID[evidenceID]
			if evidence.NodeID == nodeSafety && evidence.Kind == interview.EvidenceUserDecision && evidence.Attributes["requires_confirmation"] == "true" && evidence.Attributes["classification"] == "side-effect-posture" && evidence.Attributes["confidence"] == "confirmed" {
				return true
			}
		}
	}
	return false
}

func approvalConfirmed(s Session) bool {
	if s.Approval != "approve" && s.Approval != "save-draft" {
		return false
	}
	evidenceByID := make(map[string]interview.Evidence, len(s.Interview.Evidence))
	for _, evidence := range s.Interview.Evidence {
		evidenceByID[evidence.ID] = evidence
	}
	for _, answer := range s.Interview.Answers {
		if answer.NodeID != nodeProposal || strings.TrimSpace(answer.Source) == "" || answer.Source == readiness.DefaultRecommendationSource || strings.EqualFold(strings.TrimSpace(answer.Source), "default") || !approvalAnswerMatches(s.Approval, answer.Value) {
			continue
		}
		for _, evidenceID := range answer.EvidenceRefs {
			evidence := evidenceByID[evidenceID]
			if evidence.NodeID == nodeProposal && evidence.Kind == interview.EvidenceUserDecision && evidence.Attributes["requires_confirmation"] == "true" && evidence.Attributes["classification"] == "proposal-approval" && evidence.Attributes["confidence"] == "confirmed" {
				return true
			}
		}
	}
	return false
}

func approvalAnswerMatches(approval, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if approval == "approve" {
		return value == "approve" || value == "approved" || value == "write"
	}
	return approval == "save-draft" && (value == "save-draft" || value == "draft" || value == "defer")
}

func approvedMutationAnswer(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approve", "approved", "allow", "mutating", "mutation-approved":
		return true
	default:
		return false
	}
}

func selectedSourcePlans(s Session) []SourcePlan {
	selected := map[string]bool{}
	for _, id := range s.Intent.SelectedSourceIDs {
		selected[id] = true
	}
	var out []SourcePlan
	for _, source := range s.SourcePlans {
		if selected[source.ID] {
			out = append(out, source)
		}
	}
	return out
}

func sourcePrompt(ids []string) string {
	if len(ids) == 0 {
		return "Provide or discover a validated local API source."
	}
	return "Choose the active API source: " + strings.Join(ids, ", ")
}

func operationPrompt(ids []string) string {
	if len(ids) == 0 {
		return "Choose an operation after providing a source with operations."
	}
	if len(ids) > 12 {
		ids = append(append([]string(nil), ids[:12]...), fmt.Sprintf("... (%d more)", len(ids)-12))
	}
	return "Choose the seed operation: " + strings.Join(ids, ", ")
}

func splitCSV(value string) []string {
	return normalizeStrings(strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' }))
}

func candidateWorkflows(goal string) []CandidateWorkflow {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return nil
	}
	parts := strings.FieldsFunc(goal, func(r rune) bool { return r == '\n' || r == ';' })
	if len(parts) == 1 {
		lower := strings.ToLower(goal)
		if strings.Contains(lower, " and also ") {
			parts = strings.Split(goal, " and also ")
		} else if split := splitIndependentAndWorkflow(goal); len(split) > 1 {
			parts = split
		}
	}
	if len(parts) < 2 {
		return []CandidateWorkflow{{Title: humanTitle(goal), Outcome: goal}}
	}
	var out []CandidateWorkflow
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, CandidateWorkflow{Title: humanTitle(part), Outcome: part, DeferralReason: "active workflow has not been selected", PromotionTrigger: "operator selects this workflow as active"})
	}
	return normalizeCandidates(out)
}

func splitIndependentAndWorkflow(goal string) []string {
	actions := map[string]bool{
		"audit": true, "configure": true, "create": true, "delete": true, "deploy": true,
		"export": true, "get": true, "import": true, "list": true, "manage": true, "monitor": true, "read": true,
		"notify": true, "provision": true, "rotate": true, "send": true, "sync": true, "update": true,
	}
	independentActions := map[string]bool{
		"audit": true, "configure": true, "deploy": true, "export": true, "import": true,
		"monitor": true, "notify": true, "provision": true, "rotate": true, "send": true, "sync": true,
	}
	lower := strings.ToLower(goal)
	partStart := 0
	searchStart := 0
	var parts []string
	for {
		relative := strings.Index(lower[searchStart:], " and ")
		if relative < 0 {
			break
		}
		boundary := searchStart + relative
		left := strings.TrimSpace(goal[partStart:boundary])
		rightStart := boundary + len(" and ")
		rightWords := strings.Fields(goal[rightStart:])
		leftWords := strings.Fields(left)
		if len(leftWords) >= 2 && len(rightWords) >= 2 && independentActions[workflowWord(rightWords[0])] && workflowHasSubject(leftWords, actions) {
			next := workflowObjectWord(rightWords[1:])
			if next != "" && next != "it" && next != "them" && next != "this" && next != "that" && !workflowContainsWord(leftWords, next) {
				parts = append(parts, left)
				partStart = rightStart
			}
		}
		searchStart = rightStart
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(goal[partStart:]))
	return parts
}

func workflowHasSubject(words []string, actions map[string]bool) bool {
	for _, word := range words {
		word = workflowWord(word)
		if word != "" && !actions[word] && word != "a" && word != "an" && word != "the" && word != "all" {
			return true
		}
	}
	return false
}

func workflowObjectWord(words []string) string {
	for _, word := range words {
		word = workflowWord(word)
		if word != "" && word != "a" && word != "an" && word != "the" && word != "all" {
			return word
		}
	}
	return ""
}

func workflowContainsWord(words []string, wanted string) bool {
	return slices.ContainsFunc(words, func(word string) bool { return workflowWord(word) == wanted })
}

func workflowWord(word string) string {
	return strings.ToLower(strings.Trim(word, "\"'()[]{}<>,.;:"))
}

func publicSourcePlans(values []SourcePlan) []SourcePlan {
	out := append([]SourcePlan(nil), values...)
	for i := range out {
		out[i].Content = nil
	}
	return out
}

func proposalWorkflowSteps(resources []project.Resource) []WorkflowStep {
	var out []WorkflowStep
	for _, resource := range resources {
		roles := make([]string, 0, len(resource.Operations))
		for role := range resource.Operations {
			roles = append(roles, role)
		}
		slices.Sort(roles)
		for _, role := range roles {
			operation := resource.Operations[role]
			out = append(out, WorkflowStep{
				StepID: slug(resource.Address + "-" + role), ResourceAddress: resource.Address,
				Role: role, Method: operation.Method, SourceKind: operation.SourceKind,
				SourceID: operation.SourceID, OperationID: operation.OperationID,
			})
		}
	}
	return out
}

func humanTitle(value string) string {
	words := strings.Fields(strings.TrimSpace(value))
	if len(words) > 8 {
		words = words[:8]
	}
	for i := range words {
		words[i] = strings.Trim(words[i], ".,;:")
	}
	return strings.Join(words, " ")
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	dash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
