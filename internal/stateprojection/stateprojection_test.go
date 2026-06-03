package stateprojection

import (
	"testing"

	"github.com/OpenUdon/ramen/executor"
	tfplan "github.com/OpenUdon/ramen/plan"
	"github.com/OpenUdon/ramen/project"
)

func TestProjectTraversesJSONStringResponsePaths(t *testing.T) {
	mapping := &tfplan.MappingPlan{
		ResponseBindings: []project.ResponseBinding{{
			OperationRole: "read",
			ResponsePath:  "Policy.Statement.0.Sid",
			StatePath:     "policy_statement_sid",
			Computed:      true,
		}},
	}
	result := executor.Result{
		Success: true,
		Computed: map[string]any{
			"Policy": `{"Version":"2012-10-17","Statement":[{"Sid":"AllowInvoke","Effect":"Allow"}]}`,
		},
	}

	_, computed := Project(mapping, result)
	if computed["policy_statement_sid"] != "AllowInvoke" {
		t.Fatalf("computed = %#v", computed)
	}
}
