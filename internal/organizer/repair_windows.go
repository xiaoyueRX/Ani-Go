package organizer

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const repairReadFlags = 0

var reopenRepairSource = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

func linkRepairSource(source *os.File, _ string, _ *os.Root, parent *os.File, name string, validate func() error) error {
	// 与标准库 os.Root.Link 使用相同的 NT FileLinkInformation；源、目标目录均由句柄固定。
	// ReOpenFile 根据原句柄追加链接所需属性权限，不会再次解析可能已变化的下载路径。
	raw, _, openErr := reopenRepairSource.Call(source.Fd(), windows.FILE_WRITE_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, 0)
	h := windows.Handle(raw)
	if h == windows.InvalidHandle {
		return openErr
	}
	defer windows.CloseHandle(h)
	var info struct {
		ReplaceIfExists bool
		RootDirectory   windows.Handle
		FileNameLength  uint32
		FileName        [260]uint16
	}
	utf16, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	if len(utf16) > len(info.FileName) {
		return windows.ERROR_FILENAME_EXCED_RANGE
	}
	info.RootDirectory = windows.Handle(parent.Fd())
	info.FileNameLength = uint32(2 * (len(utf16) - 1))
	copy(info.FileName[:], utf16)
	if err := validate(); err != nil {
		return err
	}
	const fileLinkInformation = 11
	err = windows.NtSetInformationFile(h, &windows.IO_STATUS_BLOCK{}, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), fileLinkInformation)
	if st, ok := err.(windows.NTStatus); ok {
		err = st.Errno()
	}
	if errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) || errors.Is(err, windows.ERROR_INVALID_FUNCTION) || errors.Is(err, windows.ERROR_TOO_MANY_LINKS) {
		return fmt.Errorf("%w: %w", ErrLinkUnavailable, err)
	}
	return err
}
