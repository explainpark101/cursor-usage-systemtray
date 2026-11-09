//go:build windows

package w32

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comdlg32                 = windows.NewLazySystemDLL("comdlg32.dll")
	comdlg32GetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
)

const (
	ofnExplorer      = 0x00080000
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnHideReadOnly  = 0x00000004
	ofnNoChangeDir   = 0x00000008
)

// openFileNameW matches the Win32 OPENFILENAMEW layout.
type openFileNameW struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

// PickOpenFile shows a modal open-file dialog owned by hwnd.
// ok is false when the user cancels (not an error).
func PickOpenFile(hwnd uintptr, title string, filterPairs [][2]string, defExt string) (path string, ok bool) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", false
	}
	filterUTF16 := buildFileFilter(filterPairs)

	var defExtPtr *uint16
	if defExt != "" {
		defExtPtr, _ = windows.UTF16PtrFromString(defExt)
	}

	buf := make([]uint16, 32768)
	ofn := openFileNameW{
		LStructSize:  uint32(unsafe.Sizeof(openFileNameW{})),
		HwndOwner:    hwnd,
		LpstrFilter:  &filterUTF16[0],
		NFilterIndex: 1,
		LpstrFile:    &buf[0],
		NMaxFile:     uint32(len(buf)),
		LpstrTitle:   titlePtr,
		Flags:        ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnHideReadOnly | ofnNoChangeDir,
		LpstrDefExt:  defExtPtr,
	}

	r, _, _ := comdlg32GetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(buf), true
}

func buildFileFilter(pairs [][2]string) []uint16 {
	var out []uint16
	for _, p := range pairs {
		for _, part := range []string{p[0], p[1]} {
			u, err := windows.UTF16FromString(part)
			if err != nil {
				continue
			}
			out = append(out, u...) // includes trailing NUL separator
		}
	}
	out = append(out, 0) // final extra NUL
	if len(out) == 0 {
		u, _ := windows.UTF16FromString("All Files (*.*)")
		out = append(out, u...)
		u, _ = windows.UTF16FromString("*.*")
		out = append(out, u...)
		out = append(out, 0)
	}
	return out
}
