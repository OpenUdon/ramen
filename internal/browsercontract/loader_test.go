package browsercontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProfileSupportsCurrentBrowserRevisions(t *testing.T) {
	for _, test := range []struct {
		version    string
		outputType string
	}{
		{version: "uws.browser.1.5", outputType: "string"},
		{version: "uws.browser.1.6", outputType: "string"},
		{version: "uws.browser.1.7", outputType: "integer"},
	} {
		t.Run(test.version, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "profiles", "browser.yaml")
			writeBrowserContractTestFile(t, path, browserProfileFixture(test.version, false, test.outputType))
			profile, err := LoadProfile(root, path)
			if err != nil {
				t.Fatal(err)
			}
			if profile.Version != test.version || profile.Digest == "" || profile.Actions["read_status"].Outputs["status"].Type != test.outputType {
				t.Fatalf("profile = %#v", profile)
			}
			if versionHasContexts(test.version) && profile.Contexts["detail_frame"].Kind != "frame" {
				t.Fatalf("contexts = %#v", profile.Contexts)
			}
			if test.version == "uws.browser.1.7" {
				outputs := profile.Actions["read_status"].Outputs
				if outputs["ratio"].Type != "number" || outputs["enabled"].Type != "boolean" || !outputs["goal_present"].Presence {
					t.Fatalf("scalar outputs = %#v", outputs)
				}
			}
		})
	}
}

func TestLoadAuthenticationProfileSupportsCurrentRevisions(t *testing.T) {
	for _, version := range []string{"uws.browser-authentication.1.0", "uws.browser-authentication.1.1"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "authentication.yaml")
			writeBrowserContractTestFile(t, path, authenticationProfileFixture(version))
			profile, err := LoadAuthenticationProfile(root, "authentication.yaml")
			if err != nil {
				t.Fatal(err)
			}
			credentialStep := 0
			if version == "uws.browser-authentication.1.1" {
				credentialStep = 1
			}
			flow := profile.Profile.Flows["login"]
			if profile.Profile.Profile != version || profile.Digest == "" || len(flow.Sequence) <= credentialStep || flow.Sequence[credentialStep].TypeCredential == nil || flow.Sequence[credentialStep].TypeCredential.Slot != "username" {
				t.Fatalf("profile = %#v", profile)
			}
			if version == "uws.browser-authentication.1.1" && profile.Profile.Contexts["idp_popup"].Kind != "popup" {
				t.Fatalf("contexts = %#v", profile.Profile.Contexts)
			}
		})
	}
}

func TestLoadProfileAcceptsProjectNormalizedPathFromRelativeProjectDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "browser.yaml")
	writeBrowserContractTestFile(t, path, browserProfileFixture("uws.browser.1.7", false, "integer"))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeRoot, err := filepath.Rel(cwd, root)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := LoadProfile(relativeRoot, filepath.Join(relativeRoot, "browser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.Path != path || profile.Version != "uws.browser.1.7" {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestLoadProfileRejectsUnsafeAndInvalidFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	writeBrowserContractTestFile(t, outside, browserProfileFixture("uws.browser.1.5", false, "string"))
	textPath := filepath.Join(root, "profile.txt")
	writeBrowserContractTestFile(t, textPath, "not a profile")
	invalidPath := filepath.Join(root, "invalid.yaml")
	writeBrowserContractTestFile(t, invalidPath, "profile: uws.browser.1.7\n")
	unknownPath := filepath.Join(root, "unknown.yaml")
	writeBrowserContractTestFile(t, unknownPath, browserProfileFixture("uws.browser.9.9", false, "string"))
	symlinkPath := filepath.Join(root, "linked.yaml")
	if err := os.Symlink(outside, symlinkPath); err != nil {
		t.Fatal(err)
	}
	largePath := filepath.Join(root, "large.yaml")
	writeBrowserContractTestFile(t, largePath, "")
	if err := os.Truncate(largePath, maxBrowserProfileBytes+1); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "remote", path: "https://example.test/profile.yaml", want: "package-relative"},
		{name: "outside", path: outside, want: "within the project package"},
		{name: "unsupported extension", path: textPath, want: "unsupported extension"},
		{name: "invalid schema", path: invalidPath, want: "validate browser source profile"},
		{name: "unknown profile", path: unknownPath, want: "unsupported browser source profile"},
		{name: "symlink", path: symlinkPath, want: "must not traverse symlinks"},
		{name: "oversize", path: largePath, want: "exceeds"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadProfile(root, test.path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func browserProfileFixture(version string, loginRequired bool, outputType string) string {
	contexts := ""
	contextSteps := ""
	extraOutputs := ""
	if versionHasContexts(version) {
		contexts = `contexts:
  statement_popup:
    kind: popup
    parent: main
    origin: https://example.test
  detail_frame:
    kind: frame
    parent: statement_popup
    origin: https://example.test
    path: /embedded/detail
    name: Detail
`
		contextSteps = `      - click:
          locator: {role: link, name: Open detail}
          context: main
          opensContext: statement_popup
      - wait_for:
          locator: {role: status, name: Ready}
          context: detail_frame
`
	}
	if version == "uws.browser.1.7" {
		extraOutputs = `      ratio:
        type: number
        source: a11y
        locator: {role: status, name: Ratio}
      enabled:
        type: boolean
        source: a11y
        locator: {role: status, name: Enabled}
      goal_present:
        type: boolean
        source: a11y
        locator: {role: heading, name: Goal}
        presence: true
`
	}
	return fmt.Sprintf(`profile: %s
info:
  title: Reviewed status
  origin: https://example.test
  loginStateRequired: %t
observationKind: accessibility_snapshot
evidence:
  learnedAt: "2026-08-20T00:00:00Z"
  source: reviewed_synthetic_fixture
confidence: high
expiresAfter: P30D
verification:
  lastVerifiedAt: "2026-08-20T00:00:00Z"
  successfulRuns: 1
%sactions:
  read_status:
    parameters:
      type: object
      properties:
        item: {type: string}
    sequence:
      - navigate: /status
%s    outputs:
      status:
        type: %s
        source: a11y
        locator: {role: status, name: Ready}
%s    sideEffects: [read_only]
    confirmationPolicy: {required: false}
`, version, loginRequired, contexts, contextSteps, outputType, extraOutputs)
}

func versionHasContexts(version string) bool {
	return version == "uws.browser.1.6" || version == "uws.browser.1.7"
}

func authenticationProfileFixture(version string) string {
	contexts := ""
	openingStep := ""
	credentialContext := ""
	if version == "uws.browser-authentication.1.1" {
		contexts = `contexts:
  idp_popup:
    kind: popup
    parent: main
    origin: https://login.example.test
`
		openingStep = `      - click:
          locator: {role: button, name: Continue}
          opensContext: idp_popup
`
		credentialContext = "          context: idp_popup\n"
	}
	return fmt.Sprintf(`profile: %s
info:
  title: Reviewed login
  applicationOrigins: [https://example.test]
  authenticationOrigins: [https://login.example.test]
observationKind: accessibility_snapshot
evidence:
  learnedAt: "2026-08-20T00:00:00Z"
  source: reviewed_synthetic_fixture
confidence: high
expiresAfter: P30D
verification:
  lastVerifiedAt: "2026-08-20T00:00:00Z"
  successfulRuns: 1
%scredentialSlots:
  username: {kind: identifier}
flows:
  login:
    sequence:
%s      - type_credential:
          locator: {role: textbox, name: Username}
          slot: username
%s      - wait_for:
          locator: {role: heading, name: Dashboard}
    effects: [establishes_session]
    success:
      origin: https://example.test
      locator: {role: heading, name: Dashboard}
`, version, contexts, openingStep, credentialContext)
}

func writeBrowserContractTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
