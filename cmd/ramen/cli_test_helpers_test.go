package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sharedpromptcontext "github.com/OpenUdon/authoring/promptcontext"
	"github.com/OpenUdon/ramen/project"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func helperCommand(args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestHelperProcess", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	os.Args = append([]string{"ramen"}, args[1:]...)
	main()
	os.Exit(0)
}

func mustWriteCLIFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeAuthorContextForCLITest(t *testing.T, dir string, ctx sharedpromptcontext.Context) string {
	t.Helper()
	data, err := sharedpromptcontext.CanonicalJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "context.json")
	mustWriteCLIFile(t, path, data)
	return path
}

func writeAuthorOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: Author CLI Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      responses:
        "200":
          description: ok
`))
}

func writeICOTOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: iCoT CLI Test
  version: v1
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        "200":
          description: ok
    post:
      operationId: createWidget
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
`))
}

func writeCreateOnlyICOTOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: iCoT CLI Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
`))
}

func writeLifecycleICOTOpenAPIForCLITest(t *testing.T, path string) {
	t.Helper()
	mustWriteCLIFile(t, path, []byte(`openapi: 3.0.0
info:
  title: iCoT Lifecycle CLI Test
  version: v1
paths:
  /widgets:
    post:
      operationId: createWidget
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
  /widgets/{id}:
    get:
      operationId: getWidget
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        "200":
          description: ok
    patch:
      operationId: patchWidget
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
      responses:
        "200":
          description: ok
    delete:
      operationId: deleteWidget
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        "204":
          description: deleted
`))
}

func writeNativeProjectForCLITest(t *testing.T, dir string, profile project.Profile) string {
	t.Helper()
	doc := &uws1.Document{
		UWS: "1.4.0",
		Info: &uws1.Info{
			Title:   "cli_validate_fixture",
			Version: "1.0.0",
		},
		Operations: []*uws1.Operation{{
			OperationID: "review",
			Request:     map[string]any{"x-test": true},
			Extensions:  map[string]any{uws1.ExtensionOperationProfile: "ramen-cli-test"},
		}},
		Workflows: []*uws1.Workflow{{
			WorkflowID: "main",
			Type:       uws1.WorkflowTypeSequence,
			Steps: []*uws1.Step{{
				StepID:       "review",
				OperationRef: "review",
			}},
		}},
		Extensions: map[string]any{project.ExtensionKey: profile},
	}
	data, err := uwsconvert.MarshalJSONIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, project.DefaultJSON)
	mustWriteCLIFile(t, path, data)
	return path
}
