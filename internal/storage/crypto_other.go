//go:build !windows

package storage

// Non-Windows stubs so the package compiles for development; the shipped
// binary is windows-only and uses DPAPI.

func encrypt(plain []byte) ([]byte, error) { return plain, nil }
func decrypt(enc []byte) ([]byte, error)   { return enc, nil }
