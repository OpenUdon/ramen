//go:build !udon

package main

import (
	"fmt"

	"github.com/OpenUdon/ramen/executor"
)

func newUdonExecutor(_ string) (executor.Executor, error) {
	return nil, fmt.Errorf("udon executor requires a ramen build with -tags udon")
}
