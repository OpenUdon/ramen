package browsercontract

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/uws/browserauthentication"
	"github.com/OpenUdon/uws/uws1"
)

func TestValidateRoleClassifiesSymbolicExternalSession(t *testing.T) {
	doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
	contract, err := ValidateRole(doc, resource, "read", role, source, profile)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Session != "member_portal" || !contract.ExternalSession || contract.Authentication != nil {
		t.Fatalf("contract = %#v", contract)
	}
	if contract.Action.Outputs["status"].Type != "integer" || contract.Profile.Contexts["detail_frame"].Kind != "frame" {
		t.Fatalf("browser projection = %#v", contract)
	}
}

func TestValidateRoleAcceptsExactAuthenticationContract(t *testing.T) {
	for _, test := range []struct {
		callProfile string
		authProfile string
	}{
		{callProfile: browserauthentication.CallProfileName, authProfile: browserauthentication.ProfileName},
		{callProfile: browserauthentication.ContextCallProfileName, authProfile: browserauthentication.ContextProfileName},
	} {
		t.Run(test.callProfile, func(t *testing.T) {
			doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
			authPath := filepath.Join(doc.Dir, "authentication.yaml")
			writeBrowserContractTestFile(t, authPath, authenticationProfileFixture(test.authProfile))
			authOperation := authenticationOperation(t, test.callProfile, "authentication.yaml", map[string]string{"username": "member_username"})
			doc.UWS.Operations = append([]*uws1.Operation{authOperation}, doc.UWS.Operations...)
			doc.UWS.Operations[1].DependsOn = []string{authOperation.OperationID}
			role.CredentialBindings = []string{"member_username"}

			contract, err := ValidateRole(doc, resource, "read", role, source, profile)
			if err != nil {
				t.Fatal(err)
			}
			if contract.ExternalSession || contract.Authentication == nil || contract.Authentication.Profile.Profile.Profile != test.authProfile {
				t.Fatalf("contract = %#v", contract)
			}
			if len(contract.Authentication.UsedSlots) != 1 || contract.Authentication.UsedSlots[0] != "username" {
				t.Fatalf("used slots = %#v", contract.Authentication.UsedSlots)
			}
		})
	}
}

func TestValidateRoleKeepsDistinctConfigurationsForSameBrowserAction(t *testing.T) {
	doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
	doc.UWS.Operations[0].Request = map[string]any{"body": map[string]any{"item": "first"}}
	second := &uws1.Operation{
		OperationID: "read_status_second", SourceDescription: "browser", SourceOperationID: "read_status",
		Request: map[string]any{"body": map[string]any{"item": "second"}},
	}
	if err := browserauthentication.SetSessionExtension(&second.Extensions, &browserauthentication.OperationSession{Session: "other_portal"}); err != nil {
		t.Fatal(err)
	}
	doc.UWS.Operations = append(doc.UWS.Operations, second)

	firstContract, err := ValidateRole(doc, resource, "read", role, source, profile)
	if err != nil {
		t.Fatal(err)
	}
	role.UWSOperationRef = second.OperationID
	secondContract, err := ValidateRole(doc, resource, "read", role, source, profile)
	if err != nil {
		t.Fatal(err)
	}
	first, secondProjection := NewProjection(firstContract), NewProjection(secondContract)
	if first.ActionID != secondProjection.ActionID || first.UWSOperationID == secondProjection.UWSOperationID || first.Session == secondProjection.Session {
		t.Fatalf("distinct operation configurations collapsed: first=%#v second=%#v", first, secondProjection)
	}
	firstItem := first.Request["body"].(map[string]any)["item"]
	secondItem := secondProjection.Request["body"].(map[string]any)["item"]
	if firstItem != "first" || secondItem != "second" {
		t.Fatalf("requests first=%#v second=%#v", first.Request, secondProjection.Request)
	}
}

func TestValidateRoleRejectsCrossDocumentMismatches(t *testing.T) {
	t.Run("profile version floor", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.8.0", true)
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "requires UWS 1.9.0")
	})
	t.Run("login session", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.6", "1.8.0", true)
		doc.UWS.Operations[0].Extensions = nil
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "requires x-uws-browser-session")
	})
	t.Run("source operation", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.5", "1.9.0", false)
		doc.UWS.Operations[0].SourceOperationID = "another_action"
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "not role operation")
	})
	t.Run("unrelated dependency", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
		doc.UWS.Operations = append(doc.UWS.Operations, &uws1.Operation{OperationID: "prepare", Extensions: map[string]any{uws1.ExtensionOperationProfile: "example.prepare.1.0"}})
		doc.UWS.Operations[0].DependsOn = []string{"prepare"}
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "unrelated dependency")
	})
	t.Run("exact credential slots", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
		writeBrowserContractTestFile(t, filepath.Join(doc.Dir, "authentication.yaml"), authenticationProfileFixture(browserauthentication.ContextProfileName))
		authOperation := authenticationOperation(t, browserauthentication.ContextCallProfileName, "authentication.yaml", map[string]string{})
		doc.UWS.Operations = append([]*uws1.Operation{authOperation}, doc.UWS.Operations...)
		doc.UWS.Operations[1].DependsOn = []string{authOperation.OperationID}
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "exactly match used slots")
	})
	t.Run("authentication profile pair", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
		writeBrowserContractTestFile(t, filepath.Join(doc.Dir, "authentication.yaml"), authenticationProfileFixture(browserauthentication.ContextProfileName))
		authOperation := authenticationOperation(t, browserauthentication.CallProfileName, "authentication.yaml", map[string]string{"username": "member_username"})
		doc.UWS.Operations = append([]*uws1.Operation{authOperation}, doc.UWS.Operations...)
		doc.UWS.Operations[1].DependsOn = []string{authOperation.OperationID}
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "requires authentication profile")
	})
	t.Run("authentication timeout", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
		writeBrowserContractTestFile(t, filepath.Join(doc.Dir, "authentication.yaml"), authenticationProfileFixture(browserauthentication.ProfileName))
		authOperation := authenticationOperation(t, browserauthentication.CallProfileName, "authentication.yaml", map[string]string{"username": "member_username"})
		tooLong := 601.0
		authOperation.Timeout = &tooLong
		doc.UWS.Operations = append([]*uws1.Operation{authOperation}, doc.UWS.Operations...)
		doc.UWS.Operations[1].DependsOn = []string{authOperation.OperationID}
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "at most 600")
	})
	t.Run("authentication session", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
		writeBrowserContractTestFile(t, filepath.Join(doc.Dir, "authentication.yaml"), authenticationProfileFixture(browserauthentication.ProfileName))
		authOperation := authenticationOperation(t, browserauthentication.CallProfileName, "authentication.yaml", map[string]string{"username": "member_username"})
		call, _, err := browserauthentication.ReadAuthenticationExtension(authOperation.Extensions)
		if err != nil {
			t.Fatal(err)
		}
		call.Session = "another_session"
		if err := browserauthentication.SetAuthenticationExtension(&authOperation.Extensions, call); err != nil {
			t.Fatal(err)
		}
		doc.UWS.Operations = append([]*uws1.Operation{authOperation}, doc.UWS.Operations...)
		doc.UWS.Operations[1].DependsOn = []string{authOperation.OperationID}
		_, err = ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "session does not match")
	})
	t.Run("symbolic binding declaration", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.7", "1.9.0", true)
		writeBrowserContractTestFile(t, filepath.Join(doc.Dir, "authentication.yaml"), authenticationProfileFixture(browserauthentication.ContextProfileName))
		authOperation := authenticationOperation(t, browserauthentication.ContextCallProfileName, "authentication.yaml", map[string]string{"username": "undeclared_username"})
		doc.UWS.Operations = append([]*uws1.Operation{authOperation}, doc.UWS.Operations...)
		doc.UWS.Operations[1].DependsOn = []string{authOperation.OperationID}
		_, err := ValidateRole(doc, resource, "read", role, source, profile)
		assertBrowserContractError(t, err, "not declared")
	})
	t.Run("mutating role", func(t *testing.T) {
		doc, resource, role, source, profile := browserRoleFixture(t, "uws.browser.1.5", "1.9.0", false)
		_, err := ValidateRole(doc, resource, "update", role, source, profile)
		assertBrowserContractError(t, err, "must not select a read_only")
	})
}

func browserRoleFixture(t *testing.T, browserVersion, uwsVersion string, loginRequired bool) (*project.Document, project.Resource, project.OperationRole, project.APISource, *Profile) {
	t.Helper()
	root := t.TempDir()
	profilePath := filepath.Join(root, "browser.yaml")
	outputType := "string"
	if browserVersion == "uws.browser.1.7" {
		outputType = "integer"
	}
	writeBrowserContractTestFile(t, profilePath, browserProfileFixture(browserVersion, loginRequired, outputType))
	profile, err := LoadProfile(root, profilePath)
	if err != nil {
		t.Fatal(err)
	}
	browserOperation := &uws1.Operation{
		OperationID:       "read_status_uws",
		SourceDescription: "browser",
		SourceOperationID: "read_status",
	}
	if loginRequired {
		if err := browserauthentication.SetSessionExtension(&browserOperation.Extensions, &browserauthentication.OperationSession{Session: "member_portal"}); err != nil {
			t.Fatal(err)
		}
	}
	doc := &project.Document{
		Dir: root,
		UWS: &uws1.Document{
			UWS:        uwsVersion,
			Operations: []*uws1.Operation{browserOperation},
			SourceDescriptions: []*uws1.SourceDescription{{
				Name: "browser", URL: "browser.yaml", Type: uws1.SourceDescriptionTypeBrowserProfile,
			}},
		},
	}
	resource := project.Resource{Address: "example.browser", CredentialBindings: []string{"member_username"}}
	role := project.OperationRole{SourceKind: "browser-profile", SourceID: "browser", OperationID: "read_status", UWSOperationRef: "read_status_uws"}
	source := project.APISource{Kind: "browser-profile", ID: "browser", Path: profilePath}
	return doc, resource, role, source, profile
}

func authenticationOperation(t *testing.T, callProfile, profilePath string, bindings map[string]string) *uws1.Operation {
	t.Helper()
	timeout := 120.0
	operation := &uws1.Operation{
		OperationID:              "authenticate",
		OperationExecutionFields: uws1.OperationExecutionFields{Timeout: &timeout},
		Extensions:               map[string]any{uws1.ExtensionOperationProfile: callProfile},
	}
	if err := browserauthentication.SetAuthenticationExtension(&operation.Extensions, &browserauthentication.OperationAuthentication{
		Profile: profilePath, Flow: "login", Session: "member_portal", CredentialBindings: bindings,
	}); err != nil {
		t.Fatal(err)
	}
	return operation
}

func assertBrowserContractError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}
