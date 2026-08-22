package browsercontract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRecheckProjectionDetectsChangedBrowserArtifacts(t *testing.T) {
	for _, changed := range []string{"browser", "authentication"} {
		t.Run(changed, func(t *testing.T) {
			root := t.TempDir()
			browserRef := filepath.Join("profiles", "browser.yaml")
			authRef := filepath.Join("authentication", "member.yaml")
			browserPath := filepath.Join(root, browserRef)
			authPath := filepath.Join(root, authRef)
			browserText := browserProfileFixture("uws.browser.1.7", true, "integer")
			authText := authenticationProfileFixture("uws.browser-authentication.1.1")
			writeBrowserContractTestFile(t, browserPath, browserText)
			writeBrowserContractTestFile(t, authPath, authText)

			browser, err := LoadProfile(root, browserRef)
			if err != nil {
				t.Fatal(err)
			}
			auth, err := LoadAuthenticationProfile(root, authRef)
			if err != nil {
				t.Fatal(err)
			}
			projection := &Projection{
				ProfileVersion: browser.Version,
				ProfileRef:     filepath.ToSlash(browserRef),
				ProfilePath:    browser.Path,
				ProfileDigest:  browser.Digest,
				Authentication: &AuthenticationProjection{
					ProfileVersion: auth.Profile.Profile,
					ProfileRef:     filepath.ToSlash(authRef),
					ProfilePath:    auth.Path,
					ProfileDigest:  auth.Digest,
				},
			}
			if err := RecheckProjection(projection); err != nil {
				t.Fatalf("unchanged projection: %v", err)
			}

			switch changed {
			case "browser":
				writeBrowserContractTestFile(t, browserPath, strings.Replace(browserText, "Reviewed status", "Changed status", 1))
			case "authentication":
				writeBrowserContractTestFile(t, authPath, strings.Replace(authText, "Reviewed login", "Changed login", 1))
			}
			if err := RecheckProjection(projection); err == nil || !strings.Contains(err.Error(), "no longer matches the approved plan") {
				t.Fatalf("changed %s artifact error = %v", changed, err)
			}
		})
	}
}
