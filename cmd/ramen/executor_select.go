package main

import (
	"fmt"
	"strings"

	"github.com/OpenUdon/ramen/executor"
)

func selectTrustedExecutor(mode string, mock bool, udonOutputDir string) (executor.Executor, error) {
	mode = strings.TrimSpace(mode)
	if mock {
		if mode != "" && mode != "mock" {
			return nil, fmt.Errorf("--mock cannot be combined with --executor %s", mode)
		}
		mode = "mock"
	}
	switch mode {
	case "":
		return nil, nil
	case "mock":
		return &executor.MockExecutor{}, nil
	case "udon":
		return newUdonExecutor(udonOutputDir)
	default:
		return nil, fmt.Errorf("unsupported executor %q; expected mock or udon", mode)
	}
}
