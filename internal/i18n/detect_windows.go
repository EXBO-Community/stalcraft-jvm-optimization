//go:build windows

package i18n

import (
	"syscall"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

var procGetUserDefaultLocaleName = winapi.Kernel32.NewProc("GetUserDefaultLocaleName")

func systemLocale() string {
	const localeNameMaxLength = 85
	buf := make([]uint16, localeNameMaxLength)
	ret, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
