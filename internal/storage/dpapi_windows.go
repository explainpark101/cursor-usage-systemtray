//go:build windows

package storage

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPI (CryptProtectData / CryptUnprotectData) wrappers. The encrypted
// blobs are scoped to the current Windows user.

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	dllCrypt32             = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData   = dllCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = dllCrypt32.NewProc("CryptUnprotectData")
	dllKernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procLocalFree          = dllKernel32.NewProc("LocalFree")
)

func encrypt(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, nil
	}
	in := dataBlob{cbData: uint32(len(plain)), pbData: &plain[0]}
	var out dataBlob

	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // description
		0, // optional entropy
		0, // reserved
		0, // prompt struct
		0, // flags
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))

	buf := make([]byte, out.cbData)
	copy(buf, unsafe.Slice(out.pbData, out.cbData))
	runtime.KeepAlive(plain)
	return buf, nil
}

func decrypt(enc []byte) ([]byte, error) {
	if len(enc) == 0 {
		return nil, nil
	}
	in := dataBlob{cbData: uint32(len(enc)), pbData: &enc[0]}
	var out dataBlob

	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))

	buf := make([]byte, out.cbData)
	copy(buf, unsafe.Slice(out.pbData, out.cbData))
	runtime.KeepAlive(enc)
	return buf, nil
}
