//go:build linux

package organizer

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestSharedAndUnknownMountsNeverReclaimStages(t *testing.T) {
	for _, kind := range []uint32{unix.NFS_SUPER_MAGIC, unix.CIFS_SUPER_MAGIC, unix.SMB2_SUPER_MAGIC, unix.FUSE_SUPER_MAGIC, 0} {
		if localStageLockSupported(kind) {
			t.Fatalf("shared/unknown mount %x treated as local", kind)
		}
	}
}
