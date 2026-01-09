//go:build windows
// +build windows

package config

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	dllCrypt32  = syscall.NewLazyDLL("crypt32.dll")
	dllKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCryptProtectData   = dllCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = dllCrypt32.NewProc("CryptUnprotectData")
	procLocalFree          = dllKernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

// EncryptDPAPI 使用Windows DPAPI加密数据
func EncryptDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("数据为空")
	}

	input := dataBlob{
		cbData: uint32(len(data)),
		pbData: &data[0],
	}
	var output dataBlob

	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0, // 描述
		0, // 熵
		0, // 保留
		0, // prompt
		0, // 标志
		uintptr(unsafe.Pointer(&output)),
	)

	if r == 0 {
		return nil, fmt.Errorf("DPAPI加密失败: %v", err)
	}

	defer procLocalFree.Call(uintptr(unsafe.Pointer(output.pbData)))

	result := make([]byte, output.cbData)
	copy(result, unsafe.Slice(output.pbData, output.cbData))

	return result, nil
}

// DecryptDPAPI 使用Windows DPAPI解密数据
func DecryptDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("数据为空")
	}

	input := dataBlob{
		cbData: uint32(len(data)),
		pbData: &data[0],
	}
	var output dataBlob

	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0, // 描述
		0, // 熵
		0, // 保留
		0, // prompt
		0, // 标志
		uintptr(unsafe.Pointer(&output)),
	)

	if r == 0 {
		return nil, fmt.Errorf("DPAPI解密失败: %v", err)
	}

	defer procLocalFree.Call(uintptr(unsafe.Pointer(output.pbData)))

	result := make([]byte, output.cbData)
	copy(result, unsafe.Slice(output.pbData, output.cbData))

	return result, nil
}
