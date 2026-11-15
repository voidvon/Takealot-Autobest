//go:build !windows

package localstate

import "os"

func replace(source, target string) error { return os.Rename(source, target) }
