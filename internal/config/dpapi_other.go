//go:build !windows
// +build !windows

package config

import "fmt"

// EncryptDPAPI 非Windows平台不支持DPAPI
func EncryptDPAPI(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("DPAPI仅在Windows平台可用")
}

// DecryptDPAPI 非Windows平台不支持DPAPI
func DecryptDPAPI(data []byte) ([]byte, error) {
	return nil, fmt.Errorf("DPAPI仅在Windows平台可用")
}
