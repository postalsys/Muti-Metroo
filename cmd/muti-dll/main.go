//go:build windows

// Package main provides a Windows DLL entry point for rundll32.exe execution.
//
// Usage:
//
//	rundll32.exe muti-metroo.dll,Run C:\path\to\config.yaml
//
// Or with embedded config:
//
//	rundll32.exe muti-metroo.dll,Run
//
// Note: On Windows ARM64, use the x64 emulation layer rundll32:
//
//	C:\Windows\SysWOW64\rundll32.exe muti-metroo.dll,Run C:\path\to\config.yaml
package main

/*
#include <windows.h>
*/
import "C"

import (
	"strings"
	"syscall"
	"unsafe"

	"github.com/postalsys/muti-metroo/internal/agent"
	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/sysinfo"
)

// Version is set at build time via ldflags.
var Version = "dev"

func init() {
	// Sync version with sysinfo for consistency across the codebase
	if Version == "dev" {
		Version = sysinfo.Version
	} else {
		sysinfo.Version = Version
	}
}

// getDLLPath returns the full path to this DLL.
// We use GetModuleHandleEx with GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS
// to get the module handle of the DLL containing this function.
func getDLLPath() string {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getModuleHandleEx := kernel32.NewProc("GetModuleHandleExW")
	getModuleFileName := kernel32.NewProc("GetModuleFileNameW")

	const GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS = 0x00000004

	var hModule uintptr

	// Get the module handle of the DLL containing this function
	// by passing the address of a variable in this DLL's memory space.
	// We use the address of the Version variable which is in our DLL.
	ret, _, _ := getModuleHandleEx.Call(
		GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS,
		uintptr(unsafe.Pointer(&Version)),
		uintptr(unsafe.Pointer(&hModule)),
	)

	if ret == 0 {
		return ""
	}

	// Buffer for the path (MAX_PATH is 260, but long paths can be up to 32767)
	buf := make([]uint16, 32768)

	ret, _, _ = getModuleFileName.Call(
		hModule,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)

	if ret == 0 {
		return ""
	}

	return syscall.UTF16ToString(buf[:ret])
}

// Run is the exported entry point for rundll32.exe.
// Signature matches Windows rundll32 callback convention:
//
//	void CALLBACK Run(HWND hwnd, HINSTANCE hinst, LPSTR lpszCmdLine, int nCmdShow)
//
//export Run
func Run(hwnd C.HWND, hinst C.HINSTANCE, lpszCmdLine *C.char, nCmdShow C.int) {
	// Parse command line - should be the config file path
	cmdLine := strings.TrimSpace(C.GoString(lpszCmdLine))

	// Get the DLL's own path for embedded config support
	dllPath := getDLLPath()

	// Load config (embedded takes precedence, then command line path)
	cfg, _, err := config.LoadOrEmbeddedFrom(dllPath, cmdLine)
	if err != nil || cfg == nil {
		return
	}

	// Create agent
	a, err := agent.New(cfg)
	if err != nil {
		return
	}

	// Start agent
	if err := a.Start(); err != nil {
		return
	}

	// Block forever - rundll32 process will be terminated externally
	// Signal handling doesn't work reliably in DLL context on Windows
	select {}
}

// main is required for c-shared buildmode but will not be called when loaded as DLL.
func main() {}
