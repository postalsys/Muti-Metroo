//go:build windows

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

var (
	modAdvapi32            = windows.NewLazySystemDLL("advapi32.dll")
	procOpenSCManager      = modAdvapi32.NewProc("OpenSCManagerW")
	procCreateService      = modAdvapi32.NewProc("CreateServiceW")
	procOpenService        = modAdvapi32.NewProc("OpenServiceW")
	procDeleteService      = modAdvapi32.NewProc("DeleteService")
	procCloseServiceHandle = modAdvapi32.NewProc("CloseServiceHandle")
	procStartService       = modAdvapi32.NewProc("StartServiceW")
	procControlService     = modAdvapi32.NewProc("ControlService")
	procQueryServiceStatus = modAdvapi32.NewProc("QueryServiceStatus")
	procCheckTokenMembership = modAdvapi32.NewProc("CheckTokenMembership")
)

const (
	SC_MANAGER_ALL_ACCESS = 0xF003F
	SERVICE_ALL_ACCESS    = 0xF01FF
	SERVICE_WIN32_OWN_PROCESS = 0x10
	SERVICE_AUTO_START    = 0x2
	SERVICE_ERROR_NORMAL  = 0x1
	SERVICE_CONTROL_STOP  = 0x1
	SERVICE_STOPPED       = 0x1
	SERVICE_START_PENDING = 0x2
	SERVICE_STOP_PENDING  = 0x3
	SERVICE_RUNNING       = 0x4
)

type serviceStatus struct {
	serviceType             uint32
	currentState            uint32
	controlsAccepted        uint32
	win32ExitCode           uint32
	serviceSpecificExitCode uint32
	checkPoint              uint32
	waitHint                uint32
}

// isRootImpl checks if running as Administrator on Windows.
func isRootImpl() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	member, err := isTokenMemberOfSid(windows.Token(0), sid)
	if err != nil {
		return false
	}

	return member
}

func isTokenMemberOfSid(token windows.Token, sid *windows.SID) (bool, error) {
	var isMember int32
	r1, _, err := procCheckTokenMembership.Call(
		uintptr(token),
		uintptr(unsafe.Pointer(sid)),
		uintptr(unsafe.Pointer(&isMember)),
	)
	if r1 == 0 {
		return false, err
	}
	return isMember != 0, nil
}

// installImpl installs the service on Windows.
func installImpl(cfg ServiceConfig, execPath string) error {
	// Open Service Control Manager
	scManager, err := openSCManager()
	if err != nil {
		return fmt.Errorf("failed to open service control manager: %w", err)
	}
	defer closeSCHandle(scManager)

	// Check if already installed
	existingService, _ := openService(scManager, cfg.Name)
	if existingService != 0 {
		closeSCHandle(existingService)
		return fmt.Errorf("service %s is already installed", cfg.Name)
	}

	// Build command line
	cmdLine := fmt.Sprintf(`"%s" run -c "%s"`, execPath, cfg.ConfigPath)

	// Create the service
	namePtr, _ := syscall.UTF16PtrFromString(cfg.Name)
	displayNamePtr, _ := syscall.UTF16PtrFromString(cfg.DisplayName)
	cmdLinePtr, _ := syscall.UTF16PtrFromString(cmdLine)

	r1, _, err := procCreateService.Call(
		scManager,
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(displayNamePtr)),
		SERVICE_ALL_ACCESS,
		SERVICE_WIN32_OWN_PROCESS,
		SERVICE_AUTO_START,
		SERVICE_ERROR_NORMAL,
		uintptr(unsafe.Pointer(cmdLinePtr)),
		0, // No load order group
		0, // No tag
		0, // No dependencies
		0, // LocalSystem account
		0, // No password
	)
	if r1 == 0 {
		return fmt.Errorf("failed to create service: %w", err)
	}
	serviceHandle := r1
	defer closeSCHandle(serviceHandle)

	fmt.Printf("Created Windows service: %s\n", cfg.Name)

	// Set service description
	if cfg.Description != "" {
		setServiceDescription(serviceHandle, cfg.Description)
	}

	// Start the service
	r1, _, err = procStartService.Call(serviceHandle, 0, 0)
	if r1 == 0 {
		fmt.Printf("Note: service created but failed to start: %v\n", err)
		fmt.Println("You may need to start it manually with: net start", cfg.Name)
	} else {
		fmt.Printf("Started Windows service: %s\n", cfg.Name)
	}

	return nil
}

// installImplEmbedded installs Windows service for embedded config binary.
// Note: No -c flag since config is embedded in the binary.
func installImplEmbedded(cfg ServiceConfig, execPath string) error {
	// Open Service Control Manager
	scManager, err := openSCManager()
	if err != nil {
		return fmt.Errorf("failed to open service control manager: %w", err)
	}
	defer closeSCHandle(scManager)

	// Check if already installed
	existingService, _ := openService(scManager, cfg.Name)
	if existingService != 0 {
		closeSCHandle(existingService)
		return fmt.Errorf("service %s is already installed", cfg.Name)
	}

	// Build command line (no -c flag for embedded config)
	cmdLine := fmt.Sprintf(`"%s" run`, execPath)

	// Create the service
	namePtr, _ := syscall.UTF16PtrFromString(cfg.Name)
	displayNamePtr, _ := syscall.UTF16PtrFromString(cfg.DisplayName)
	cmdLinePtr, _ := syscall.UTF16PtrFromString(cmdLine)

	r1, _, err := procCreateService.Call(
		scManager,
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(displayNamePtr)),
		SERVICE_ALL_ACCESS,
		SERVICE_WIN32_OWN_PROCESS,
		SERVICE_AUTO_START,
		SERVICE_ERROR_NORMAL,
		uintptr(unsafe.Pointer(cmdLinePtr)),
		0, // No load order group
		0, // No tag
		0, // No dependencies
		0, // LocalSystem account
		0, // No password
	)
	if r1 == 0 {
		return fmt.Errorf("failed to create service: %w", err)
	}
	serviceHandle := r1
	defer closeSCHandle(serviceHandle)

	fmt.Printf("Created Windows service: %s\n", cfg.Name)

	// Set service description
	if cfg.Description != "" {
		setServiceDescription(serviceHandle, cfg.Description)
	}

	// Start the service
	r1, _, err = procStartService.Call(serviceHandle, 0, 0)
	if r1 == 0 {
		fmt.Printf("Note: service created but failed to start: %v\n", err)
		fmt.Println("You may need to start it manually with: net start", cfg.Name)
	} else {
		fmt.Printf("Started Windows service: %s\n", cfg.Name)
	}

	return nil
}

// uninstallImpl removes the Windows service.
func uninstallImpl(serviceName string) error {
	// Open Service Control Manager
	scManager, err := openSCManager()
	if err != nil {
		return fmt.Errorf("failed to open service control manager: %w", err)
	}
	defer closeSCHandle(scManager)

	// Open the service
	serviceHandle, err := openService(scManager, serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed: %w", serviceName, err)
	}
	defer closeSCHandle(serviceHandle)

	// Stop the service if running
	var status serviceStatus
	procQueryServiceStatus.Call(serviceHandle, uintptr(unsafe.Pointer(&status)))
	if status.currentState != SERVICE_STOPPED {
		fmt.Printf("Stopping service: %s\n", serviceName)
		procControlService.Call(serviceHandle, SERVICE_CONTROL_STOP, uintptr(unsafe.Pointer(&status)))
		// Wait a bit for it to stop
		for i := 0; i < 30; i++ {
			procQueryServiceStatus.Call(serviceHandle, uintptr(unsafe.Pointer(&status)))
			if status.currentState == SERVICE_STOPPED {
				break
			}
			windows.SleepEx(1000, false)
		}
		fmt.Printf("Stopped service: %s\n", serviceName)
	}

	// Delete the service
	r1, _, err := procDeleteService.Call(serviceHandle)
	if r1 == 0 {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	fmt.Printf("Removed Windows service: %s\n", serviceName)
	return nil
}

// statusImpl returns the service status on Windows.
func statusImpl(serviceName string) (string, error) {
	scManager, err := openSCManager()
	if err != nil {
		return "", fmt.Errorf("failed to open service control manager: %w", err)
	}
	defer closeSCHandle(scManager)

	serviceHandle, err := openService(scManager, serviceName)
	if err != nil {
		return "not installed", nil
	}
	defer closeSCHandle(serviceHandle)

	var status serviceStatus
	r1, _, _ := procQueryServiceStatus.Call(serviceHandle, uintptr(unsafe.Pointer(&status)))
	if r1 == 0 {
		return "unknown", nil
	}

	switch status.currentState {
	case SERVICE_STOPPED:
		return "stopped", nil
	case SERVICE_START_PENDING:
		return "starting", nil
	case SERVICE_STOP_PENDING:
		return "stopping", nil
	case SERVICE_RUNNING:
		return "running", nil
	default:
		return "unknown", nil
	}
}

// isInstalledImpl checks if the service is installed on Windows.
func isInstalledImpl(serviceName string) bool {
	scManager, err := openSCManager()
	if err != nil {
		return false
	}
	defer closeSCHandle(scManager)

	serviceHandle, err := openService(scManager, serviceName)
	if err != nil {
		return false
	}
	closeSCHandle(serviceHandle)
	return true
}

func openSCManager() (uintptr, error) {
	r1, _, err := procOpenSCManager.Call(0, 0, SC_MANAGER_ALL_ACCESS)
	if r1 == 0 {
		return 0, err
	}
	return r1, nil
}

func openService(scManager uintptr, name string) (uintptr, error) {
	namePtr, _ := syscall.UTF16PtrFromString(name)
	r1, _, err := procOpenService.Call(scManager, uintptr(unsafe.Pointer(namePtr)), SERVICE_ALL_ACCESS)
	if r1 == 0 {
		return 0, err
	}
	return r1, nil
}

func closeSCHandle(handle uintptr) {
	procCloseServiceHandle.Call(handle)
}

func setServiceDescription(serviceHandle uintptr, description string) {
	// SERVICE_DESCRIPTION structure
	type serviceDescription struct {
		description *uint16
	}

	descPtr, _ := syscall.UTF16PtrFromString(description)
	sd := serviceDescription{description: descPtr}

	// ChangeServiceConfig2W with SERVICE_CONFIG_DESCRIPTION (1)
	modAdvapi32 := windows.NewLazySystemDLL("advapi32.dll")
	procChangeServiceConfig2 := modAdvapi32.NewProc("ChangeServiceConfig2W")
	procChangeServiceConfig2.Call(serviceHandle, 1, uintptr(unsafe.Pointer(&sd)))
}

// Helper to check if output contains certain strings (for compatibility with other code)
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// isInteractiveImpl returns true if the process is running interactively (not as a Windows service).
func isInteractiveImpl() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		// If we can't determine, assume interactive
		return true
	}
	return !isService
}

// runAsServiceImpl runs the ServiceRunner as a Windows service.
func runAsServiceImpl(name string, runner ServiceRunner) error {
	return svc.Run(name, &windowsServiceHandler{runner: runner})
}

// windowsServiceHandler implements svc.Handler for Windows services.
type windowsServiceHandler struct {
	runner ServiceRunner
}

// Execute implements svc.Handler.Execute.
func (h *windowsServiceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// Report that we're starting
	changes <- svc.Status{State: svc.StartPending}

	// Start the service
	if err := h.runner.Start(); err != nil {
		// Log error and return failure
		return false, 1
	}

	// Report that we're running
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Wait for stop/shutdown signal
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				break loop
			default:
				// Ignore unknown commands
			}
		}
	}

	// Report that we're stopping
	changes <- svc.Status{State: svc.StopPending}

	// Stop the service with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := h.runner.StopWithContext(ctx); err != nil {
		// Log error but continue shutdown
		return false, 2
	}

	return false, 0
}

// =============================================================================
// User service stubs (not supported on Windows - use Windows Service instead)
// =============================================================================

// hasCrontab returns false on Windows (user service not supported).
func hasCrontab() bool {
	return false
}

// ErrCrontabNotFound is returned when crontab is not available.
var ErrCrontabNotFound = errors.New("user service installation is only supported on Linux")

// installUserImpl is not supported on Windows.
func installUserImpl(cfg ServiceConfig, execPath string) error {
	return fmt.Errorf("user service installation is only supported on Linux")
}

// uninstallUserImpl is not supported on Windows.
func uninstallUserImpl() error {
	return fmt.Errorf("user service is only supported on Linux")
}

// statusUserImpl is not supported on Windows.
func statusUserImpl() (string, error) {
	return "", fmt.Errorf("user service is only supported on Linux")
}

// startUserImpl is not supported on Windows.
func startUserImpl() error {
	return fmt.Errorf("user service is only supported on Linux")
}

// stopUserImpl is not supported on Windows.
func stopUserImpl() error {
	return fmt.Errorf("user service is only supported on Linux")
}

// isUserInstalledImpl returns false on Windows.
func isUserInstalledImpl() bool {
	return false
}
