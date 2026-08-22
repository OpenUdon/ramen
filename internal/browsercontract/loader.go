// Package browsercontract loads and validates the UWS browser contracts that
// Ramen needs while projecting desired state. It contains no browser runtime.
package browsercontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenUdon/evidence/digest"
	"github.com/OpenUdon/uws/browserauthentication"
	"github.com/OpenUdon/uws/schemas"
	"gopkg.in/yaml.v3"
)

const maxBrowserProfileBytes int64 = 20 << 20

// Profile is the validated, Ramen-relevant projection of a UWS browser source
// profile. Sequence and parameter shapes remain inert source metadata.
type Profile struct {
	Path               string
	Digest             string
	Version            string
	LoginStateRequired bool
	Contexts           map[string]Context
	Actions            map[string]Action
}

type Context struct {
	Kind   string `json:"kind" yaml:"kind"`
	Parent string `json:"parent" yaml:"parent"`
	Origin string `json:"origin" yaml:"origin"`
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
}

type Action struct {
	Parameters         map[string]any     `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Sequence           []map[string]any   `json:"sequence" yaml:"sequence"`
	Outputs            map[string]Output  `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	SideEffects        []string           `json:"sideEffects" yaml:"sideEffects"`
	ConfirmationPolicy ConfirmationPolicy `json:"confirmationPolicy" yaml:"confirmationPolicy"`
}

type Output struct {
	Type     string `json:"type" yaml:"type"`
	Source   string `json:"source" yaml:"source"`
	Context  string `json:"context,omitempty" yaml:"context,omitempty"`
	Presence bool   `json:"presence,omitempty" yaml:"presence,omitempty"`
}

type ConfirmationPolicy struct {
	Required bool   `json:"required" yaml:"required"`
	Prompt   string `json:"prompt,omitempty" yaml:"prompt,omitempty"`
}

// AuthenticationProfile is one validated UWS browser-authentication profile.
type AuthenticationProfile struct {
	Path    string
	Digest  string
	Profile browserauthentication.Profile
}

type profileWire struct {
	Profile string `json:"profile" yaml:"profile"`
	Info    struct {
		LoginStateRequired bool `json:"loginStateRequired" yaml:"loginStateRequired"`
	} `json:"info" yaml:"info"`
	Contexts map[string]Context `json:"contexts,omitempty" yaml:"contexts,omitempty"`
	Actions  map[string]Action  `json:"actions" yaml:"actions"`
}

// LoadProfile validates and decodes a contained browser source profile.
func LoadProfile(anchorDir, path string) (*Profile, error) {
	resolved, data, err := readContainedFile(anchorDir, path, "browser source profile", maxBrowserProfileBytes)
	if err != nil {
		return nil, err
	}
	if err := schemas.ValidateBrowserSourceProfile(data); err != nil {
		return nil, fmt.Errorf("validate browser source profile %s: %w", resolved, err)
	}
	var wire profileWire
	if err := yaml.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("decode browser source profile %s: %w", resolved, err)
	}
	return &Profile{
		Path:               resolved,
		Digest:             digest.SHA256String(data),
		Version:            strings.TrimSpace(wire.Profile),
		LoginStateRequired: wire.Info.LoginStateRequired,
		Contexts:           wire.Contexts,
		Actions:            wire.Actions,
	}, nil
}

// LoadAuthenticationProfile validates and decodes a contained browser
// authentication profile. Credential values are never accepted by this type.
func LoadAuthenticationProfile(anchorDir, path string) (*AuthenticationProfile, error) {
	resolved, data, err := readContainedFile(anchorDir, path, "browser authentication profile", maxBrowserProfileBytes)
	if err != nil {
		return nil, err
	}
	if err := schemas.ValidateBrowserAuthenticationProfile(data); err != nil {
		return nil, fmt.Errorf("validate browser authentication profile %s: %w", resolved, err)
	}
	var profile browserauthentication.Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("decode browser authentication profile %s: %w", resolved, err)
	}
	return &AuthenticationProfile{Path: resolved, Digest: digest.SHA256String(data), Profile: profile}, nil
}

func readContainedFile(anchorDir, path, label string, limit int64) (string, []byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil, fmt.Errorf("%s path is required", label)
	}
	if strings.Contains(path, "://") || strings.HasPrefix(strings.ToLower(path), "urn:") {
		return "", nil, fmt.Errorf("%s path must be package-relative", label)
	}
	anchor, err := filepath.Abs(anchorDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve %s anchor: %w", label, err)
	}
	canonicalAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		return "", nil, fmt.Errorf("resolve %s anchor: %w", label, err)
	}
	rel := filepath.FromSlash(path)
	if !filepath.IsAbs(rel) {
		cwdRelative, absErr := filepath.Abs(rel)
		if absErr == nil {
			if fromAnchor, relErr := filepath.Rel(anchor, cwdRelative); relErr == nil && (fromAnchor == "." || (fromAnchor != ".." && !strings.HasPrefix(fromAnchor, ".."+string(filepath.Separator)))) {
				rel = fromAnchor
			}
		}
	}
	if filepath.IsAbs(rel) {
		rel, err = filepath.Rel(anchor, filepath.Clean(rel))
		if err != nil {
			return "", nil, fmt.Errorf("resolve %s path: %w", label, err)
		}
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("%s path must remain within the project package", label)
	}
	ext := strings.ToLower(filepath.Ext(rel))
	if ext != ".json" && ext != ".yaml" && ext != ".yml" {
		return "", nil, fmt.Errorf("%s path has unsupported extension %q", label, ext)
	}
	current := canonicalAnchor
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return "", nil, fmt.Errorf("load %s %s: %w", label, current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("%s path must not traverse symlinks", label)
		}
	}
	info, err := os.Stat(current)
	if err != nil {
		return "", nil, fmt.Errorf("load %s %s: %w", label, current, err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%s path must name a regular file", label)
	}
	if info.Size() > limit {
		return "", nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	data, err := os.ReadFile(current)
	if err != nil {
		return "", nil, fmt.Errorf("load %s %s: %w", label, current, err)
	}
	if int64(len(data)) > limit {
		return "", nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return current, data, nil
}
