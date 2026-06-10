// Command corpusgen builds a regression corpus of Terraform-to-UWS conversions.
//
// It scans provider fixtures for the services that ramen can currently map
// (derived from tfmapping.SupportedTypes), pairs each config with the matching
// API source document, runs ramen's conversion, and keeps only the conversions
// that map cleanly (no unsupported/fallback diagnostics). As mapping coverage
// grows, re-running corpusgen yields more entries with no hand-editing. Large
// API source documents are referenced by path, not copied.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/OpenUdon/ramen/internal/tfconvert"
	"github.com/OpenUdon/ramen/tfmapping"
	ramenvalidate "github.com/OpenUdon/ramen/validate"
	"github.com/OpenUdon/uws/convert"
)

const (
	defaultAWSProviderDir        = "../terraform-provider-aws"
	defaultAWSSmithyDir          = "../apitools/catalog-openapi-cache/aws-smithy"
	defaultGoogleProviderDir     = "../terraform-provider-google"
	defaultGoogleDiscoveryDir    = "../apitools/catalog-openapi-cache/google-discovery"
	defaultAzureProviderDir      = "../terraform-provider-azurerm"
	defaultAzureOpenAPIDir       = "../apitools/catalog-openapi-cache/openapi"
	defaultCloudflareProviderDir = "../terraform-provider-cloudflare"
	defaultCloudflareOpenAPIDir  = "testdata/api-sources"
	defaultKubernetesProviderDir = "../terraform-provider-kubernetes"
	defaultKubernetesOpenAPIDir  = "../apitools/catalog-openapi-cache/openapi"
	defaultOpenTofuDir           = "../opentofu"
	defaultOutDir                = "testdata/corpus"
)

var (
	resourceRe    = regexp.MustCompile(`(?m)^\s*resource\s+"((?:aws|google|azurerm|cloudflare|kubernetes)_[A-Za-z0-9_]+)"`)
	dataRe        = regexp.MustCompile(`(?m)^\s*data\s+"((?:aws|google|azurerm|cloudflare|kubernetes)_[A-Za-z0-9_]+)"`)
	placeholderRe = regexp.MustCompile(`%\{([A-Za-z0-9_]+)\}`)
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

type apiSourceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Path string `json:"path"`
}

type entryMeta struct {
	Path          string         `json:"path"`
	Provider      string         `json:"provider,omitempty"`
	Service       string         `json:"service"`
	ResourceTypes []string       `json:"resource_types"`
	DataSources   []string       `json:"data_sources,omitempty"`
	APISources    []apiSourceRef `json:"api_sources,omitempty"`
	SmithyModels  []modelRef     `json:"smithy_models,omitempty"`
	SourceDir     string         `json:"source_dir"`
	SourceRepo    string         `json:"source_repo,omitempty"`
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
	droppedValidation  int
	droppedHCL         int
	droppedTemplate    int
	emitted            int
	perService         map[string]int
}

func main() {
	awsProviderDir := flag.String("provider-dir", defaultAWSProviderDir, "Path to the terraform-provider-aws checkout")
	awsSmithyDir := flag.String("smithy-dir", defaultAWSSmithyDir, "Directory of aws-<service>-smithy-model.json files")
	googleProviderDir := flag.String("google-provider-dir", defaultGoogleProviderDir, "Path to the terraform-provider-google checkout")
	googleDiscoveryDir := flag.String("google-discovery-dir", defaultGoogleDiscoveryDir, "Directory of Google discovery JSON files")
	azureProviderDir := flag.String("azure-provider-dir", defaultAzureProviderDir, "Path to the terraform-provider-azurerm checkout")
	azureOpenAPIDir := flag.String("azure-openapi-dir", defaultAzureOpenAPIDir, "Directory of Azure OpenAPI JSON files")
	cloudflareProviderDir := flag.String("cloudflare-provider-dir", defaultCloudflareProviderDir, "Path to the terraform-provider-cloudflare checkout")
	cloudflareOpenAPIDir := flag.String("cloudflare-openapi-dir", defaultCloudflareOpenAPIDir, "Directory of Cloudflare OpenAPI YAML/JSON files")
	kubernetesProviderDir := flag.String("kubernetes-provider-dir", defaultKubernetesProviderDir, "Path to the terraform-provider-kubernetes checkout")
	kubernetesOpenAPIDir := flag.String("kubernetes-openapi-dir", defaultKubernetesOpenAPIDir, "Directory of Kubernetes OpenAPI/Swagger JSON files")
	openTofuDir := flag.String("opentofu-dir", defaultOpenTofuDir, "Path to the OpenTofu checkout for optional static documentation/example Terraform inputs")
	providers := flag.String("providers", "aws,google,azurerm,cloudflare,kubernetes", "Comma-separated providers to scan: aws, google, azurerm, cloudflare, kubernetes")
	outDir := flag.String("out", defaultOutDir, "Corpus output directory (relative to repo root)")
	action := flag.String("action", "create", "Desired action passed to ramen convert")
	check := flag.Bool("check", false, "Regenerate into a temporary directory and fail if committed corpus output is stale")
	flag.Parse()

	if err := run(runOptions{
		AWSProviderDir:        *awsProviderDir,
		AWSSmithyDir:          *awsSmithyDir,
		GoogleProviderDir:     *googleProviderDir,
		GoogleDiscoveryDir:    *googleDiscoveryDir,
		AzureProviderDir:      *azureProviderDir,
		AzureOpenAPIDir:       *azureOpenAPIDir,
		CloudflareProviderDir: *cloudflareProviderDir,
		CloudflareOpenAPIDir:  *cloudflareOpenAPIDir,
		KubernetesProviderDir: *kubernetesProviderDir,
		KubernetesOpenAPIDir:  *kubernetesOpenAPIDir,
		OpenTofuDir:           *openTofuDir,
		Providers:             *providers,
		OutDir:                *outDir,
		Action:                *action,
		Check:                 *check,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "corpusgen:", err)
		os.Exit(1)
	}
}

type runOptions struct {
	AWSProviderDir        string
	AWSSmithyDir          string
	GoogleProviderDir     string
	GoogleDiscoveryDir    string
	AzureProviderDir      string
	AzureOpenAPIDir       string
	CloudflareProviderDir string
	CloudflareOpenAPIDir  string
	KubernetesProviderDir string
	KubernetesOpenAPIDir  string
	OpenTofuDir           string
	Providers             string
	OutDir                string
	Action                string
	Check                 bool
}

type providerSpec struct {
	Name        string
	ProviderDir string
	SourceDir   string
	SourceKind  string
	SourceRepo  string
	testInputs  func(string, string) ([]configInput, error)
	findSource  func(string, string) (string, bool)
}

func run(opts runOptions) error {
	if opts.Check {
		return checkCorpus(opts)
	}
	ctx := context.Background()
	registry := tfmapping.DefaultRegistry()
	providerSpecs := specsForOptions(opts)
	if len(providerSpecs) == 0 {
		return fmt.Errorf("no enabled provider checkouts found")
	}

	st := &stats{perService: map[string]int{}}
	var entries []entryMeta
	emittedDirs := map[string]map[string]bool{}

	for _, spec := range providerSpecs {
		mapping, err := mappedProviderTypes(registry, spec.Name)
		if err != nil {
			return err
		}
		serviceSource := map[string]string{}
		for svc := range mapping.services {
			if path, ok := spec.findSource(spec.SourceDir, svc); ok {
				serviceSource[svc] = path
			} else {
				st.servicesNoModel = append(st.servicesNoModel, spec.Name+"/"+svc)
			}
		}
		sort.Strings(st.servicesNoModel)

		providerEmittedDirs := map[string]bool{}
		for _, svc := range sortedKeys(mapping.services) {
			if _, ok := serviceSource[svc]; !ok {
				continue
			}
			st.servicesScanned++
			inputs, err := spec.testInputs(spec.ProviderDir, svc)
			if err != nil {
				return err
			}
			for _, input := range inputs {
				meta, emitted, err := processConfigDir(ctx, processArgs{
					input:          input,
					provider:       spec.Name,
					service:        svc,
					providerDir:    spec.ProviderDir,
					outDir:         opts.OutDir,
					action:         opts.Action,
					sourceKind:     spec.SourceKind,
					sourceRepo:     spec.SourceRepo,
					mappedKinds:    mapping.mappedKinds,
					serviceForType: mapping.serviceForType,
					serviceSource:  serviceSource,
				}, st)
				if err != nil {
					return err
				}
				if emitted {
					entries = append(entries, meta)
					providerEmittedDirs[filepath.Join(opts.OutDir, filepath.FromSlash(meta.Path))] = true
					st.emitted++
					st.perService[spec.Name+"/"+svc]++
				}
			}
		}
		emittedDirs[spec.Name] = providerEmittedDirs
	}

	if dirExists(opts.OpenTofuDir) {
		openTofuEntries, openTofuDirs, err := processOpenTofuInputs(ctx, opts, registry, providerSpecs, st)
		if err != nil {
			return err
		}
		entries = append(entries, openTofuEntries...)
		if len(openTofuDirs) > 0 {
			emittedDirs["opentofu"] = openTofuDirs
		}
	}

	for provider, dirs := range emittedDirs {
		pruneStaleEntries(filepath.Join(opts.OutDir, provider), dirs)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if err := writeManifest(opts.OutDir, entries); err != nil {
		return err
	}
	if err := writeCoverage(opts.OutDir, st, registry); err != nil {
		return err
	}
	printSummary(st)
	return nil
}

func checkCorpus(opts runOptions) error {
	tmp, err := os.MkdirTemp("", "corpusgen-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	regenerated := opts
	regenerated.OutDir = tmp
	regenerated.Check = false
	if err := run(regenerated); err != nil {
		return err
	}
	if err := compareCorpusTrees(tmp, opts.OutDir); err != nil {
		return err
	}
	fmt.Printf("corpusgen: %s is up to date\n", opts.OutDir)
	return nil
}

func specsForOptions(opts runOptions) []providerSpec {
	enabled := map[string]bool{}
	for _, p := range strings.Split(opts.Providers, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			enabled[p] = true
		}
	}
	var specs []providerSpec
	if enabled["aws"] && dirExists(opts.AWSProviderDir) {
		specs = append(specs, providerSpec{
			Name:        "aws",
			ProviderDir: opts.AWSProviderDir,
			SourceDir:   opts.AWSSmithyDir,
			SourceKind:  tfmapping.APISourceKindAWSSmithy,
			SourceRepo:  "terraform-provider-aws",
			testInputs: func(providerDir, svc string) ([]configInput, error) {
				return configInputs(filepath.Join(providerDir, "internal", "service", svc, "testdata"))
			},
			findSource: findSmithyModel,
		})
	}
	if enabled["google"] && dirExists(opts.GoogleProviderDir) {
		specs = append(specs, providerSpec{
			Name:        "google",
			ProviderDir: opts.GoogleProviderDir,
			SourceDir:   opts.GoogleDiscoveryDir,
			SourceKind:  tfmapping.APISourceKindGoogleDiscovery,
			SourceRepo:  "terraform-provider-google",
			testInputs: func(providerDir, svc string) ([]configInput, error) {
				return googleConfigInputs(filepath.Join(providerDir, "google", "services", svc))
			},
			findSource: findGoogleDiscoveryModel,
		})
	}
	if enabled["azurerm"] && dirExists(opts.AzureProviderDir) {
		specs = append(specs, providerSpec{
			Name:        "azurerm",
			ProviderDir: opts.AzureProviderDir,
			SourceDir:   opts.AzureOpenAPIDir,
			SourceKind:  tfmapping.APISourceKindOpenAPI,
			SourceRepo:  "terraform-provider-azurerm",
			testInputs: func(providerDir, svc string) ([]configInput, error) {
				return azureRMConfigInputs(filepath.Join(providerDir, "internal", "services", svc))
			},
			findSource: findAzureRMOpenAPIModel,
		})
	}
	if enabled["cloudflare"] && dirExists(opts.CloudflareProviderDir) {
		specs = append(specs, providerSpec{
			Name:        "cloudflare",
			ProviderDir: opts.CloudflareProviderDir,
			SourceDir:   opts.CloudflareOpenAPIDir,
			SourceKind:  tfmapping.APISourceKindOpenAPI,
			SourceRepo:  "terraform-provider-cloudflare",
			testInputs: func(providerDir, svc string) ([]configInput, error) {
				return cloudflareConfigInputs(filepath.Join(providerDir, "internal", "services", svc, "testdata"))
			},
			findSource: findCloudflareOpenAPIModel,
		})
	}
	if enabled["kubernetes"] && dirExists(opts.KubernetesProviderDir) {
		specs = append(specs, providerSpec{
			Name:        "kubernetes",
			ProviderDir: opts.KubernetesProviderDir,
			SourceDir:   opts.KubernetesOpenAPIDir,
			SourceKind:  tfmapping.APISourceKindOpenAPI,
			SourceRepo:  "terraform-provider-kubernetes",
			testInputs: func(providerDir, svc string) ([]configInput, error) {
				return terraformFileInputs(filepath.Join(providerDir, "examples"))
			},
			findSource: findKubernetesOpenAPIModel,
		})
	}
	return specs
}

type providerMapping struct {
	mappedKinds    map[string]map[string]bool
	serviceForType map[string]string
	services       map[string]bool
}

func mappedProviderTypes(registry tfmapping.Registry, provider string) (providerMapping, error) {
	out := providerMapping{
		mappedKinds: map[string]map[string]bool{
			"resource":    {},
			"data_source": {},
		},
		serviceForType: map[string]string{},
		services:       map[string]bool{},
	}
	for _, t := range registry.SupportedTypes() {
		if t.Provider != provider {
			continue
		}
		svc := serviceForTerraformType(provider, t.Type)
		out.serviceForType[t.Type] = svc
		for _, kind := range t.Kinds {
			if _, ok := out.mappedKinds[kind]; ok {
				out.mappedKinds[kind][t.Type] = true
			}
			if kind == "resource" {
				out.services[svc] = true
			}
		}
	}
	if len(out.services) == 0 {
		return out, fmt.Errorf("no mapped %s resource or data source types found in tfmapping registry", provider)
	}
	return out, nil
}

func processOpenTofuInputs(ctx context.Context, opts runOptions, registry tfmapping.Registry, specs []providerSpec, st *stats) ([]entryMeta, map[string]bool, error) {
	inputs, err := openTofuConfigInputs(opts.OpenTofuDir)
	if err != nil {
		return nil, nil, err
	}
	if len(inputs) == 0 {
		return nil, nil, nil
	}

	specByProvider := map[string]providerSpec{}
	mappingByProvider := map[string]providerMapping{}
	sourceByProvider := map[string]map[string]string{}
	for _, spec := range specs {
		specByProvider[spec.Name] = spec
		mapping, err := mappedProviderTypes(registry, spec.Name)
		if err != nil {
			return nil, nil, err
		}
		mappingByProvider[spec.Name] = mapping
		serviceSource := map[string]string{}
		for svc := range mapping.services {
			if path, ok := spec.findSource(spec.SourceDir, svc); ok {
				serviceSource[svc] = path
			}
		}
		sourceByProvider[spec.Name] = serviceSource
	}

	var entries []entryMeta
	emittedDirs := map[string]bool{}
	for _, input := range inputs {
		rendered, resources, _, err := prepareInput(input)
		if err != nil {
			return nil, nil, err
		}
		if len(resources) == 0 {
			st.configsConsidered++
			st.droppedNoResource++
			continue
		}
		provider := providerForTerraformTypes(resources)
		spec, ok := specByProvider[provider]
		if !ok {
			st.configsConsidered++
			st.droppedUnsupported++
			continue
		}
		mapping := mappingByProvider[provider]
		services := map[string]bool{}
		supported := true
		for _, rt := range resources {
			if !mapping.mappedKinds["resource"][rt] {
				supported = false
				break
			}
			services[mapping.serviceForType[rt]] = true
		}
		if !supported || len(services) != 1 {
			st.configsConsidered++
			st.droppedUnsupported++
			continue
		}
		service := sortedKeys(services)[0]
		if _, ok := sourceByProvider[provider][service]; !ok {
			st.configsConsidered++
			st.droppedNoModel++
			continue
		}
		input.Content = rendered
		meta, emitted, err := processConfigDir(ctx, processArgs{
			input:          input,
			provider:       provider,
			service:        service,
			providerDir:    opts.OpenTofuDir,
			outDir:         opts.OutDir,
			action:         opts.Action,
			sourceKind:     spec.SourceKind,
			sourceRepo:     "opentofu",
			entryPrefix:    "opentofu",
			mappedKinds:    mapping.mappedKinds,
			serviceForType: mapping.serviceForType,
			serviceSource:  sourceByProvider[provider],
		}, st)
		if err != nil {
			return nil, nil, err
		}
		if emitted {
			entries = append(entries, meta)
			emittedDirs[filepath.Join(opts.OutDir, filepath.FromSlash(meta.Path))] = true
			st.emitted++
			st.perService[provider+"/"+service]++
		}
	}
	return entries, emittedDirs, nil
}

func providerForTerraformTypes(types []string) string {
	providers := map[string]bool{}
	for _, typ := range types {
		parts := strings.SplitN(typ, "_", 2)
		if len(parts) == 2 {
			providers[parts[0]] = true
		}
	}
	if len(providers) != 1 {
		return ""
	}
	return sortedKeys(providers)[0]
}

type processArgs struct {
	input          configInput
	provider       string
	service        string
	providerDir    string
	outDir         string
	action         string
	sourceKind     string
	sourceRepo     string
	entryPrefix    string
	mappedKinds    map[string]map[string]bool
	serviceForType map[string]string
	serviceSource  map[string]string
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
		if !a.mappedKinds["resource"][rt] {
			st.droppedUnsupported++
			return entryMeta{}, false, nil
		}
	}
	sources, ok := sourcesForTypes(resources, a.sourceKind, a.serviceForType, a.serviceSource)
	if !ok {
		st.droppedNoModel++
		return entryMeta{}, false, nil
	}

	parts := []string{a.provider, a.service, a.input.EntryRel}
	if strings.TrimSpace(a.entryPrefix) != "" {
		parts = append([]string{a.entryPrefix}, parts...)
	}
	entryRel := filepath.Join(parts...)
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

	apiSources := make([]tfconvert.APISourceInput, 0, len(sources))
	for _, src := range sources {
		apiSources = append(apiSources, tfconvert.APISourceInput{Kind: src.Kind, ID: src.ID, Path: src.Path})
	}
	res, err := tfconvert.Convert(ctx, tfconvert.Options{
		ConfigDir:  inputDir,
		APISources: apiSources,
		Action:     a.action,
		OutDir:     tmp,
	})
	if err != nil || res == nil || !cleanDiagnostics(res.Diagnostics) {
		os.RemoveAll(entryDir)
		st.droppedDiagnostics++
		return entryMeta{}, false, nil
	}
	validateInputs := make([]ramenvalidate.APISourceInput, 0, len(apiSources))
	for _, src := range apiSources {
		validateInputs = append(validateInputs, ramenvalidate.APISourceInput{Kind: src.Kind, ID: src.ID, Path: src.Path})
	}
	validation, err := ramenvalidate.Run(ctx, ramenvalidate.Options{
		ProjectPath: res.NativeProjectPath,
		APISources:  validateInputs,
		Strict:      true,
	})
	if err != nil || !validation.Valid {
		os.RemoveAll(entryDir)
		st.droppedValidation++
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
		Provider:      a.provider,
		Service:       a.service,
		ResourceTypes: resources,
		DataSources:   datas,
		SourceDir:     filepath.ToSlash(mustRel(a.providerDir, a.input.Path)),
		SourceRepo:    a.sourceRepo,
	}
	if a.sourceKind == tfmapping.APISourceKindAWSSmithy {
		for _, src := range sources {
			meta.SmithyModels = append(meta.SmithyModels, modelRef{ID: src.ID, Path: src.Path})
		}
	} else {
		meta.APISources = sources
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
	rendered = normalizeTerraformBytes(rendered)
	if input.Template {
		return os.WriteFile(filepath.Join(inputDir, "main.tf"), rendered, 0o644)
	}
	if len(input.Content) > 0 {
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

func compareCorpusTrees(gotDir, wantDir string) error {
	gotFiles, err := corpusFiles(gotDir)
	if err != nil {
		return err
	}
	wantFiles, err := corpusFiles(wantDir)
	if err != nil {
		return err
	}
	for _, rel := range sortedKeys(wantFiles) {
		if !gotFiles[rel] {
			return fmt.Errorf("corpus drift: missing regenerated file %s", rel)
		}
		if err := compareCorpusFile(filepath.Join(gotDir, filepath.FromSlash(rel)), filepath.Join(wantDir, filepath.FromSlash(rel)), rel); err != nil {
			return err
		}
	}
	for _, rel := range sortedKeys(gotFiles) {
		if !wantFiles[rel] {
			return fmt.Errorf("corpus drift: extra regenerated file %s", rel)
		}
	}
	return nil
}

func corpusFiles(root string) (map[string]bool, error) {
	out := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = true
		return nil
	})
	return out, err
}

func compareCorpusFile(gotPath, wantPath, rel string) error {
	got, err := os.ReadFile(gotPath)
	if err != nil {
		return err
	}
	want, err := os.ReadFile(wantPath)
	if err != nil {
		return err
	}
	if isProjectDocument(rel) {
		equal, err := projectDocumentsEqual(got, want, rel)
		if err != nil {
			return err
		}
		if !equal {
			return fmt.Errorf("corpus drift: regenerated %s differs structurally from committed output", rel)
		}
		return nil
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("corpus drift: regenerated %s differs from committed output; re-run `go run ./cmd/corpusgen`", rel)
	}
	return nil
}

func isProjectDocument(rel string) bool {
	switch filepath.Base(rel) {
	case "project.uws.yaml", "project.uws.hcl":
		return true
	default:
		return false
	}
}

func projectDocumentsEqual(got, want []byte, rel string) (bool, error) {
	gotDoc, err := decodeProjectDocument(got, rel)
	if err != nil {
		return false, err
	}
	wantDoc, err := decodeProjectDocument(want, rel)
	if err != nil {
		return false, err
	}
	normalizeConfigDir(gotDoc)
	normalizeConfigDir(wantDoc)
	return reflect.DeepEqual(gotDoc, wantDoc), nil
}

func decodeProjectDocument(data []byte, rel string) (any, error) {
	var jsonData []byte
	var err error
	switch filepath.Ext(rel) {
	case ".hcl":
		jsonData, err = convert.HCLToJSON(data)
	case ".yaml", ".yml":
		jsonData, err = convert.YAMLToJSON(data)
	default:
		return nil, fmt.Errorf("unsupported project document %s", rel)
	}
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	var doc any
	if err := json.Unmarshal(jsonData, &doc); err != nil {
		return nil, fmt.Errorf("decode %s: %w", rel, err)
	}
	return doc, nil
}

func normalizeConfigDir(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if k == "config_dir" {
				x[k] = "<normalized-config-dir>"
				continue
			}
			normalizeConfigDir(child)
		}
	case []any:
		for _, child := range x {
			normalizeConfigDir(child)
		}
	}
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

func sourcesForTypes(types []string, kind string, serviceForType map[string]string, serviceSource map[string]string) ([]apiSourceRef, bool) {
	seen := map[string]bool{}
	var out []apiSourceRef
	for _, rt := range types {
		svc := serviceForType[rt]
		if svc == "" {
			svc = serviceForResourceType(rt)
		}
		path, ok := serviceSource[svc]
		if !ok {
			return nil, false
		}
		if seen[svc] {
			continue
		}
		seen[svc] = true
		out = append(out, apiSourceRef{Kind: kind, ID: svc, Path: filepath.ToSlash(path)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, true
}

type configInput struct {
	Path     string
	EntryRel string
	Template bool
	Content  []byte
}

func prepareInput(input configInput) ([]byte, []string, []string, error) {
	if len(input.Content) > 0 {
		data := normalizeTerraformBytes(input.Content)
		resources, datas := extractTypesFromBytes(data)
		return data, resources, datas, nil
	}
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

func normalizeTerraformBytes(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	return data
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
	parts := strings.SplitN(t, "_", 3)
	if len(parts) < 2 {
		return t
	}
	return parts[1]
}

func serviceForTerraformType(provider, t string) string {
	switch provider {
	case "azurerm":
		if strings.HasPrefix(t, "azurerm_cosmosdb_") {
			return "cosmos"
		}
	case "cloudflare":
		switch {
		case strings.HasPrefix(t, "cloudflare_r2_bucket"):
			return "r2_bucket"
		case strings.HasPrefix(t, "cloudflare_d1_database"):
			return "d1_database"
		}
	case "kubernetes":
		return "core"
	}
	return serviceForResourceType(t)
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

func findGoogleDiscoveryModel(dir, svc string) (string, bool) {
	candidates := []string{
		"google-cloud-" + svc + "-discovery-v1.json",
		"google-" + svc + "-discovery-v1.json",
		svc + "-discovery-v1.json",
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func findAzureRMOpenAPIModel(dir, svc string) (string, bool) {
	candidates := []string{
		"azure-" + svc + "-resource-manager-openapi.json",
		"azure-" + svc + "-db-resource-manager-openapi.json",
		svc + "-resource-manager-openapi.json",
	}
	if svc == "cosmos" {
		candidates = append([]string{"azure-cosmos-db-resource-manager-openapi.json"}, candidates...)
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func findKubernetesOpenAPIModel(dir, svc string) (string, bool) {
	candidates := []string{"kubernetes-v1-19-2-swagger.json", "k8s-swagger.json", "kubernetes-openapi.json", "kubernetes-swagger.json"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func findCloudflareOpenAPIModel(dir, svc string) (string, bool) {
	candidates := []string{"cloudflare-r2-d1-openapi.json", "cloudflare-api-openapi.yaml", "cloudflare-api-openapi.json", "cloudflare-openapi.yaml"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func googleConfigInputs(serviceDir string) ([]configInput, error) {
	return goTestConfigInputs(serviceDir, sanitizeGoTerraformSnippet)
}

func azureRMConfigInputs(serviceDir string) ([]configInput, error) {
	return goTestConfigInputs(serviceDir, sanitizeAzureRMTerraformSnippet)
}

func cloudflareConfigInputs(testdata string) ([]configInput, error) {
	return sanitizedTerraformFileInputs(testdata, sanitizeCloudflareTerraformSnippet)
}

func goTestConfigInputs(serviceDir string, sanitize func(string) string) ([]configInput, error) {
	var out []configInput
	err := filepath.WalkDir(serviceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		inputs, err := terraformSnippetsFromGoTest(path, sanitize)
		if err != nil {
			return err
		}
		out = append(out, inputs...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryRel < out[j].EntryRel })
	return out, nil
}

func terraformFileInputs(root string) ([]configInput, error) {
	return sanitizedTerraformFileInputs(root, nil)
}

func openTofuConfigInputs(root string) ([]configInput, error) {
	var out []configInput
	for _, relRoot := range []string{
		"website/docs",
		"docs",
		"testing/equivalence-tests",
	} {
		inputs, err := sanitizedTerraformFileInputs(filepath.Join(root, filepath.FromSlash(relRoot)), nil)
		if err != nil {
			return nil, err
		}
		for _, input := range inputs {
			rel, err := filepath.Rel(root, input.Path)
			if err != nil {
				return nil, err
			}
			input.EntryRel = strings.TrimSuffix(filepath.ToSlash(rel), ".tf")
			out = append(out, input)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryRel < out[j].EntryRel })
	return out, nil
}

func sanitizedTerraformFileInputs(root string, sanitize func(string) string) ([]configInput, error) {
	var out []configInput
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".tf") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if sanitize != nil {
			text = sanitize(text)
			data = []byte(text)
		}
		resources, datas := extractTypesFromBytes(data)
		if len(resources) == 0 && len(datas) == 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entryRel := strings.TrimSuffix(filepath.ToSlash(rel), ".tf")
		out = append(out, configInput{Path: path, EntryRel: entryRel, Content: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryRel < out[j].EntryRel })
	return out, nil
}

func terraformSnippetsFromGoTest(path string, sanitize func(string) string) ([]configInput, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []configInput
	base := strings.TrimSuffix(filepath.Base(path), ".go")
	idx := 0
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		text = sanitize(text)
		resources, datas := extractTypesFromBytes([]byte(text))
		if len(resources) == 0 && len(datas) == 0 {
			return true
		}
		if seen[text] {
			return true
		}
		seen[text] = true
		idx++
		out = append(out, configInput{
			Path:     path,
			EntryRel: filepath.ToSlash(filepath.Join("go", fmt.Sprintf("%s_%03d", base, idx))),
			Content:  []byte(text),
		})
		return true
	})
	return out, nil
}

func sanitizeGoTerraformSnippet(text string) string {
	text = placeholderRe.ReplaceAllStringFunc(text, func(match string) string {
		name := placeholderRe.FindStringSubmatch(match)[1]
		switch {
		case strings.Contains(name, "project"):
			return "ramen-corpus-project"
		case strings.Contains(name, "bucket"):
			return "ramen-corpus-bucket"
		case strings.Contains(name, "location"):
			return "US"
		default:
			return "ramen-corpus"
		}
	})
	text = replaceFmtVerbs(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	return text + "\n"
}

func sanitizeAzureRMTerraformSnippet(text string) string {
	text = replaceFmtVerbs(text)
	replacements := map[string]string{
		`azurerm_resource_group.test.location`: `"eastus"`,
		`azurerm_resource_group.test.name`:     `"ramen-corpus-rg"`,
	}
	for old, newValue := range replacements {
		text = strings.ReplaceAll(text, old, newValue)
	}
	text = regexp.MustCompile(`kind\s*=\s*"ramen-corpus"`).ReplaceAllString(text, `kind = "GlobalDocumentDB"`)
	text = regexp.MustCompile(`consistency_level\s*=\s*"ramen-corpus"`).ReplaceAllString(text, `consistency_level = "Session"`)
	text = regexp.MustCompile(`type\s*=\s*"ramen-corpus"`).ReplaceAllString(text, `type = "SystemAssigned"`)
	text = removeTerraformBlocks(text, "resource", "azurerm_resource_group")
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	return text + "\n"
}

func sanitizeCloudflareTerraformSnippet(text string) string {
	replacements := map[string]string{
		"%[1]s": "ramen_corpus",
		"%[2]s": "023e105f4ecef8ad9ca31a8372d0c353",
		"%[3]s": "ENAM",
		"%s":    "ramen_corpus",
	}
	for old, newValue := range replacements {
		text = strings.ReplaceAll(text, old, newValue)
	}
	text = regexp.MustCompile(`storage_class(\s*=\s*)"ENAM"`).ReplaceAllString(text, `storage_class${1}"Standard"`)
	text = regexp.MustCompile(`jurisdiction(\s*=\s*)"ENAM"`).ReplaceAllString(text, `jurisdiction${1}"eu"`)
	text = replaceFmtVerbs(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	return text + "\n"
}

func removeTerraformBlocks(text, kind, typ string) string {
	pattern := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(kind) + `[ \t]+"` + regexp.QuoteMeta(typ) + `"[ \t]+"[^"]+"[ \t]*\{`)
	var b strings.Builder
	for {
		loc := pattern.FindStringIndex(text)
		if loc == nil {
			b.WriteString(text)
			return b.String()
		}
		b.WriteString(text[:loc[0]])
		end := terraformBlockEnd(text, loc[1]-1)
		if end <= loc[1] {
			b.WriteString(text[loc[0]:])
			return b.String()
		}
		for end < len(text) && (text[end] == '\n' || text[end] == '\r') {
			end++
		}
		text = text[end:]
	}
}

func terraformBlockEnd(text string, openBrace int) int {
	depth := 0
	inString := false
	escaped := false
	for i := openBrace; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(text)
}

func replaceFmtVerbs(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '%' {
			b.WriteByte(text[i])
			i++
			continue
		}
		if i+1 < len(text) && text[i+1] == '%' {
			b.WriteByte('%')
			i += 2
			continue
		}
		j := i + 1
		if j < len(text) && text[j] == '[' {
			for j < len(text) && text[j] != ']' {
				j++
			}
			if j < len(text) {
				j++
			}
		}
		for j < len(text) && strings.ContainsRune("#+- 0.123456789", rune(text[j])) {
			j++
		}
		if j >= len(text) {
			b.WriteByte('%')
			i++
			continue
		}
		switch text[j] {
		case 'd', 'b', 'o', 'x', 'X', 'U':
			b.WriteString("1")
		default:
			b.WriteString("ramen-corpus")
		}
		i = j + 1
	}
	return b.String()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
	fmt.Fprintf(&b, "- dropped (unsupported Terraform type): %d\n", st.droppedUnsupported)
	fmt.Fprintf(&b, "- dropped (no API source document for a needed service): %d\n", st.droppedNoModel)
	fmt.Fprintf(&b, "- dropped (fallback/unsupported/error diagnostics): %d\n", st.droppedDiagnostics)
	fmt.Fprintf(&b, "- dropped (strict native validation): %d\n", st.droppedValidation)
	fmt.Fprintf(&b, "- dropped (HCL round-trip not yet clean): %d\n", st.droppedHCL)
	fmt.Fprintf(&b, "- dropped (template render failed): %d\n", st.droppedTemplate)
	if len(st.servicesNoModel) > 0 {
		fmt.Fprintf(&b, "- mapped services without a local API source document: %s\n", strings.Join(st.servicesNoModel, ", "))
	}
	b.WriteString("\n## Emitted entries by service\n\n")
	b.WriteString("| provider/service | entries |\n|---|---|\n")
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
	fmt.Printf("corpusgen: emitted=%d considered=%d services=%d dropped(unsupported=%d no-resource=%d no-model=%d diagnostics=%d validation=%d hcl=%d template=%d)\n",
		st.emitted, st.configsConsidered, st.servicesScanned,
		st.droppedUnsupported, st.droppedNoResource, st.droppedNoModel, st.droppedDiagnostics, st.droppedValidation, st.droppedHCL, st.droppedTemplate)
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
