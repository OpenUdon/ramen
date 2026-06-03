package corpus

import (
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNoForbiddenProviderRuntimeImports(t *testing.T) {
	forbiddenPrefixes := []string{
		"github.com/hashicorp/terraform-provider-",
		"github.com/hashicorp/terraform-plugin-",
		"github.com/hashicorp/terraform/",
		"github.com/hashicorp/terraform-exec",
		"github.com/opentofu/opentofu",
		"github.com/OpenUdon/openudon",
		"github.com/OpenUdon/tfconfig/_upstream/",
	}
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			for _, prefix := range forbiddenPrefixes {
				if strings.HasPrefix(importPath, prefix) {
					t.Errorf("%s imports forbidden provider/runtime package %q", path, importPath)
				}
			}
			if strings.HasPrefix(importPath, "github.com/genelet/udon/") && !hasGoBuildTag(path, "udon") {
				t.Errorf("%s imports private udon package %q without //go:build udon", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan imports: %v", err)
	}
}

func hasGoBuildTag(path, tag string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			if strings.HasPrefix(line, "//go:build") {
				expr, err := constraint.Parse(line)
				if err != nil {
					return false
				}
				return expr.Eval(func(name string) bool { return name == tag })
			}
			continue
		}
		return false
	}
	return false
}
