// Command corpusgen builds a regression corpus of Terraform-to-UWS conversions.
//
// It scans static Terraform testdata in the AWS provider for the services that
// ramen can currently map (derived from tfmapping.SupportedTypes), pairs each
// config with the matching AWS Smithy model, runs ramen's conversion, and keeps
// only the conversions that map cleanly (no unsupported/fallback diagnostics).
// As mapping coverage grows, re-running corpusgen yields more entries with no
// hand-editing. Large Smithy models are referenced by path, not copied.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/OpenUdon/ramen/internal/tfconvert"
	"github.com/OpenUdon/ramen/tfmapping"
	"github.com/OpenUdon/uws/convert"
)

const (
	defaultProviderDir = "../terraform-provider-aws"
	defaultSmithyDir   = "../apitools/catalog-openapi-cache/aws-smithy"
	defaultOutDir      = "testdata/corpus"
)

var (
	resourceRe = regexp.MustCompile(`(?m)^\s*resource\s+"(aws_[A-Za-z0-9_]+)"`)
	dataRe     = regexp.MustCompile(`(?m)^\s*data\s+"(aws_[A-Za-z0-9_]+)"`)
	// fallbackUnsupported diagnostics that disqualify a conversion from the corpus.
	dropCodes = map[string]bool{
		string(tfmapping.DiagnosticCodeUnsupportedProvider): true,
		string(tfmapping.DiagnosticCodeUnsupportedType):     true,
		string(tfmapping.DiagnosticCodeUnsupportedAction):   true,
		string(tfmapping.DiagnosticCodeFallbackOnly):        true,
		string(tfmapping.DiagnosticCodeMissingIdentity):     true,
	}
	// serviceAliases maps a provider service token to its Smithy model basename
	// stem where they differ (basename = aws-<stem>-smithy-model.json).
	serviceAliases = map[string]string{
		"apigateway":             "api-gateway",
		"secretsmanager":         "secrets-manager",
		"elasticloadbalancingv2": "elbv2",
		"elasticloadbalancing":   "elb",
	}
)

type modelRef struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type entryMeta struct {
	Path          string     `json:"path"`
	Service       string     `json:"service"`
	ResourceTypes []string   `json:"resource_types"`
	DataSources   []string   `json:"data_sources,omitempty"`
	SmithyModels  []modelRef `json:"smithy_models"`
	SourceDir     string     `json:"source_dir"`
}

type manifest struct {
	Version string      `json:"version"`
	Entries []entryMeta `json:"entries"`
}

type stats struct {
	servicesScanned    int
	servicesNoModel    []string
	configsConsidered  int
	droppedNoResource  int
	droppedUnsupported int
	droppedNoModel     int
	droppedDiagnostics int
	droppedHCL         int
	droppedTemplate    int
	emitted            int
	perService         map[string]int
}

func main() {
	providerDir := flag.String("provider-dir", defaultProviderDir, "Path to the terraform-provider-aws checkout")
	smithyDir := flag.String("smithy-dir", defaultSmithyDir, "Directory of aws-<service>-smithy-model.json files")
	outDir := flag.String("out", defaultOutDir, "Corpus output directory (relative to repo root)")
	action := flag.String("action", "create", "Desired action passed to ramen convert")
	flag.Parse()

	if err := run(*providerDir, *smithyDir, *outDir, *action); err != nil {
		fmt.Fprintln(os.Stderr, "corpusgen:", err)
		os.Exit(1)
	}
}

func run(providerDir, smithyDir, outDir, action string) error {
	ctx := context.Background()
	registry := tfmapping.DefaultRegistry()

	mappedResources := map[string]bool{}
	serviceForType := map[string]string{}
	services := map[string]bool{}
	for _, t := range registry.SupportedTypes() {
		if t.Provider != "aws" || !contains(t.Kinds, "resource") {
			continue
		}
		mappedResources[t.Type] = true
		svc := serviceForResourceType(t.Type)
		serviceForType[t.Type] = svc
		services[svc] = true
	}
	if len(services) == 0 {
		return fmt.Errorf("no mapped AWS resource types found in tfmapping registry")
	}

	serviceModel := map[string]string{}
	st := &stats{perService: map[string]int{}}
	for svc := range services {
		if path, ok := findSmithyModel(smithyDir, svc); ok {
			serviceModel[svc] = path
		} else {
			st.servicesNoModel = append(st.servicesNoModel, svc)
		}
	}
	sort.Strings(st.servicesNoModel)

	var entries []entryMeta
	emittedDirs := map[string]bool{}
	for _, svc := range sortedKeys(services) {
		if _, ok := serviceModel[svc]; !ok {
			continue
		}
		st.servicesScanned++
		testdata := filepath.Join(providerDir, "internal", "service", svc, "testdata")
		inputs, err := configInputs(testdata)
		if err != nil {
			return err
		}
		for _, input := range inputs {
			meta, emitted, err := processConfigDir(ctx, processArgs{
				input:           input,
				service:         svc,
				providerDir:     providerDir,
				outDir:          outDir,
				action:          action,
				mappedResources: mappedResources,
				serviceForType:  serviceForType,
				serviceModel:    serviceModel,
			}, st)
			if err != nil {
				return err
			}
			if emitted {
				entries = append(entries, meta)
				emittedDirs[filepath.Join(outDir, filepath.FromSlash(meta.Path))] = true
				st.emitted++
				st.perService[svc]++
			}
		}
	}

	pruneStaleEntries(filepath.Join(outDir, "aws"), emittedDirs)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if err := writeManifest(outDir, entries); err != nil {
		return err
	}
	if err := writeCoverage(outDir, st, registry); err != nil {
		return err
	}
	printSummary(st)
	return nil
}

type processArgs struct {
	input           configInput
	service         string
	providerDir     string
	outDir          string
	action          string
	mappedResources map[string]bool
	serviceForType  map[string]string
	serviceModel    map[string]string
}

func processConfigDir(ctx context.Context, a processArgs, st *stats) (entryMeta, bool, error) {
	st.configsConsidered++
	rendered, resources, datas, err := prepareInput(a.input)
	if err != nil {
		if a.input.Template {
			st.droppedTemplate++
			return entryMeta{}, false, nil
		}
		return entryMeta{}, false, err
	}
	if len(resources) == 0 {
		st.droppedNoResource++
		return entryMeta{}, false, nil
	}
	for _, rt := range resources {
		if !a.mappedResources[rt] {
			st.droppedUnsupported++
			return entryMeta{}, false, nil
		}
	}
	models, ok := modelsForTypes(resources, a.serviceForType, a.serviceModel)
	if !ok {
		st.droppedNoModel++
		return entryMeta{}, false, nil
	}

	entryRel := filepath.Join("aws", a.service, a.input.EntryRel)
	entryDir := filepath.Join(a.outDir, entryRel)

	// Copy the input first and convert from the corpus input directory so the
	// config_dir recorded in the generated project matches what the regression
	// test reproduces (it converts from this same path).
	inputDir := filepath.Join(entryDir, "input")
	if err := copyInputFiles(a.input, rendered, inputDir); err != nil {
		os.RemoveAll(entryDir)
		return entryMeta{}, false, err
	}

	tmp, err := os.MkdirTemp("", "corpusgen-out-")
	if err != nil {
		return entryMeta{}, false, err
	}
	defer os.RemoveAll(tmp)

	sources := make([]tfconvert.APISourceInput, 0, len(models))
	for _, m := range models {
		sources = append(sources, tfconvert.APISourceInput{Kind: tfmapping.APISourceKindAWSSmithy, ID: m.ID, Path: m.Path})
	}
	res, err := tfconvert.Convert(ctx, tfconvert.Options{
		ConfigDir:  inputDir,
		APISources: sources,
		Action:     a.action,
		OutDir:     tmp,
	})
	if err != nil || res == nil || !cleanDiagnostics(res.Diagnostics) {
		os.RemoveAll(entryDir)
		st.droppedDiagnostics++
		return entryMeta{}, false, nil
	}

	// Only keep entries whose generated HCL round-trips back to the same
	// document as the YAML. Some configs still expose UWS HCL string-escaping
	// gaps; dropping them keeps every committed .hcl valid and the corpus green.
	if !hclRoundTrips(res.NativeProjectHCLPath, res.NativeProjectPath) {
		os.RemoveAll(entryDir)
		st.droppedHCL++
		return entryMeta{}, false, nil
	}

	if err := copyArtifacts(entryDir, res); err != nil {
		return entryMeta{}, false, err
	}
	meta := entryMeta{
		Path:          filepath.ToSlash(entryRel),
		Service:       a.service,
		ResourceTypes: resources,
		DataSources:   datas,
		SmithyModels:  models,
		SourceDir:     filepath.ToSlash(mustRel(a.providerDir, a.input.Path)),
	}
	if err := writeJSON(filepath.Join(entryDir, "meta.json"), meta); err != nil {
		return entryMeta{}, false, err
	}
	return meta, true, nil
}

func copyInputFiles(input configInput, rendered []byte, inputDir string) error {
	if err := os.RemoveAll(inputDir); err != nil {
		return err
	}
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		return err
	}
	if input.Template {
		return os.WriteFile(filepath.Join(inputDir, "main.tf"), rendered, 0o644)
	}
	tfFiles, err := filepath.Glob(filepath.Join(input.Path, "*.tf"))
	if err != nil {
		return err
	}
	for _, f := range tfFiles {
		if err := copyFile(f, filepath.Join(inputDir, filepath.Base(f))); err != nil {
			return err
		}
	}
	return nil
}

func copyArtifacts(entryDir string, res *tfconvert.Result) error {
	deterministic := map[string]string{
		res.NativeProjectPath: filepath.Join(entryDir, "project.uws.yaml"),
		res.PlanJSONPath:      filepath.Join(entryDir, "plan.json"),
		res.DiagnosticsJSON:   filepath.Join(entryDir, "diagnostics.json"),
	}
	for src, dst := range deterministic {
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	// The HCL marshaller (dethcl) emits map keys in nondeterministic order, so
	// preserve an already-committed .hcl when it is semantically equal to the
	// freshly generated one to keep regenerated corpus commits stable.
	return writeStableHCL(res.NativeProjectHCLPath, filepath.Join(entryDir, "project.uws.hcl"))
}

func writeStableHCL(srcPath, dstPath string) error {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(dstPath); err == nil && hclSemanticEqual(existing, src) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dstPath, src, 0o644)
}

func hclRoundTrips(hclPath, yamlPath string) bool {
	hclData, err := os.ReadFile(hclPath)
	if err != nil {
		return false
	}
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		return false
	}
	jh, err := convert.HCLToJSON(hclData)
	if err != nil {
		return false
	}
	jy, err := convert.YAMLToJSON(yamlData)
	if err != nil {
		return false
	}
	var dh, dy any
	if json.Unmarshal(jh, &dh) != nil || json.Unmarshal(jy, &dy) != nil {
		return false
	}
	return reflect.DeepEqual(dh, dy)
}

func hclSemanticEqual(a, b []byte) bool {
	ja, ea := convert.HCLToJSON(a)
	jb, eb := convert.HCLToJSON(b)
	if ea != nil || eb != nil {
		return false
	}
	return bytes.Equal(ja, jb)
}

// pruneStaleEntries removes entry directories (those holding meta.json) under
// root that were not emitted this run, then drops any now-empty directories.
func pruneStaleEntries(root string, keep map[string]bool) {
	var stale []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if !d.IsDir() && d.Name() == "meta.json" {
			dir := filepath.Dir(path)
			if !keep[dir] {
				stale = append(stale, dir)
			}
		}
		return nil
	})
	for _, dir := range stale {
		_ = os.RemoveAll(dir)
	}
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir) // removes only empty directories
	}
}

func cleanDiagnostics(diags []tfconvert.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == "error" || dropCodes[d.Code] {
			return false
		}
	}
	return true
}

func extractTypes(dir string) (resources, datas []string, err error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, nil, err
	}
	rset := map[string]bool{}
	dset := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		for _, m := range resourceRe.FindAllSubmatch(data, -1) {
			rset[string(m[1])] = true
		}
		for _, m := range dataRe.FindAllSubmatch(data, -1) {
			dset[string(m[1])] = true
		}
	}
	return sortedKeys(rset), sortedKeys(dset), nil
}

func extractTypesFromBytes(data []byte) (resources, datas []string) {
	rset := map[string]bool{}
	dset := map[string]bool{}
	for _, m := range resourceRe.FindAllSubmatch(data, -1) {
		rset[string(m[1])] = true
	}
	for _, m := range dataRe.FindAllSubmatch(data, -1) {
		dset[string(m[1])] = true
	}
	return sortedKeys(rset), sortedKeys(dset)
}

func modelsForTypes(resources []string, serviceForType map[string]string, serviceModel map[string]string) ([]modelRef, bool) {
	seen := map[string]bool{}
	var out []modelRef
	for _, rt := range resources {
		svc := serviceForType[rt]
		if svc == "" {
			svc = serviceForResourceType(rt)
		}
		path, ok := serviceModel[svc]
		if !ok {
			return nil, false
		}
		if seen[svc] {
			continue
		}
		seen[svc] = true
		out = append(out, modelRef{ID: svc, Path: filepath.ToSlash(path)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, true
}

type configInput struct {
	Path     string
	EntryRel string
	Template bool
}

func prepareInput(input configInput) ([]byte, []string, []string, error) {
	if !input.Template {
		resources, datas, err := extractTypes(input.Path)
		return nil, resources, datas, err
	}
	data, err := renderProviderTemplate(input.Path)
	if err != nil {
		return nil, nil, nil, err
	}
	resources, datas := extractTypesFromBytes(data)
	return data, resources, datas, nil
}

func renderProviderTemplate(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	const harnessTemplates = `{{ define "region" }}{{ end }}{{ define "tags" }}{{ end }}{{ define "acctest.ConfigAvailableAZsNoOptInExclude" }}{{ end }}`
	tmpl, err := template.New(filepath.Base(path)).Option("missingkey=error").Parse(harnessTemplates + string(data))
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, map[string]any{}); err != nil {
		return nil, err
	}
	out := strings.TrimLeft(b.String(), "\n")
	if strings.Contains(out, "var.rName") && !strings.Contains(out, `variable "rName"`) {
		out += `

variable "rName" {
  description = "Name for resource"
  type        = string
  nullable    = false
}
`
	}
	return []byte(out), nil
}

func configInputs(testdata string) ([]configInput, error) {
	static := map[string]bool{}
	templates := map[string]string{}
	err := filepath.WalkDir(testdata, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch name := d.Name(); {
		case name == "main_gen.tf" || name == "main.tf":
			static[filepath.Dir(path)] = true
		case strings.HasSuffix(name, ".gtpl"):
			rel, err := filepath.Rel(testdata, path)
			if err != nil {
				return err
			}
			entryRel := strings.TrimSuffix(filepath.ToSlash(rel), ".gtpl")
			templates[entryRel] = path
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []configInput
	for _, path := range sortedKeys(static) {
		rel, err := filepath.Rel(testdata, path)
		if err != nil {
			return nil, err
		}
		out = append(out, configInput{Path: path, EntryRel: filepath.ToSlash(rel)})
	}
	for _, entryRel := range sortedKeys(templates) {
		out = append(out, configInput{Path: templates[entryRel], EntryRel: entryRel, Template: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryRel < out[j].EntryRel })
	return out, nil
}

func serviceForResourceType(t string) string {
	stem := strings.SplitN(strings.TrimPrefix(t, "aws_"), "_", 2)[0]
	return stem
}

func findSmithyModel(dir, svc string) (string, bool) {
	candidates := []string{svc}
	if alias, ok := serviceAliases[svc]; ok {
		candidates = append([]string{alias}, candidates...)
	}
	for _, c := range candidates {
		path := filepath.Join(dir, "aws-"+c+"-smithy-model.json")
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func mustRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func writeManifest(outDir string, entries []entryMeta) error {
	if entries == nil {
		entries = []entryMeta{}
	}
	return writeJSON(filepath.Join(outDir, "manifest.json"), manifest{Version: "ramen.corpus.v1", Entries: entries})
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeCoverage(outDir string, st *stats, registry tfmapping.Registry) error {
	var b strings.Builder
	b.WriteString("# Ramen conversion corpus coverage\n\n")
	b.WriteString("Generated by `go run ./cmd/corpusgen`. Do not edit by hand.\n\n")
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "- services scanned: %d\n", st.servicesScanned)
	fmt.Fprintf(&b, "- config inputs considered: %d\n", st.configsConsidered)
	fmt.Fprintf(&b, "- clean entries emitted: %d\n", st.emitted)
	fmt.Fprintf(&b, "- dropped (no managed resource): %d\n", st.droppedNoResource)
	fmt.Fprintf(&b, "- dropped (unsupported resource type): %d\n", st.droppedUnsupported)
	fmt.Fprintf(&b, "- dropped (no Smithy model for a needed service): %d\n", st.droppedNoModel)
	fmt.Fprintf(&b, "- dropped (fallback/unsupported/error diagnostics): %d\n", st.droppedDiagnostics)
	fmt.Fprintf(&b, "- dropped (HCL round-trip not yet clean): %d\n", st.droppedHCL)
	fmt.Fprintf(&b, "- dropped (template render failed): %d\n", st.droppedTemplate)
	if len(st.servicesNoModel) > 0 {
		fmt.Fprintf(&b, "- mapped services without a local Smithy model: %s\n", strings.Join(st.servicesNoModel, ", "))
	}
	b.WriteString("\n## Emitted entries by service\n\n")
	b.WriteString("| service | entries |\n|---|---|\n")
	for _, svc := range sortedKeys(st.perService) {
		fmt.Fprintf(&b, "| %s | %d |\n", svc, st.perService[svc])
	}
	b.WriteString("\n## Mapped resource types (tfmapping)\n\n")
	b.WriteString("| provider | type | kinds |\n|---|---|---|\n")
	for _, t := range registry.SupportedTypes() {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", t.Provider, t.Type, strings.Join(t.Kinds, ", "))
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "COVERAGE.md"), []byte(b.String()), 0o644)
}

func printSummary(st *stats) {
	fmt.Printf("corpusgen: emitted=%d considered=%d services=%d dropped(unsupported=%d no-resource=%d no-model=%d diagnostics=%d hcl=%d template=%d)\n",
		st.emitted, st.configsConsidered, st.servicesScanned,
		st.droppedUnsupported, st.droppedNoResource, st.droppedNoModel, st.droppedDiagnostics, st.droppedHCL, st.droppedTemplate)
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
