//go:build !darwin && !linux && !windows

package organizer

import "os"

const repairReadFlags = 0

func linkRepairSource(*os.File, string, *os.Root, *os.File, string, func() error) error {
	return ErrLinkUnavailable
}
