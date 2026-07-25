package governance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	evapproval "github.com/OpenUdon/evidence/approval"
	"github.com/OpenUdon/evidence/digest"
	"gopkg.in/yaml.v3"
)

const (
	ResultVersion = "ramen.governance.v1"
	PolicyVersion = "ramen.policy.v1"
)

type Input struct {
	Action       string
	InputsDigest string
	Resources    []Resource
}

type Resource struct {
	Address  string `json:"address"`
	Type     string `json:"type,omitempty"`
	Provider string `json:"provider,omitempty"`
	Action   string `json:"action"`
}

type Policy struct {
	Version                  string   `json:"version,omitempty" yaml:"version,omitempty"`
	Name                     string   `json:"name,omitempty" yaml:"name,omitempty"`
	DenyActions              []string `json:"deny_actions,omitempty" yaml:"deny_actions,omitempty"`
	WarnActions              []string `json:"warn_actions,omitempty" yaml:"warn_actions,omitempty"`
	RequireApprovalActions   []string `json:"require_approval_actions,omitempty" yaml:"require_approval_actions,omitempty"`
	RequireApprovalAddresses []string `json:"require_approval_addresses,omitempty" yaml:"require_approval_addresses,omitempty"`
	MaxDeletes               int      `json:"max_deletes,omitempty" yaml:"max_deletes,omitempty"`
	RequiredApproverRoles    []string `json:"required_approver_roles,omitempty" yaml:"required_approver_roles,omitempty"`
}

type PolicyRef struct {
	Name   string `json:"name,omitempty"`
	Path   string `json:"path,omitempty"`
	Digest string `json:"digest,omitempty"`
}

type Decision struct {
	Policy   string `json:"policy,omitempty"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Address  string `json:"address,omitempty"`
	Action   string `json:"action,omitempty"`
}

type ApprovalRequirement struct {
	ID           string   `json:"id"`
	Policy       string   `json:"policy,omitempty"`
	Reason       string   `json:"reason"`
	Address      string   `json:"address,omitempty"`
	Action       string   `json:"action,omitempty"`
	MinApprovals int      `json:"min_approvals"`
	Roles        []string `json:"roles,omitempty"`
}

type Approver struct {
	Identity   string    `json:"identity"`
	Role       string    `json:"role,omitempty"`
	Context    string    `json:"context,omitempty"`
	ApprovedAt time.Time `json:"approved_at"`
}

type Result struct {
	Version              string                `json:"version,omitempty"`
	Policies             []PolicyRef           `json:"policies,omitempty"`
	Decisions            []Decision            `json:"decisions,omitempty"`
	ApprovalRequirements []ApprovalRequirement `json:"approval_requirements,omitempty"`
}

type Engine interface {
	Evaluate(Input) Result
}

type StaticEngine struct {
	Policies []Policy
	Refs     []PolicyRef
}

func LoadPolicyFiles(paths []string) (StaticEngine, []Decision) {
	engine := StaticEngine{}
	var decisions []Decision
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			decisions = append(decisions, Decision{Severity: "error", Code: "policy.file_load_error", Message: err.Error()})
			continue
		}
		var policy Policy
		switch strings.ToLower(filepath.Ext(path)) {
		case ".yaml", ".yml":
			err = yaml.Unmarshal(data, &policy)
		default:
			err = json.Unmarshal(data, &policy)
		}
		if err != nil {
			decisions = append(decisions, Decision{Severity: "error", Code: "policy.file_parse_error", Message: err.Error()})
			continue
		}
		if strings.TrimSpace(policy.Version) != "" && policy.Version != PolicyVersion {
			decisions = append(decisions, Decision{Severity: "error", Code: "policy.version_invalid", Message: fmt.Sprintf("policy %s has unsupported version %q", path, policy.Version)})
			continue
		}
		if strings.TrimSpace(policy.Name) == "" {
			policy.Name = filepath.Base(path)
		}
		engine.Policies = append(engine.Policies, policy)
		engine.Refs = append(engine.Refs, PolicyRef{Name: policy.Name, Path: path, Digest: digestBytes(data)})
	}
	return engine, decisions
}

func (e StaticEngine) Evaluate(input Input) Result {
	result := Result{Version: ResultVersion, Policies: slices.Clone(e.Refs)}
	for _, policy := range e.Policies {
		result.Decisions = append(result.Decisions, evaluatePolicy(policy, input)...)
		result.ApprovalRequirements = append(result.ApprovalRequirements, approvalRequirements(policy, input)...)
	}
	sortResult(&result)
	return result
}

func MergeResults(results ...Result) Result {
	merged := Result{Version: ResultVersion}
	for _, result := range results {
		merged.Policies = append(merged.Policies, result.Policies...)
		merged.Decisions = append(merged.Decisions, result.Decisions...)
		merged.ApprovalRequirements = append(merged.ApprovalRequirements, result.ApprovalRequirements...)
	}
	sortResult(&merged)
	return merged
}

// RequirementsSatisfied evaluates Ramen approval requirements using the shared
// evidence/approval primitives. Ramen requirements never expire, so evaluation
// is independent of the wall clock.
func RequirementsSatisfied(requirements []ApprovalRequirement, approvers []Approver) error {
	reqs := make([]evapproval.Requirement, 0, len(requirements))
	for _, req := range requirements {
		reqs = append(reqs, evapproval.Requirement{
			ID:           req.ID,
			Reason:       req.Reason,
			Address:      req.Address,
			Action:       req.Action,
			MinApprovals: req.MinApprovals,
			Roles:        req.Roles,
		})
	}
	evApprovers := make([]evapproval.Approver, 0, len(approvers))
	for _, approver := range approvers {
		evApprovers = append(evApprovers, evapproval.Approver(approver))
	}
	return evapproval.RequirementsSatisfied(reqs, evApprovers)
}

// NormalizeApprover trims approver identity metadata and normalizes the
// approval timestamp to UTC via the shared evidence/approval primitive.
func NormalizeApprover(approver Approver) (Approver, error) {
	normalized, err := evapproval.NormalizeApprover(evapproval.Approver(approver))
	if err != nil {
		return Approver{}, err
	}
	return Approver(normalized), nil
}

func evaluatePolicy(policy Policy, input Input) []Decision {
	var decisions []Decision
	for _, resource := range input.Resources {
		if containsTrimmed(policy.DenyActions, resource.Action) {
			decisions = append(decisions, Decision{Policy: policy.Name, Severity: "error", Code: "policy.deny", Message: fmt.Sprintf("policy %s denies %s", policy.Name, resource.Action), Address: resource.Address, Action: resource.Action})
		}
		if containsTrimmed(policy.WarnActions, resource.Action) {
			decisions = append(decisions, Decision{Policy: policy.Name, Severity: "warning", Code: "policy.warn", Message: fmt.Sprintf("policy %s warns on %s", policy.Name, resource.Action), Address: resource.Address, Action: resource.Action})
		}
	}
	if policy.MaxDeletes > 0 {
		deletes := 0
		for _, resource := range input.Resources {
			if resource.Action == "delete" || resource.Action == "replace" {
				deletes++
			}
		}
		if deletes > policy.MaxDeletes {
			decisions = append(decisions, Decision{Policy: policy.Name, Severity: "error", Code: "policy.max_deletes", Message: fmt.Sprintf("policy %s allows at most %d delete/replacement action(s)", policy.Name, policy.MaxDeletes)})
		}
	}
	return decisions
}

func approvalRequirements(policy Policy, input Input) []ApprovalRequirement {
	var requirements []ApprovalRequirement
	for _, resource := range input.Resources {
		if containsTrimmed(policy.RequireApprovalActions, resource.Action) || containsTrimmed(policy.RequireApprovalAddresses, resource.Address) {
			requirements = append(requirements, ApprovalRequirement{
				ID:           fmt.Sprintf("%s:%s:%s", firstNonEmpty(policy.Name, "policy"), resource.Action, resource.Address),
				Policy:       policy.Name,
				Reason:       fmt.Sprintf("policy %s requires approval for %s", policy.Name, resource.Address),
				Address:      resource.Address,
				Action:       resource.Action,
				MinApprovals: 1,
				Roles:        trimList(policy.RequiredApproverRoles),
			})
		}
	}
	return requirements
}

func sortResult(result *Result) {
	slices.SortFunc(result.Policies, func(a, b PolicyRef) int {
		return strings.Compare(a.Name+a.Path, b.Name+b.Path)
	})
	slices.SortFunc(result.Decisions, func(a, b Decision) int {
		return strings.Compare(a.Code+a.Address+a.Action+a.Message, b.Code+b.Address+b.Action+b.Message)
	})
	slices.SortFunc(result.ApprovalRequirements, func(a, b ApprovalRequirement) int {
		return strings.Compare(a.ID, b.ID)
	})
}

func containsTrimmed(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func trimList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	slices.Sort(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func digestBytes(data []byte) string {
	return digest.SHA256String(data)
}
