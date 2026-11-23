//go:build !linux && !windows && !darwin

package organizer

import "os"

func publishNewFile(src, dst string) error {
	return os.Link(src, dst)
}
