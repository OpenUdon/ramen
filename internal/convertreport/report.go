// Package convertreport owns the deterministic reporting contracts shared by
// Ramen's static conversion adapters. It does not own converter semantics.
package convertreport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	ManifestVersion    = "ramen.convert.manifest.v1"
	DiagnosticsVersion = "ramen.convert.diagnostics.v1"

	ConverterTerraform = "terraform"
	ConverterAnsible   = "ansible"

	ModeStrict  = "strict"
	ModePartial = "partial"

	StatusComplete = "complete"
	StatusPartial  = "partial"
	StatusFailed   = "failed"
)

type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Byte   int `json:"byte,omitempty"`
}

type SourceRange struct {
	SourceID string   `json:"source_id,omitempty"`
	Path     string   `json:"path,omitempty"`
	Start    Position `json:"start"`
	End      Position `json:"end"`
}

type Subject struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Module string `json:"module,omitempty"`
}

type Diagnostic struct {
	Code          string       `json:"code"`
	Severity      string       `json:"severity"`
	Message       string       `json:"message"`
	StrictFailure bool         `json:"strict_failure"`
	Subject       *Subject     `json:"subject,omitempty"`
	SourceRange   *SourceRange `json:"source_range,omitempty"`
	TodoID        string       `json:"todo_id,omitempty"`
}

type Diagnostics struct {
	Version     string       `json:"version"`
	Converter   string       `json:"converter"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Input struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Path   string `json:"path,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type Artifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type CoverageItem struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Disposition    string `json:"disposition"`
	Reason         string `json:"reason,omitempty"`
	DiagnosticCode string `json:"diagnostic_code,omitempty"`
}

type Coverage struct {
	Converted   int            `json:"converted"`
	Symbolic    int            `json:"symbolic"`
	Unsupported int            `json:"unsupported"`
	Ignored     int            `json:"ignored"`
	Items       []CoverageItem `json:"items"`
}

type DiagnosticSummary struct {
	Path           string `json:"path"`
	Total          int    `json:"total"`
	Errors         int    `json:"errors"`
	Warnings       int    `json:"warnings"`
	Info           int    `json:"info"`
	StrictFailures int    `json:"strict_failures"`
}

type Execution struct {
	Performed bool `json:"performed"`
}

type Manifest struct {
	Version     string            `json:"version"`
	Converter   string            `json:"converter"`
	Mode        string            `json:"mode"`
	Status      string            `json:"status"`
	Inputs      []Input           `json:"inputs"`
	Artifacts   []Artifact        `json:"artifacts"`
	Coverage    Coverage          `json:"coverage"`
	Diagnostics DiagnosticSummary `json:"diagnostics"`
	Execution   Execution         `json:"execution"`
}

func NewDiagnostics(converter string, diagnostics []Diagnostic) Diagnostics {
	if diagnostics == nil {
		diagnostics = []Diagnostic{}
	}
	return Diagnostics{Version: DiagnosticsVersion, Converter: converter, Diagnostics: diagnostics}
}

func Summarize(path string, diagnostics []Diagnostic) DiagnosticSummary {
	summary := DiagnosticSummary{Path: filepath.ToSlash(path), Total: len(diagnostics)}
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case "error":
			summary.Errors++
		case "warning":
			summary.Warnings++
		case "info":
			summary.Info++
		}
		if diagnostic.StrictFailure {
			summary.StrictFailures++
		}
	}
	return summary
}

func NormalizeCoverage(items []CoverageItem) Coverage {
	items = slices.Clone(items)
	slices.SortFunc(items, func(a, b CoverageItem) int {
		return strings.Compare(strings.Join([]string{a.Kind, a.ID, a.Disposition, a.Reason, a.DiagnosticCode}, "\x00"), strings.Join([]string{b.Kind, b.ID, b.Disposition, b.Reason, b.DiagnosticCode}, "\x00"))
	})
	coverage := Coverage{Items: items}
	for _, item := range items {
		switch item.Disposition {
		case "converted":
			coverage.Converted++
		case "symbolic":
			coverage.Symbolic++
		case "unsupported":
			coverage.Unsupported++
		case "ignored":
			coverage.Ignored++
		}
	}
	return coverage
}

// FileInput records a file without leaking an absolute host path.
func FileInput(kind, id, path string) (Input, error) {
	digest, err := fileDigest(path)
	if err != nil {
		return Input{}, err
	}
	return Input{Kind: kind, ID: id, Path: filepath.Base(filepath.Clean(path)), SHA256: digest}, nil
}

// FileArtifact records an artifact path relative to the output root.
func FileArtifact(root, kind, path string) (Artifact, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return Artifact{}, fmt.Errorf("artifact %q is outside output root %q", path, root)
	}
	digest, err := fileDigest(path)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Kind: kind, Path: filepath.ToSlash(rel), SHA256: digest}, nil
}

func fileDigest(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func WriteDiagnostics(path string, payload Diagnostics) error {
	return writeValidatedJSON(path, payload, ValidateDiagnostics)
}

func WriteManifest(path string, manifest Manifest) error {
	return writeValidatedJSON(path, manifest, ValidateManifest)
}

func writeValidatedJSON(path string, value any, validate func([]byte) error) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := validate(data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
