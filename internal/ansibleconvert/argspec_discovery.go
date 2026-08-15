package ansibleconvert

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxArgspecDirectories = 16
	maxDiscoveredArgspecs = 256
)

// DiscoverArgspecs recursively discovers Ramen-owned argspec documents from
// local directories. It reads no collection code and performs no network or
// executable discovery.
func DiscoverArgspecs(directories []string) ([]ArgspecInput, error) {
	if len(directories) == 0 {
		return nil, nil
	}
	if len(directories) > maxArgspecDirectories {
		return nil, fmt.Errorf("argspec directory count %d exceeds the limit of %d", len(directories), maxArgspecDirectories)
	}
	var paths []string
	for _, root := range directories {
		label := filepath.Base(filepath.Clean(root))
		info, err := os.Lstat(root)
		if err != nil {
			return nil, fmt.Errorf("argspec directory %q could not be inspected: %s", label, inventoryIOReason(err))
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("argspec directory %q must be a directory and not a symlink", label)
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("could not inspect an entry under %q", label)
			}
			if path == root {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("argspec directory %q contains symlink %q", label, entry.Name())
			}
			if entry.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(entry.Name()), ".argspec.json") {
				return nil
			}
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("argspec candidate %q is not a regular file", entry.Name())
			}
			if info.Size() > maxArgspecBytes {
				return fmt.Errorf("argspec candidate %q exceeds the %d-byte limit", entry.Name(), maxArgspecBytes)
			}
			paths = append(paths, filepath.Clean(path))
			if len(paths) > maxDiscoveredArgspecs {
				return fmt.Errorf("discovered argspec count exceeds the limit of %d", maxDiscoveredArgspecs)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no *.argspec.json documents were found in the supplied argspec directories")
	}
	sort.Strings(paths)
	inputs := make([]ArgspecInput, 0, len(paths))
	for _, path := range paths {
		data, err := readArgspecData(path)
		if err != nil {
			return nil, fmt.Errorf("discovered argspec %q: %w", filepath.Base(path), err)
		}
		if err := ValidateArgspecDocument(data); err != nil {
			return nil, fmt.Errorf("discovered argspec %q: schema validation failed: %w", filepath.Base(path), err)
		}
		var doc argspecDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("discovered argspec %q: invalid JSON", filepath.Base(path))
		}
		inputs = append(inputs, ArgspecInput{ID: doc.Collection, Path: path})
	}
	return inputs, nil
}
