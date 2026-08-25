package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain preserves the repository-root working directory used by this
// integration/evidence test package before it moved under tests/.
func TestMain(m *testing.M) {
	packageDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve tests package directory: %v\n", err)
		os.Exit(1)
	}
	if filepath.Base(packageDir) != "tests" {
		fmt.Fprintf(os.Stderr, "unexpected tests package directory %q\n", packageDir)
		os.Exit(1)
	}
	if err := os.Chdir(filepath.Dir(packageDir)); err != nil {
		fmt.Fprintf(os.Stderr, "change to repository root: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
