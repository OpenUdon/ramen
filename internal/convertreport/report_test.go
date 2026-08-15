package convertreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedSchemasValidateOutsideRepository(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	diagnostics := NewDiagnostics(ConverterTerraform, []Diagnostic{{Code: "terraform.example", Severity: "warning", Message: "review", StrictFailure: false}})
	diagnosticPath := filepath.Join(root, "expected", "diagnostics.json")
	if err := WriteDiagnostics(diagnosticPath, diagnostics); err != nil {
		t.Fatalf("WriteDiagnostics: %v", err)
	}
	artifact, err := FileArtifact(root, "diagnostics", diagnosticPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		Version: ManifestVersion, Converter: ConverterTerraform, Mode: ModePartial, Status: StatusPartial,
		Inputs: []Input{}, Artifacts: []Artifact{artifact}, Coverage: NormalizeCoverage([]CoverageItem{{Kind: "resource", ID: "example.test", Disposition: "symbolic"}}),
		Diagnostics: Summarize("expected/diagnostics.json", diagnostics.Diagnostics), Execution: Execution{Performed: false},
	}
	if err := WriteManifest(filepath.Join(root, "expected", "manifest.json"), manifest); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
}

func TestSchemasRejectUnknownFields(t *testing.T) {
	for name, document := range map[string]string{
		"diagnostics": `{"version":"ramen.convert.diagnostics.v1","converter":"terraform","diagnostics":[],"future":true}`,
		"manifest":    `{"version":"ramen.convert.manifest.v1","converter":"terraform","mode":"partial","status":"complete","inputs":[],"artifacts":[],"coverage":{"converted":0,"symbolic":0,"unsupported":0,"ignored":0,"items":[]},"diagnostics":{"path":"expected/diagnostics.json","total":0,"errors":0,"warnings":0,"info":0,"strict_failures":0},"execution":{"performed":false},"future":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			var err error
			if name == "manifest" {
				err = ValidateManifest([]byte(document))
			} else {
				err = ValidateDiagnostics([]byte(document))
			}
			if err == nil {
				t.Fatal("expected schema rejection")
			}
		})
	}
}

func TestWrittenDocumentsAreTypedJSON(t *testing.T) {
	payload := NewDiagnostics(ConverterAnsible, nil)
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDiagnostics(data); err != nil {
		t.Fatal(err)
	}
}
