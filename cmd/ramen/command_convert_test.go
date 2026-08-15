package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/internal/ansibleconvert"
	"github.com/OpenUdon/ramen/internal/tfconvert"
	uwsconvert "github.com/OpenUdon/uws/convert"
	"github.com/OpenUdon/uws/uws1"
)

func TestCLIConvertHelpIncludesContract(t *testing.T) {
	cmd := helperCommand("--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("top-level help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "author") {
		t.Fatalf("top-level help missing author command:\n%s", output)
	}
	if !strings.Contains(string(output), "icot") {
		t.Fatalf("top-level help missing icot command:\n%s", output)
	}
	if strings.Contains(string(output), "destroy") {
		t.Fatalf("top-level help still advertises deprecated destroy command:\n%s", output)
	}

	cmd = helperCommand("author", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("author help failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Usage: ramen author", "--context", "--goal", "--out", "--validate", "--graph", "--plan", "does not execute"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("author help missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "destroy") {
		t.Fatalf("author help still names deprecated destroy command:\n%s", text)
	}

	cmd = helperCommand("convert", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert help failed: %v\n%s", err, output)
	}
	text = string(output)
	for _, expected := range []string{"Usage: ramen convert", "--config-dir", "--api-source", "--openapi", "--provider-schema", "--action", "--target", "--mode", "--strict", "does not execute Terraform"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("convert help missing %q:\n%s", expected, text)
		}
	}
	for _, expected := range []string{"ansible", "--playbook"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("convert help missing ansible subcommand %q:\n%s", expected, text)
		}
	}

	for _, helpArg := range []string{"-h", "help"} {
		cmd = helperCommand("convert", helpArg)
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("convert %s failed: %v\n%s", helpArg, err, output)
		}
		if !strings.Contains(string(output), "Usage: ramen convert") {
			t.Fatalf("convert %s output missing usage:\n%s", helpArg, output)
		}
	}

	cmd = helperCommand("convert", "tf", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert tf --help failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Usage: ramen convert") {
		t.Fatalf("convert tf help missing usage:\n%s", output)
	}
	if !strings.Contains(string(output), "--provider-schema") || !strings.Contains(string(output), "never obtained by running a provider") {
		t.Fatalf("convert tf help missing offline provider schema boundary:\n%s", output)
	}

	cmd = helperCommand("convert", "ansible", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert ansible --help failed: %v\n%s", err, output)
	}
	for _, expected := range []string{"Usage: ramen convert ansible", "--playbook", "--argspec", "--project-dir", "--roles-path", "--collections-path", "--inventory", "--extra-var", "--target-uws", "--mode", "--ignore-unsupported", "ansible-module", "resolving play roles/import_role", "host fan-out over $inputs.hosts", "not used for static expression lowering"} {
		if !strings.Contains(string(output), expected) {
			t.Fatalf("convert ansible help missing %q:\n%s", expected, output)
		}
	}

	cmd = helperCommand("icot", "--help")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("icot help failed: %v\n%s", err, output)
	}
	text = string(output)
	for _, expected := range []string{"Usage: ramen icot", "--goal", "--api-source", "--prompt-mode", "--no-llm", "--answers", "never executes"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("icot help missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "destroy") {
		t.Fatalf("icot help still names deprecated destroy command:\n%s", text)
	}
}

func TestCLIConvertWritesDraftArtifacts(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	openAPIPath := filepath.Join(root, "openapi.yaml")
	outDir := filepath.Join(root, "out")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`
resource "aws_instance" "web" {
  name = "web"
}
`))
	mustWriteCLIFile(t, openAPIPath, []byte(`openapi: 3.0.0
info:
  title: AWS Test
  version: v1
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`))
	cmd := helperCommand("convert", "--config-dir", configDir, "--openapi", "aws="+openAPIPath, "--action", "create", "--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "ramen: convert wrote") {
		t.Fatalf("convert output missing summary:\n%s", output)
	}
	projectData, err := os.ReadFile(filepath.Join(outDir, "project.uws.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projectData), tfconvert.TerraformProvenanceVersion) || strings.Contains(string(projectData), "ramen-review-todo") {
		t.Fatalf("converted project lacks versioned Terraform metadata:\n%s", projectData)
	}
	for _, rel := range []string{"project.md", "project.uws.yaml", "workflows/workflow.uws.yaml", "expected/conversion.json", "expected/mappings.json", "expected/diagnostics.json", "expected/diagnostics.md", "expected/review.md", "expected/manifest.json"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	planPath := filepath.Join(root, "native-plan.json")
	cmd = helperCommand("plan", "--project", outDir, "--state", filepath.Join(root, "state.db"), "--out", planPath)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native project plan failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "create=1") {
		t.Fatalf("native project plan output missing summary:\n%s", output)
	}
	cmd = helperCommand("convert", "tf", "--config-dir", configDir, "--openapi", "aws="+openAPIPath, "--action", "create", "--out", filepath.Join(root, "subcommand-out"))
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert tf subcommand failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(root, "subcommand-out", "workflows", "workflow.uws.yaml")); err != nil {
		t.Fatalf("convert tf subcommand missing UWS artifact: %v", err)
	}
}

func TestCLIConvertAnsibleWritesReviewArtifacts(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "ansible")
	playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "nginx", "playbook.yml")
	argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "builtin="+argspecPath,
		"--project-dir", root,
		"--roles-path", filepath.Join(root, "roles"),
		"--collections-path", filepath.Join(root, "collections"),
		"--inventory", filepath.Join(root, "inventory.ini"),
		"--extra-var", "env=test",
		"--ignore-unsupported",
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("convert ansible failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"Converted playbook:", "UWS document:", "Diagnostics:", "Review:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("convert ansible output missing %q:\n%s", expected, output)
		}
	}
	for _, rel := range []string{"workflows/workflow.uws.yaml", "workflows/workflow.hcl", "expected/diagnostics.json", "expected/diagnostics.md", "expected/review.md", "expected/manifest.json"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	review, err := os.ReadFile(filepath.Join(outDir, "expected", "review.md"))
	if err != nil {
		t.Fatalf("read review: %v", err)
	}
	reviewText := string(review)
	for _, expected := range []string{"## Unsupported Gate", "Generated artifacts are static review scaffolding", "Project directory:", "Roles paths:", "Collections paths:", "Inventory inputs:", "Extra vars:", "env=test"} {
		if !strings.Contains(reviewText, expected) {
			t.Fatalf("review missing %q:\n%s", expected, review)
		}
	}
	if !strings.Contains(reviewText, filepath.Join(root, "roles")) {
		t.Fatalf("review missing expected sections:\n%s", review)
	}
}

func TestCLIConvertAnsibleSupportedTargetsRunInCheckMode(t *testing.T) {
	tests := []struct {
		target     string
		docVersion string
	}{
		{target: "1.5", docVersion: "1.5.0"},
		{target: "1.6", docVersion: "1.6.0"},
		{target: "1.7", docVersion: "1.7.0"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			root := t.TempDir()
			outDir := filepath.Join(root, "ansible")
			playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "nginx", "playbook.yml")
			argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
			convertCmd := helperCommand("convert", "ansible",
				"--playbook", playbookPath,
				"--argspec", "builtin="+argspecPath,
				"--target-uws", tt.target,
				"--ignore-unsupported",
				"--out", outDir)
			if output, err := convertCmd.CombinedOutput(); err != nil {
				t.Fatalf("convert ansible --target-uws %s failed: %v\n%s", tt.target, err, output)
			}
			docPath := filepath.Join(outDir, "workflows", "workflow.uws.yaml")
			data, err := os.ReadFile(docPath)
			if err != nil {
				t.Fatalf("read UWS %s output: %v", tt.target, err)
			}
			var doc uws1.Document
			if err := uwsconvert.UnmarshalYAML(data, &doc); err != nil {
				t.Fatalf("parse UWS %s output: %v", tt.target, err)
			}
			if doc.UWS != tt.docVersion || len(doc.SourceDescriptions) != 0 {
				t.Fatalf("UWS %s conversion emitted unexpected binding: version=%q sources=%#v", tt.target, doc.UWS, doc.SourceDescriptions)
			}
			op := findOperationInDoc(&doc, "install_nginx")
			if op == nil || op.SourceDescription != "" || op.SourceOperationID != "" || op.SourceOperationRef != "" || op.Extensions[uws1.ExtensionOperationProfile] != ansibleconvert.ProfileName {
				t.Fatalf("target %s install operation is not extension-owned: %#v", tt.target, op)
			}
			if len(op.Extensions) != 3 || op.Extensions[ansibleconvert.ExtensionAnsibleModule] == nil || op.Extensions[ansibleconvert.ExtensionAnsibleProvenance] == nil {
				t.Fatalf("target %s extensions = %#v, want exact Ramen selector/module/provenance shape", tt.target, op.Extensions)
			}
			if err := ansibleconvert.ValidateOperationExtensions(op.Extensions); err != nil {
				t.Fatalf("target %s operation extensions are invalid: %v", tt.target, err)
			}
			for _, retired := range []string{"uws.ansible.1.0", "uws.ansible-module-call.1.0", "x-uws-ansible-module", "x-ansible"} {
				if strings.Contains(string(data), retired) {
					t.Fatalf("target %s workflow contains retired identifier %q:\n%s", tt.target, retired, data)
				}
			}
			runCmd := helperCommand("run", docPath, "--check", "--mock", "--state", filepath.Join(root, "state.db"))
			if output, err := runCmd.CombinedOutput(); err != nil {
				t.Fatalf("run check rejected converted UWS %s document: %v\n%s", tt.target, err, output)
			}
		})
	}
}

func TestInstalledCLIConvertUsesEmbeddedSchemas(t *testing.T) {
	root := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "ramen")
	build := exec.Command("go", "build", "-o", binary, "./cmd/ramen")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build installed CLI: %v\n%s", err, output)
	}

	playbook := filepath.Join(root, "playbook.yml")
	argspec := filepath.Join(root, "argspec.json")
	mustWriteCLIFile(t, playbook, []byte(`- name: embedded schemas
  hosts: localhost
  tasks:
    - name: Install package
      ansible.builtin.apt:
        name: nginx
        state: present
`))
	mustWriteCLIFile(t, argspec, []byte(`{
  "argspec": "ramen.ansible.1.0",
  "collection": "ansible.builtin",
  "modules": {
    "ansible.builtin.apt": {
      "parameters": {
        "name": {"type": "str", "required": true},
        "state": {"type": "str", "choices": ["present", "absent"]}
      }
    }
  }
}`))

	command := exec.Command(binary,
		"convert", "ansible",
		"--playbook", "playbook.yml",
		"--argspec", "builtin=argspec.json",
		"--out", "out",
	)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installed CLI conversion outside repository: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(root, "out", "workflows", "workflow.uws.yaml"))
	if err != nil {
		t.Fatalf("read installed CLI output: %v", err)
	}
	if !strings.Contains(string(data), ansibleconvert.ProfileName) || !strings.Contains(string(data), ansibleconvert.ExtensionAnsibleModule) {
		t.Fatalf("installed CLI output lacks Ramen-owned metadata:\n%s", data)
	}

	tfDir := filepath.Join(root, "tf")
	apiPath := filepath.Join(root, "api.yaml")
	mustWriteCLIFile(t, filepath.Join(tfDir, "main.tf"), []byte(`
resource "aws_instance" "web" {
  name = "web"
}
`))
	mustWriteCLIFile(t, apiPath, []byte(`openapi: 3.0.0
info:
  title: Installed Terraform conversion
  version: v1
paths:
  /instances:
    post:
      operationId: createAwsInstance
      responses:
        "200":
          description: ok
`))
	command = exec.Command(binary,
		"convert", "tf",
		"--config-dir", "tf",
		"--openapi", "aws=api.yaml",
		"--action", "create",
		"--out", "tf-out",
	)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installed Terraform conversion outside repository: %v\n%s", err, output)
	}
	tfProjectPath := filepath.Join(root, "tf-out", "project.uws.yaml")
	tfProject, err := os.ReadFile(tfProjectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tfProject), tfconvert.TerraformProvenanceVersion) {
		t.Fatalf("installed Terraform output lacks provenance discriminator:\n%s", tfProject)
	}
	command = exec.Command(binary, "validate", "--project", "tf-out")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installed CLI rejected new Terraform metadata: %v\n%s", err, output)
	}

	legacy := strings.Replace(string(tfProject), "            version: "+tfconvert.TerraformProvenanceVersion+"\n", "", 1)
	if legacy == string(tfProject) {
		t.Fatalf("failed to construct unversioned Terraform fixture:\n%s", tfProject)
	}
	mustWriteCLIFile(t, tfProjectPath, []byte(legacy))
	command = exec.Command(binary, "validate", "--project", "tf-out")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "validate.terraform_metadata_invalid") {
		t.Fatalf("installed CLI accepted unversioned Terraform metadata: err=%v\n%s", err, output)
	}
}

func TestCLIConvertAnsibleRejectsInvalidTargetUWS(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "ansible")
	playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "nginx", "playbook.yml")
	argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "builtin="+argspecPath,
		"--target-uws", "1.4",
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("convert ansible accepted invalid target:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("convert ansible exit = %v, want code 2\n%s", err, output)
	}
	if !strings.Contains(string(output), "unsupported --target-uws") {
		t.Fatalf("invalid target output missing diagnostic:\n%s", output)
	}
}

func findOperationInDoc(doc *uws1.Document, operationID string) *uws1.Operation {
	for _, op := range doc.Operations {
		if op != nil && op.OperationID == operationID {
			return op
		}
	}
	return nil
}

func TestCLIConvertAnsibleUnsupportedExitsByDefault(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "ansible")
	playbookPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "tier3", "playbook.yml")
	argspecPath := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "argspec", "ansible-builtin.argspec.json")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "builtin="+argspecPath,
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("convert ansible unsupported unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("convert ansible exit = %v, want code 3\n%s", err, output)
	}
	text := string(output)
	for _, expected := range []string{"unsupported Ansible features found", "ansible.jinja_unsupported", "rerun with --mode partial"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("unsupported output missing %q:\n%s", expected, output)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "workflows", "workflow.uws.yaml")); !os.IsNotExist(err) {
		t.Fatalf("unsupported conversion should not write workflow, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "expected", "diagnostics.json")); err != nil {
		t.Fatalf("unsupported conversion should write diagnostics: %v", err)
	}
}

func TestCLIConvertModePolicy(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "tf")
	mustWriteCLIFile(t, filepath.Join(configDir, "main.tf"), []byte(`resource "aws_instance" "web" { name = "web" }`))
	outDir := filepath.Join(root, "strict-out")
	cmd := helperCommand("convert", "tf", "--config-dir", configDir, "--out", outDir)
	output, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("strict Terraform exit = %v, want code 3\n%s", err, output)
	}
	for _, rel := range []string{"expected/diagnostics.json", "expected/review.md", "expected/manifest.json"} {
		if _, statErr := os.Stat(filepath.Join(outDir, rel)); statErr != nil {
			t.Fatalf("strict Terraform missing %s: %v", rel, statErr)
		}
	}
	for _, rel := range []string{"project.uws.yaml", "project.uws.hcl", "workflows/workflow.uws.yaml", "workflows/workflow.hcl"} {
		if _, statErr := os.Stat(filepath.Join(outDir, rel)); !os.IsNotExist(statErr) {
			t.Fatalf("strict Terraform wrote semantic payload %s: %v", rel, statErr)
		}
	}
	partialOut := filepath.Join(root, "partial-out")
	partialAPI := filepath.Join(root, "partial-api.yaml")
	mustWriteCLIFile(t, partialAPI, []byte("openapi: 3.0.0\ninfo:\n  title: Unrelated\n  version: v1\npaths:\n  /users:\n    get:\n      operationId: getUser\n      responses:\n        '200':\n          description: ok\n"))
	cmd = helperCommand("convert", "tf", "--config-dir", configDir, "--openapi", "users="+partialAPI, "--action", "create", "--mode", "partial", "--out", partialOut)
	if output, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("explicit partial Terraform conversion failed: %v\n%s", err, output)
	}
	manifest, readErr := os.ReadFile(filepath.Join(partialOut, "expected", "manifest.json"))
	if readErr != nil || !strings.Contains(string(manifest), `"mode": "partial"`) || !strings.Contains(string(manifest), `"status": "partial"`) {
		t.Fatalf("partial manifest is missing partial mode/status: err=%v\n%s", readErr, manifest)
	}

	cmd = helperCommand("convert", "tf", "--config-dir", configDir, "--mode", "partial", "--strict", "--out", filepath.Join(root, "conflict"))
	output, err = cmd.CombinedOutput()
	exitErr, ok = err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 2 || !strings.Contains(string(output), "conflicts with --mode partial") {
		t.Fatalf("Terraform mode conflict = %v, want usage exit 2\n%s", err, output)
	}

	playbook := filepath.Join("..", "..", "internal", "ansibleconvert", "testdata", "nginx", "playbook.yml")
	cmd = helperCommand("convert", "ansible", "--playbook", playbook, "--mode", "strict", "--ignore-unsupported", "--out", filepath.Join(root, "ansible-conflict"))
	output, err = cmd.CombinedOutput()
	exitErr, ok = err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 2 || !strings.Contains(string(output), "conflicts with --mode strict") {
		t.Fatalf("Ansible mode conflict = %v, want usage exit 2\n%s", err, output)
	}

	cmd = helperCommand("convert", "tf", "--config-dir", configDir, "--mode", "unsafe", "--out", filepath.Join(root, "invalid"))
	output, err = cmd.CombinedOutput()
	exitErr, ok = err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 2 || !strings.Contains(string(output), "unsupported --mode") {
		t.Fatalf("invalid mode = %v, want usage exit 2\n%s", err, output)
	}
}

func TestCLIConvertAnsibleArgspecIngestionFailureExitsOneWithoutArtifacts(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	argspecPath := filepath.Join(root, "argspec.json")
	outDir := filepath.Join(root, "out")
	if err := os.WriteFile(playbookPath, []byte(`- name: invalid argspec
  hosts: localhost
  tasks:
    - name: Safe task
      acme.tools.file:
        path: /tmp/safe
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}
	if err := os.WriteFile(argspecPath, []byte(`{
  "argspec": "ramen.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {"path": {"type": "path"}},
      "unknown": true
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("write argspec: %v", err)
	}

	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "tools="+argspecPath,
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid argspec unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("convert ansible exit = %v, want code 1\n%s", err, output)
	}
	if !strings.Contains(string(output), "schema validation failed") {
		t.Fatalf("invalid argspec output missing schema failure:\n%s", output)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("argspec ingestion failure wrote conversion artifacts, stat err=%v", err)
	}
}

func TestCLIConvertAnsibleRejectsRetiredUWSArgspec(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	argspecPath := filepath.Join(root, "argspec.json")
	outDir := filepath.Join(root, "out")
	mustWriteCLIFile(t, playbookPath, []byte(`- name: retired argspec
  hosts: localhost
  tasks:
    - name: Safe task
      acme.tools.file:
        path: /tmp/safe
`))
	mustWriteCLIFile(t, argspecPath, []byte(`{
  "argspec": "uws.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {"path": {"type": "path"}}
    }
  }
}`))

	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "tools="+argspecPath,
		"--out", outDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("retired UWS argspec unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("convert ansible exit = %v, want code 1\n%s", err, output)
	}
	if !strings.Contains(string(output), "schema validation failed") || !strings.Contains(string(output), "ramen.ansible.1.0") {
		t.Fatalf("retired argspec output missing hard-break diagnostic:\n%s", output)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("retired argspec wrote conversion artifacts, stat err=%v", err)
	}
}

func TestCLIConvertAnsibleArgspecConflictExitsThreeAndPartialOutputOmitsTask(t *testing.T) {
	root := t.TempDir()
	playbookPath := filepath.Join(root, "playbook.yml")
	argspecPath := filepath.Join(root, "argspec.json")
	if err := os.WriteFile(playbookPath, []byte(`- name: aliases
  hosts: localhost
  tasks:
    - name: Conflicting task
      acme.tools.file:
        path: /tmp/one
        dest: /tmp/two
    - name: Safe task
      acme.tools.file:
        path: /tmp/safe
`), 0o644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}
	if err := os.WriteFile(argspecPath, []byte(`{
  "argspec": "ramen.ansible.1.0",
  "collection": "acme.tools",
  "modules": {
    "acme.tools.file": {
      "parameters": {
        "path": {"type": "path", "required": true, "aliases": ["dest"]}
      }
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("write argspec: %v", err)
	}

	strictOut := filepath.Join(root, "strict")
	cmd := helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "tools="+argspecPath,
		"--out", strictOut)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("conflicting aliases unexpectedly succeeded:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 3 {
		t.Fatalf("convert ansible exit = %v, want code 3\n%s", err, output)
	}
	if !strings.Contains(string(output), "ansible.argspec_violation") {
		t.Fatalf("strict output missing argspec diagnostic:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(strictOut, "workflows", "workflow.uws.yaml")); !os.IsNotExist(err) {
		t.Fatalf("strict conversion wrote workflow, stat err=%v", err)
	}

	partialOut := filepath.Join(root, "partial")
	cmd = helperCommand("convert", "ansible",
		"--playbook", playbookPath,
		"--argspec", "tools="+argspecPath,
		"--ignore-unsupported",
		"--out", partialOut)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("partial conversion failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(partialOut, "workflows", "workflow.uws.yaml"))
	if err != nil {
		t.Fatalf("read partial workflow: %v", err)
	}
	var doc uws1.Document
	if err := uwsconvert.UnmarshalYAML(data, &doc); err != nil {
		t.Fatalf("parse partial workflow: %v", err)
	}
	if findOperationInDoc(&doc, "conflicting_task") != nil {
		t.Fatalf("conflicting task leaked into partial workflow: %#v", doc.Operations)
	}
	if findOperationInDoc(&doc, "safe_task") == nil {
		t.Fatalf("safe task missing from partial workflow: %#v", doc.Operations)
	}
}
