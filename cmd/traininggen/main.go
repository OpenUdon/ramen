// Command traininggen builds the T01 NL-to-Ramen training manifest.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/OpenUdon/ramen/internal/trainingdata"
	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/ramen/validate"
)

const (
	defaultCorpusRoot = "testdata/corpus"
	defaultParityDoc  = "docs/parity_nl.md"
	defaultOutDir     = "testdata/training"
)

var markdownLinkRe = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

type corpusManifest struct {
	Version string        `json:"version"`
	Entries []corpusEntry `json:"entries"`
}

type corpusEntry struct {
	Path          string                   `json:"path"`
	Provider      string                   `json:"provider,omitempty"`
	Service       string                   `json:"service"`
	ResourceTypes []string                 `json:"resource_types"`
	DataSources   []string                 `json:"data_sources,omitempty"`
	APISources    []trainingdata.APISource `json:"api_sources,omitempty"`
	SmithyModels  []modelRef               `json:"smithy_models,omitempty"`
	SourceDir     string                   `json:"source_dir"`
	SourceRepo    string                   `json:"source_repo,omitempty"`
}

type modelRef struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type parityRow struct {
	Entry         string
	HCLPath       string
	WorkflowPaths []string
	Goal          string
	APISources    []trainingdata.APISource
	ProjectName   string
}

func main() {
	corpusRoot := flag.String("corpus", defaultCorpusRoot, "Path to testdata/corpus")
	parityDoc := flag.String("parity-doc", defaultParityDoc, "Path to docs/parity_nl.md")
	outDir := flag.String("out", defaultOutDir, "Training output directory")
	check := flag.Bool("check", false, "Regenerate into a temporary directory and fail if committed output is stale")
	flag.Parse()

	if err := run(context.Background(), runOptions{
		CorpusRoot: *corpusRoot,
		ParityDoc:  *parityDoc,
		OutDir:     *outDir,
		Check:      *check,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "traininggen:", err)
		os.Exit(1)
	}
}

type runOptions struct {
	CorpusRoot string
	ParityDoc  string
	OutDir     string
	Check      bool
}

func run(ctx context.Context, opts runOptions) error {
	if opts.Check {
		return checkTraining(ctx, opts)
	}
	rows, err := parseParityRows(opts.ParityDoc)
	if err != nil {
		return err
	}
	corpus, err := loadCorpusManifest(opts.CorpusRoot)
	if err != nil {
		return err
	}
	entries := make([]trainingdata.Entry, 0, len(rows)+len(corpus.Entries))
	for _, row := range rows {
		entry, err := goldEntry(ctx, opts.ParityDoc, row)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
	}
	for _, corpusEntry := range corpus.Entries {
		entry, err := silverEntry(ctx, opts.CorpusRoot, corpusEntry)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	manifest := trainingdata.Manifest{Version: trainingdata.Version, Entries: entries}
	if err := writeJSON(filepath.Join(opts.OutDir, "manifest.json"), manifest); err != nil {
		return err
	}
	if err := writeCoverage(filepath.Join(opts.OutDir, "COVERAGE.md"), entries); err != nil {
		return err
	}
	fmt.Printf("traininggen: emitted=%d gold=%d silver=%d\n", len(entries), len(rows), len(corpus.Entries))
	return nil
}

func checkTraining(ctx context.Context, opts runOptions) error {
	tmp, err := os.MkdirTemp("", "traininggen-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	regenerated := opts
	regenerated.OutDir = tmp
	regenerated.Check = false
	if err := run(ctx, regenerated); err != nil {
		return err
	}
	for _, name := range []string{"manifest.json", "COVERAGE.md"} {
		if err := compareFile(filepath.Join(tmp, name), filepath.Join(opts.OutDir, name), name); err != nil {
			return err
		}
	}
	fmt.Printf("traininggen: %s is up to date\n", opts.OutDir)
	return nil
}

func parseParityRows(path string) ([]parityRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	idx := strings.Index(text, "## Detailed Prompt Inventory")
	if idx < 0 {
		return nil, fmt.Errorf("%s missing Detailed Prompt Inventory", path)
	}
	text = text[idx:]
	var rows []parityRow
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") || strings.HasPrefix(line, "| Entry ") {
			continue
		}
		cols := splitMarkdownTableRow(line)
		if len(cols) != 7 {
			continue
		}
		id := strings.TrimSpace(cols[0])
		if id == "Z06" || id == "H04" || seen[id] {
			continue
		}
		workflowPaths := markdownLinks(cols[2])
		if len(workflowPaths) == 0 {
			continue
		}
		row := parityRow{
			Entry:         id,
			HCLPath:       firstMarkdownLink(cols[1]),
			WorkflowPaths: workflowPaths,
			Goal:          strings.TrimSpace(cols[4]),
			APISources:    parseAPISourceCell(cols[5]),
			ProjectName:   strings.Trim(strings.TrimSpace(cols[6]), "`"),
		}
		if row.Goal == "" || row.ProjectName == "" || len(row.APISources) == 0 {
			return nil, fmt.Errorf("parity row %s is missing goal, project name, or API source", id)
		}
		seen[id] = true
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Entry < rows[j].Entry })
	return rows, nil
}

func splitMarkdownTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func firstMarkdownLink(cell string) string {
	links := markdownLinks(cell)
	if len(links) == 0 {
		return ""
	}
	return links[0]
}

func markdownLinks(cell string) []string {
	var out []string
	for _, match := range markdownLinkRe.FindAllStringSubmatch(cell, -1) {
		out = append(out, repoPath(match[1]))
	}
	return out
}

func parseAPISourceCell(cell string) []trainingdata.APISource {
	cell = strings.Trim(cell, "` ")
	var out []trainingdata.APISource
	for _, part := range strings.Split(cell, ",") {
		part = strings.Trim(part, "` ")
		if part == "" || strings.EqualFold(part, "none") {
			continue
		}
		left, path, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		kind, id, ok := strings.Cut(left, ":")
		if !ok {
			continue
		}
		out = append(out, trainingdata.APISource{Kind: strings.TrimSpace(kind), ID: strings.TrimSpace(id), Path: repoPath(path)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func goldEntry(ctx context.Context, parityDoc string, row parityRow) (trainingdata.Entry, error) {
	primary := row.WorkflowPaths[0]
	doc, err := project.Load(primary)
	if err != nil {
		return trainingdata.Entry{}, err
	}
	resourceTypes, dataSources := projectTypes(doc)
	provider := providerForTypes(resourceTypes, dataSources)
	service := serviceForSources(row.APISources)
	validation, err := validateEntry(ctx, primary, row.APISources)
	if err != nil {
		return trainingdata.Entry{}, err
	}
	id := "gold-" + strings.ToLower(row.Entry)
	return trainingdata.Entry{
		ID:                  id,
		Tier:                "gold",
		Confidence:          1,
		ParityBacked:        true,
		NaturalLanguage:     trainingdata.NaturalLanguage{Goal: row.Goal, GoalSource: "curated"},
		WorkflowPaths:       row.WorkflowPaths,
		PrimaryWorkflowPath: primary,
		HCLPath:             row.HCLPath,
		Provider:            provider,
		Service:             service,
		ResourceTypes:       resourceTypes,
		DataSources:         dataSources,
		APISources:          row.APISources,
		Provenance: trainingdata.Provenance{
			SourceRepo: "github.com/OpenUdon/ramen",
			SourcePath: row.Entry,
			SourceDoc:  filepath.ToSlash(parityDoc),
		},
		Conversion: trainingdata.Conversion{Status: "parity-reviewed"},
		Validation: validation,
		Metadata:   map[string]string{"parity_row": row.Entry, "project_name": row.ProjectName},
	}, nil
}

func silverEntry(ctx context.Context, corpusRoot string, entry corpusEntry) (trainingdata.Entry, error) {
	entryDir := filepath.Join(corpusRoot, filepath.FromSlash(entry.Path))
	primary := filepath.ToSlash(filepath.Join(entryDir, "project.uws.yaml"))
	hclWorkflow := filepath.ToSlash(filepath.Join(entryDir, "project.uws.hcl"))
	hclPath := firstTerraformFile(filepath.Join(entryDir, "input"))
	doc, err := project.Load(primary)
	if err != nil {
		return trainingdata.Entry{}, err
	}
	apiSources := corpusAPISources(entry)
	validation, err := validateEntry(ctx, primary, apiSources)
	if err != nil {
		return trainingdata.Entry{}, err
	}
	resourceTypes, dataSources := projectTypes(doc)
	if len(resourceTypes) == 0 {
		resourceTypes = entry.ResourceTypes
	}
	if len(dataSources) == 0 {
		dataSources = entry.DataSources
	}
	sourceRepo := strings.TrimSpace(entry.SourceRepo)
	if sourceRepo == "" {
		sourceRepo = "unknown"
	}
	id := "silver-" + slug(entry.Path)
	return trainingdata.Entry{
		ID:                  id,
		Tier:                "silver",
		Confidence:          0.72,
		ParityBacked:        false,
		NaturalLanguage:     trainingdata.NaturalLanguage{Goal: silverGoal(entry.Provider, entry.Service, doc), GoalSource: "generated"},
		WorkflowPaths:       []string{primary, hclWorkflow},
		PrimaryWorkflowPath: primary,
		HCLPath:             hclPath,
		Provider:            entry.Provider,
		Service:             entry.Service,
		ResourceTypes:       resourceTypes,
		DataSources:         dataSources,
		APISources:          apiSources,
		Provenance: trainingdata.Provenance{
			SourceRepo:        sourceRepo,
			SourcePath:        entry.SourceDir,
			SourceDoc:         filepath.ToSlash(filepath.Join(entryDir, "meta.json")),
			ConversionCommand: conversionCommand(entryDir, apiSources),
		},
		Conversion: trainingdata.Conversion{Status: "clean"},
		Validation: validation,
	}, nil
}

func loadCorpusManifest(root string) (corpusManifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return corpusManifest{}, err
	}
	var m corpusManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return corpusManifest{}, err
	}
	if m.Version != "ramen.corpus.v1" {
		return corpusManifest{}, fmt.Errorf("unexpected corpus version %q", m.Version)
	}
	return m, nil
}

func corpusAPISources(entry corpusEntry) []trainingdata.APISource {
	if len(entry.APISources) > 0 {
		out := append([]trainingdata.APISource(nil), entry.APISources...)
		sortAPISources(out)
		return out
	}
	out := make([]trainingdata.APISource, 0, len(entry.SmithyModels))
	for _, model := range entry.SmithyModels {
		out = append(out, trainingdata.APISource{Kind: "aws-smithy", ID: model.ID, Path: model.Path})
	}
	sortAPISources(out)
	return out
}

func validateEntry(ctx context.Context, projectPath string, sources []trainingdata.APISource) (trainingdata.Validation, error) {
	inputs := make([]validate.APISourceInput, 0, len(sources))
	for _, source := range sources {
		inputs = append(inputs, validate.APISourceInput{Kind: source.Kind, ID: source.ID, Path: source.Path})
	}
	result, err := validate.Run(ctx, validate.Options{ProjectPath: projectPath, APISources: inputs, Strict: true})
	if err != nil {
		return trainingdata.Validation{}, err
	}
	status := "valid"
	if !result.Valid {
		status = "invalid"
		return trainingdata.Validation{Status: status, Strict: true, Summary: result.Summary}, fmt.Errorf("%s failed strict validation: %#v", projectPath, result.Diagnostics)
	}
	return trainingdata.Validation{Status: status, Strict: true, Summary: result.Summary}, nil
}

func projectTypes(doc *project.Document) ([]string, []string) {
	resources := map[string]bool{}
	dataSources := map[string]bool{}
	for _, resource := range doc.Profile.Resources {
		switch resource.Kind {
		case "data_source":
			dataSources[resource.Type] = true
		default:
			resources[resource.Type] = true
		}
	}
	return sortedKeys(resources), sortedKeys(dataSources)
}

func providerForTypes(resourceTypes, dataSources []string) string {
	for _, typ := range append(append([]string{}, resourceTypes...), dataSources...) {
		if provider, _, ok := strings.Cut(typ, "_"); ok {
			return provider
		}
	}
	return ""
}

func serviceForSources(sources []trainingdata.APISource) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0].ID
}

func silverGoal(provider, service string, doc *project.Document) string {
	api := providerDisplay(provider, service)
	if len(doc.Profile.Resources) == 0 {
		return "Create a Ramen workflow over the " + api + " API using clean converted Terraform/OpenTofu metadata."
	}
	resource := doc.Profile.Resources[0]
	attrs := sortedKeys(resource.Attributes)
	attrText := "no explicit converted attributes"
	if len(attrs) > 0 {
		limit := len(attrs)
		if limit > 4 {
			limit = 4
		}
		quoted := make([]string, 0, limit)
		for _, attr := range attrs[:limit] {
			quoted = append(quoted, "`"+attr+"`")
		}
		attrText = "converted attributes " + strings.Join(quoted, ", ")
	}
	credentialText := ""
	if len(resource.CredentialBindings) > 0 {
		credentialText = " and credential binding `" + resource.CredentialBindings[0] + "`"
	}
	return fmt.Sprintf("Create a Ramen workflow over the %s API that creates `%s` using %s%s.", api, resource.Address, attrText, credentialText)
}

func providerDisplay(provider, service string) string {
	switch provider {
	case "aws":
		return "AWS " + strings.ToUpper(service)
	case "azurerm":
		return "Azure " + service
	case "google":
		return "Google Cloud " + service
	case "cloudflare":
		return "Cloudflare " + service
	case "kubernetes":
		return "Kubernetes " + service
	default:
		return strings.TrimSpace(provider + " " + service)
	}
}

func firstTerraformFile(inputDir string) string {
	matches, _ := filepath.Glob(filepath.Join(inputDir, "*.tf"))
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return filepath.ToSlash(matches[0])
}

func conversionCommand(entryDir string, sources []trainingdata.APISource) string {
	args := []string{"go", "run", "./cmd/ramen", "convert", "--config-dir", filepath.ToSlash(filepath.Join(entryDir, "input"))}
	for _, source := range sources {
		args = append(args, "--api-source", source.Kind+":"+source.ID+"="+source.Path)
	}
	args = append(args, "--action", "create", "--out", filepath.ToSlash(filepath.Join(entryDir, "regen")))
	return strings.Join(args, " ")
}

func writeCoverage(path string, entries []trainingdata.Entry) error {
	var b strings.Builder
	b.WriteString("# Ramen training corpus coverage\n\n")
	b.WriteString("Generated by `go run ./cmd/traininggen`. Do not edit by hand.\n\n")
	gold, silver := 0, 0
	byTierProviderService := map[string]int{}
	byTierProvider := map[string]int{}
	for _, entry := range entries {
		switch entry.Tier {
		case "gold":
			gold++
		case "silver":
			silver++
		}
		byTierProvider[entry.Tier+"/"+entry.Provider]++
		byTierProviderService[entry.Tier+"/"+entry.Provider+"/"+entry.Service]++
	}
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(&b, "- entries: %d\n", len(entries))
	fmt.Fprintf(&b, "- gold: %d\n", gold)
	fmt.Fprintf(&b, "- silver: %d\n", silver)
	b.WriteString("\n## Entries by tier and provider\n\n")
	b.WriteString("| tier/provider | entries |\n|---|---:|\n")
	for _, key := range sortedKeys(byTierProvider) {
		fmt.Fprintf(&b, "| %s | %d |\n", key, byTierProvider[key])
	}
	b.WriteString("\n## Entries by tier, provider, and service\n\n")
	b.WriteString("| tier/provider/service | entries |\n|---|---:|\n")
	for _, key := range sortedKeys(byTierProviderService) {
		fmt.Fprintf(&b, "| %s | %d |\n", key, byTierProviderService[key])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
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

func compareFile(gotPath, wantPath, rel string) error {
	got, err := os.ReadFile(gotPath)
	if err != nil {
		return err
	}
	want, err := os.ReadFile(wantPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("training corpus drift: regenerated %s differs from committed output; re-run `go run ./cmd/traininggen`", rel)
	}
	return nil
}

func repoPath(path string) string {
	path = strings.TrimSpace(path)
	for strings.HasPrefix(path, "../") {
		path = strings.TrimPrefix(path, "../")
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func slug(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	var b strings.Builder
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func sortAPISources(sources []trainingdata.APISource) {
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].ID < sources[j].ID
	})
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
