package trainingdata

import "github.com/OpenUdon/ramen/validate"

const Version = "ramen.training.v1"

type Manifest struct {
	Version string  `json:"version"`
	Entries []Entry `json:"entries"`
}

type Entry struct {
	ID                  string            `json:"id"`
	Tier                string            `json:"tier"`
	Confidence          float64           `json:"confidence"`
	ParityBacked        bool              `json:"parity_backed"`
	NaturalLanguage     NaturalLanguage   `json:"natural_language"`
	WorkflowPaths       []string          `json:"workflow_paths"`
	PrimaryWorkflowPath string            `json:"primary_workflow_path"`
	HCLPath             string            `json:"hcl_path,omitempty"`
	Provider            string            `json:"provider,omitempty"`
	Service             string            `json:"service,omitempty"`
	ResourceTypes       []string          `json:"resource_types,omitempty"`
	DataSources         []string          `json:"data_sources,omitempty"`
	APISources          []APISource       `json:"api_sources,omitempty"`
	Provenance          Provenance        `json:"provenance"`
	Conversion          Conversion        `json:"conversion"`
	Validation          Validation        `json:"validation"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

type NaturalLanguage struct {
	Goal       string `json:"goal"`
	GoalSource string `json:"goal_source"`
}

type APISource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Path string `json:"path"`
}

type Provenance struct {
	SourceRepo        string `json:"source_repo"`
	SourcePath        string `json:"source_path"`
	SourceDoc         string `json:"source_doc,omitempty"`
	ConversionCommand string `json:"conversion_command,omitempty"`
}

type Conversion struct {
	Status string `json:"status"`
}

type Validation struct {
	Status  string           `json:"status"`
	Strict  bool             `json:"strict"`
	Summary validate.Summary `json:"summary"`
}
