package browsercontract

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/OpenUdon/ramen/project"
	"github.com/OpenUdon/uws/browserauthentication"
	"github.com/OpenUdon/uws/schemas"
	"github.com/OpenUdon/uws/uws1"
)

// Authentication describes the validated in-workflow authentication
// dependency of one browser action.
type Authentication struct {
	Operation          *uws1.Operation
	CallProfile        string
	ProfileRef         string
	Profile            *AuthenticationProfile
	Flow               string
	Session            string
	CredentialBindings map[string]string
	UsedSlots          []string
}

// Role is the validated cross-document browser contract selected by a Ramen
// operation role.
type Role struct {
	Profile         *Profile
	ProfileRef      string
	ActionID        string
	Action          Action
	Operation       *uws1.Operation
	Session         string
	ExternalSession bool
	Authentication  *Authentication
}

// ValidateRole closes the Ramen/UWS/browser cross-document references for one
// desired-state operation role without executing or mutating anything.
func ValidateRole(doc *project.Document, resource project.Resource, purpose string, operationRole project.OperationRole, source project.APISource, profile *Profile) (*Role, error) {
	if doc == nil || doc.UWS == nil || profile == nil {
		return nil, fmt.Errorf("browser contract validation requires a loaded project and profile")
	}
	operation, err := operationByID(doc.UWS, strings.TrimSpace(operationRole.UWSOperationRef))
	if err != nil {
		return nil, err
	}
	sourceDescription, err := sourceDescriptionByName(doc.UWS, operation.SourceDescription)
	if err != nil {
		return nil, err
	}
	if sourceDescription.EffectiveType() != uws1.SourceDescriptionTypeBrowserProfile {
		return nil, fmt.Errorf("UWS operation %s source %s is not browser-profile", operation.OperationID, sourceDescription.Name)
	}
	if sourceDescription.Name != strings.TrimSpace(operationRole.SourceID) || sourceDescription.Name != strings.TrimSpace(source.ID) {
		return nil, fmt.Errorf("UWS operation %s source %s does not match Ramen source %s", operation.OperationID, sourceDescription.Name, operationRole.SourceID)
	}
	resolvedSource, err := resolvePackageReference(doc.Dir, sourceDescription.URL)
	if err != nil {
		return nil, fmt.Errorf("UWS source description %s: %w", sourceDescription.Name, err)
	}
	if !samePath(resolvedSource, profile.Path) || !samePath(resolvedSource, source.Path) {
		return nil, fmt.Errorf("UWS source description %s path does not match Ramen browser source path", sourceDescription.Name)
	}
	if strings.TrimSpace(operationRole.SourcePath) != "" && !samePath(resolvedSource, operationRole.SourcePath) {
		return nil, fmt.Errorf("UWS source description %s path does not match operation role source_path", sourceDescription.Name)
	}
	actionID, err := selectedActionID(operation)
	if err != nil {
		return nil, err
	}
	if actionID != strings.TrimSpace(operationRole.OperationID) {
		return nil, fmt.Errorf("UWS operation %s selects browser action %s, not role operation %s", operation.OperationID, actionID, operationRole.OperationID)
	}
	action, ok := profile.Actions[actionID]
	if !ok {
		return nil, fmt.Errorf("browser action %s was not found", actionID)
	}
	if err := requireProfileVersion(doc.UWS.UWS, profile.Version); err != nil {
		return nil, err
	}
	sessionExtension, hasSession, err := browserauthentication.ReadSessionExtension(operation.Extensions)
	if err != nil {
		return nil, err
	}
	session := ""
	if hasSession {
		session = strings.TrimSpace(sessionExtension.Session)
	}
	if profile.LoginStateRequired && session == "" {
		return nil, fmt.Errorf("browser profile %s requires x-uws-browser-session", profile.Version)
	}
	result := &Role{Profile: profile, ProfileRef: sourceDescription.URL, ActionID: actionID, Action: action, Operation: operation, Session: session}
	auth, err := validateAuthenticationDependency(doc, resource, operationRole, operation, session)
	if err != nil {
		return nil, err
	}
	result.Authentication = auth
	result.ExternalSession = session != "" && auth == nil
	if err := validateSideEffects(purpose, action.SideEffects); err != nil {
		return nil, err
	}
	return result, nil
}

func validateAuthenticationDependency(doc *project.Document, resource project.Resource, role project.OperationRole, operation *uws1.Operation, session string) (*Authentication, error) {
	var authOperation *uws1.Operation
	for _, dependency := range operation.DependsOn {
		candidate, err := operationByID(doc.UWS, strings.TrimSpace(dependency))
		if err != nil {
			return nil, fmt.Errorf("browser operation %s dependency %s is not a top-level UWS operation", operation.OperationID, dependency)
		}
		callProfile := candidate.ExtensionProfile()
		if callProfile != browserauthentication.CallProfileName && callProfile != browserauthentication.ContextCallProfileName {
			return nil, fmt.Errorf("browser operation %s has unrelated dependency %s", operation.OperationID, dependency)
		}
		if authOperation != nil {
			return nil, fmt.Errorf("browser operation %s has multiple authentication dependencies", operation.OperationID)
		}
		authOperation = candidate
	}
	if authOperation == nil {
		return nil, nil
	}
	if session == "" {
		return nil, fmt.Errorf("browser operation %s authentication dependency requires x-uws-browser-session", operation.OperationID)
	}
	if !authOperation.IsExtensionOwned() {
		return nil, fmt.Errorf("authentication operation %s must be extension-owned", authOperation.OperationID)
	}
	callProfile := authOperation.ExtensionProfile()
	rawCall, ok := authOperation.Extensions[browserauthentication.ExtensionAuthentication]
	if !ok {
		return nil, fmt.Errorf("authentication operation %s requires %s", authOperation.OperationID, browserauthentication.ExtensionAuthentication)
	}
	envelope, err := json.Marshal(map[string]any{browserauthentication.ExtensionAuthentication: rawCall})
	if err != nil {
		return nil, err
	}
	if err := schemas.ValidateBrowserAuthenticationCallSupplementForProfile(envelope, callProfile); err != nil {
		return nil, fmt.Errorf("authentication operation %s: %w", authOperation.OperationID, err)
	}
	call, ok, err := browserauthentication.ReadAuthenticationExtension(authOperation.Extensions)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("authentication operation %s requires %s", authOperation.OperationID, browserauthentication.ExtensionAuthentication)
	}
	// The raw supplement validation above guarantees this typed read remains
	// lossless for the supported closed extension shape.
	if authOperation.Timeout == nil || *authOperation.Timeout <= 0 || *authOperation.Timeout > 600 {
		return nil, fmt.Errorf("authentication operation %s timeout must be greater than zero and at most 600 seconds", authOperation.OperationID)
	}
	if strings.TrimSpace(call.Session) != session {
		return nil, fmt.Errorf("authentication operation %s session does not match browser operation session", authOperation.OperationID)
	}
	authProfile, err := LoadAuthenticationProfile(doc.Dir, call.Profile)
	if err != nil {
		return nil, err
	}
	wantProfile := browserauthentication.ProfileName
	wantUWS := "1.7.0"
	if callProfile == browserauthentication.ContextCallProfileName {
		wantProfile = browserauthentication.ContextProfileName
		wantUWS = "1.8.0"
	}
	if authProfile.Profile.Profile != wantProfile {
		return nil, fmt.Errorf("authentication call profile %s requires authentication profile %s", callProfile, wantProfile)
	}
	if !versionAtLeast(doc.UWS.UWS, wantUWS) {
		return nil, fmt.Errorf("authentication call profile %s requires UWS %s or newer", callProfile, wantUWS)
	}
	flow, ok := authProfile.Profile.Flows[strings.TrimSpace(call.Flow)]
	if !ok {
		return nil, fmt.Errorf("authentication flow %s was not found", call.Flow)
	}
	usedSlots := usedCredentialSlots(flow)
	boundSlots := make([]string, 0, len(call.CredentialBindings))
	for slot := range call.CredentialBindings {
		boundSlots = append(boundSlots, slot)
	}
	slices.Sort(boundSlots)
	if !slices.Equal(usedSlots, boundSlots) {
		return nil, fmt.Errorf("authentication flow %s credential bindings must exactly match used slots", call.Flow)
	}
	declared := map[string]bool{}
	for _, binding := range resource.CredentialBindings {
		declared[strings.TrimSpace(binding)] = true
	}
	for _, binding := range role.CredentialBindings {
		declared[strings.TrimSpace(binding)] = true
	}
	for _, binding := range call.CredentialBindings {
		if !declared[strings.TrimSpace(binding)] {
			return nil, fmt.Errorf("authentication credential binding %s is not declared by the resource or operation role", binding)
		}
	}
	return &Authentication{Operation: authOperation, CallProfile: callProfile, ProfileRef: call.Profile, Profile: authProfile, Flow: call.Flow, Session: session, CredentialBindings: call.CredentialBindings, UsedSlots: usedSlots}, nil
}

// LoadProjectRole loads and validates the browser contract selected by one
// already-loaded native Ramen project role.
func LoadProjectRole(doc *project.Document, resource project.Resource, purpose string, role project.OperationRole) (*Role, error) {
	if doc == nil {
		return nil, fmt.Errorf("browser contract validation requires a loaded project")
	}
	source := project.SourceForRole(doc.Profile, role)
	if strings.TrimSpace(source.Kind) != string(uws1.SourceDescriptionTypeBrowserProfile) {
		return nil, fmt.Errorf("operation role source is not browser-profile")
	}
	profile, err := LoadProfile(doc.Dir, source.Path)
	if err != nil {
		return nil, err
	}
	return ValidateRole(doc, resource, purpose, role, source, profile)
}

func operationByID(doc *uws1.Document, id string) (*uws1.Operation, error) {
	if id == "" {
		return nil, fmt.Errorf("uws_operation_ref is required")
	}
	var matched *uws1.Operation
	for _, operation := range doc.Operations {
		if operation != nil && operation.OperationID == id {
			if matched != nil {
				return nil, fmt.Errorf("UWS operation reference %s is ambiguous", id)
			}
			matched = operation
		}
	}
	if matched == nil {
		return nil, fmt.Errorf("UWS operation reference %s was not found", id)
	}
	return matched, nil
}

func sourceDescriptionByName(doc *uws1.Document, name string) (*uws1.SourceDescription, error) {
	for _, source := range doc.SourceDescriptions {
		if source != nil && source.Name == name {
			return source, nil
		}
	}
	return nil, fmt.Errorf("UWS source description %s was not found", name)
}

func selectedActionID(operation *uws1.Operation) (string, error) {
	if id := strings.TrimSpace(operation.SourceOperationID); id != "" {
		return id, nil
	}
	ref := strings.TrimSpace(operation.SourceOperationRef)
	const prefix = "#/actions/"
	if !strings.HasPrefix(ref, prefix) || strings.Contains(strings.TrimPrefix(ref, prefix), "/") {
		return "", fmt.Errorf("UWS operation %s must select one browser action by sourceOperationId or #/actions/<id>", operation.OperationID)
	}
	id := strings.TrimPrefix(ref, prefix)
	id = strings.ReplaceAll(id, "~1", "/")
	id = strings.ReplaceAll(id, "~0", "~")
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("UWS operation %s has an empty browser action reference", operation.OperationID)
	}
	return id, nil
}

// SelectedActionID returns the browser action selected by a UWS operation.
func SelectedActionID(operation *uws1.Operation) (string, error) {
	return selectedActionID(operation)
}

func resolvePackageReference(anchorDir, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" || filepath.IsAbs(filepath.FromSlash(reference)) || strings.Contains(reference, "://") || strings.HasPrefix(strings.ToLower(reference), "urn:") {
		return "", fmt.Errorf("browser source URL must be a package-relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(reference))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("browser source URL must remain within the project package")
	}
	return filepath.Join(anchorDir, clean), nil
}

func samePath(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func requireProfileVersion(uwsVersion, profileVersion string) error {
	required := ""
	switch profileVersion {
	case "uws.browser.1.5":
		required = "1.5.0"
	case "uws.browser.1.6":
		required = "1.8.0"
	case "uws.browser.1.7":
		required = "1.9.0"
	default:
		return fmt.Errorf("unsupported browser profile %s", profileVersion)
	}
	if !versionAtLeast(uwsVersion, required) {
		return fmt.Errorf("browser profile %s requires UWS %s or newer", profileVersion, required)
	}
	return nil
}

// ValidateProfileVersion checks the UWS version floor for a browser profile.
func ValidateProfileVersion(uwsVersion, profileVersion string) error {
	return requireProfileVersion(uwsVersion, profileVersion)
}

func versionAtLeast(actual, required string) bool {
	parse := func(value string) [3]int {
		var out [3]int
		parts := strings.Split(strings.TrimSpace(value), ".")
		for i := 0; i < len(parts) && i < len(out); i++ {
			out[i], _ = strconv.Atoi(parts[i])
		}
		return out
	}
	left, right := parse(actual), parse(required)
	for i := range left {
		if left[i] != right[i] {
			return left[i] > right[i]
		}
	}
	return true
}

func usedCredentialSlots(flow browserauthentication.Flow) []string {
	seen := map[string]bool{}
	for _, step := range flow.Sequence {
		if step.TypeCredential != nil {
			seen[strings.TrimSpace(step.TypeCredential.Slot)] = true
		}
		if step.Challenge != nil && strings.TrimSpace(step.Challenge.Slot) != "" {
			seen[strings.TrimSpace(step.Challenge.Slot)] = true
		}
	}
	delete(seen, "")
	out := make([]string, 0, len(seen))
	for slot := range seen {
		out = append(out, slot)
	}
	slices.Sort(out)
	return out
}

func validateSideEffects(purpose string, effects []string) error {
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	readOnly := len(effects) == 1 && effects[0] == "read_only"
	if purpose == "read" && !readOnly {
		return fmt.Errorf("read operation role must select a read_only browser action")
	}
	mutating := map[string]bool{
		"create": true, "update": true, "delete": true, "replace": true,
		"post": true, "put": true, "patch": true, "import": true,
		"suspend": true, "detach": true, "disable": true, "remove_config": true,
	}
	if mutating[purpose] && readOnly {
		return fmt.Errorf("mutating operation role %s must not select a read_only browser action", purpose)
	}
	return nil
}
