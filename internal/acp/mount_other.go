//go:build !linux

package acp

import "os"

func readOnlyMount(_ *os.File) bool { return false }
