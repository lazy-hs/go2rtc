//go:build windows

package api

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	coinitApartmentThreaded = 0x2
	clsctxInprocServer      = 0x1

	fileOpenNoChangeDirectory = 0x00000008
	fileOpenPickFolders       = 0x00000020
	fileOpenForceFileSystem   = 0x00000040
	fileOpenPathMustExist     = 0x00000800

	shellItemFileSystemPath = 0x80058000

	hresultOK        = 0x00000000
	hresultFalse     = 0x00000001
	hresultCancelled = 0x800704C7
)

var (
	ole32                         = windows.NewLazySystemDLL("ole32.dll")
	shell32                       = windows.NewLazySystemDLL("shell32.dll")
	procCoInitializeEx            = ole32.NewProc("CoInitializeEx")
	procCoUninitialize            = ole32.NewProc("CoUninitialize")
	procCoCreateInstance          = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree             = ole32.NewProc("CoTaskMemFree")
	procSHCreateItemFromParseName = shell32.NewProc("SHCreateItemFromParsingName")

	clsidFileOpenDialog = windows.GUID{
		Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE,
		Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7},
	}
	iidFileDialog = windows.GUID{
		Data1: 0x42F85136, Data2: 0xDB7E, Data3: 0x439C,
		Data4: [8]byte{0x85, 0xF1, 0xE4, 0x07, 0x5D, 0x13, 0x5F, 0xC8},
	}
	iidShellItem = windows.GUID{
		Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE,
		Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE},
	}
)

type fileDialog struct {
	vtbl *fileDialogVtbl
}

type fileDialogVtbl struct {
	queryInterface      uintptr
	addRef              uintptr
	release             uintptr
	show                uintptr
	setFileTypes        uintptr
	setFileTypeIndex    uintptr
	getFileTypeIndex    uintptr
	advise              uintptr
	unadvise            uintptr
	setOptions          uintptr
	getOptions          uintptr
	setDefaultFolder    uintptr
	setFolder           uintptr
	getFolder           uintptr
	getCurrentSelection uintptr
	setFileName         uintptr
	getFileName         uintptr
	setTitle            uintptr
	setOKButtonLabel    uintptr
	setFileNameLabel    uintptr
	getResult           uintptr
	addPlace            uintptr
	setDefaultExtension uintptr
	close               uintptr
	setClientGUID       uintptr
	clearClientData     uintptr
	setFilter           uintptr
}

type shellItem struct {
	vtbl *shellItemVtbl
}

type shellItemVtbl struct {
	queryInterface uintptr
	addRef         uintptr
	release        uintptr
	bindToHandler  uintptr
	getParent      uintptr
	getDisplayName uintptr
	getAttributes  uintptr
	compare        uintptr
}

func simulateNativeFolderPickerAvailable() bool {
	return true
}

func simulatePickNativeFolder(ctx context.Context, initialDir string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	selectedDir, err := showWindowsFolderPicker(initialDir)
	if err != nil {
		return "", err
	}
	if selectedDir == "" {
		return "", errSimulateFolderPickerCancelled
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return selectedDir, nil
}

func showWindowsFolderPicker(initialDir string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hresult, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if hresultFailed(hresult) {
		return "", hresultError("CoInitializeEx", hresult)
	}
	if hresult == hresultOK || hresult == hresultFalse {
		defer procCoUninitialize.Call()
	}

	var dialog *fileDialog
	hresult, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if hresultFailed(hresult) {
		return "", hresultError("CoCreateInstance(IFileOpenDialog)", hresult)
	}
	defer releaseFileDialog(dialog)

	var options uint32
	hresult, _, _ = syscall.SyscallN(
		dialog.vtbl.getOptions,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(&options)),
	)
	if hresultFailed(hresult) {
		return "", hresultError("IFileDialog.GetOptions", hresult)
	}
	options |= fileOpenNoChangeDirectory | fileOpenPickFolders | fileOpenForceFileSystem | fileOpenPathMustExist
	hresult, _, _ = syscall.SyscallN(
		dialog.vtbl.setOptions,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(options),
	)
	if hresultFailed(hresult) {
		return "", hresultError("IFileDialog.SetOptions", hresult)
	}

	if err := setFileDialogText(dialog, dialog.vtbl.setTitle, "选择要上传的文件夹"); err != nil {
		return "", err
	}
	if err := setFileDialogText(dialog, dialog.vtbl.setOKButtonLabel, "选择文件夹"); err != nil {
		return "", err
	}
	if err := setFileDialogText(dialog, dialog.vtbl.setFileNameLabel, "文件夹:"); err != nil {
		return "", err
	}

	initialDir = strings.TrimSpace(initialDir)
	if initialDir != "" {
		initialPath, err := windows.UTF16PtrFromString(initialDir)
		if err != nil {
			return "", err
		}
		var initialFolder *shellItem
		hresult, _, _ = procSHCreateItemFromParseName.Call(
			uintptr(unsafe.Pointer(initialPath)),
			0,
			uintptr(unsafe.Pointer(&iidShellItem)),
			uintptr(unsafe.Pointer(&initialFolder)),
		)
		runtime.KeepAlive(initialPath)
		if !hresultFailed(hresult) {
			defer releaseShellItem(initialFolder)
			hresult, _, _ = syscall.SyscallN(
				dialog.vtbl.setDefaultFolder,
				uintptr(unsafe.Pointer(dialog)),
				uintptr(unsafe.Pointer(initialFolder)),
			)
			if hresultFailed(hresult) {
				return "", hresultError("IFileDialog.SetDefaultFolder", hresult)
			}
			hresult, _, _ = syscall.SyscallN(
				dialog.vtbl.setFolder,
				uintptr(unsafe.Pointer(dialog)),
				uintptr(unsafe.Pointer(initialFolder)),
			)
			if hresultFailed(hresult) {
				return "", hresultError("IFileDialog.SetFolder", hresult)
			}
		}
	}

	hresult, _, _ = syscall.SyscallN(dialog.vtbl.show, uintptr(unsafe.Pointer(dialog)), 0)
	if uint32(hresult) == hresultCancelled {
		return "", nil
	}
	if hresultFailed(hresult) {
		return "", hresultError("IFileDialog.Show", hresult)
	}

	var selectedItem *shellItem
	hresult, _, _ = syscall.SyscallN(
		dialog.vtbl.getResult,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(&selectedItem)),
	)
	if hresultFailed(hresult) {
		return "", hresultError("IFileDialog.GetResult", hresult)
	}
	defer releaseShellItem(selectedItem)

	var pathPointer uintptr
	hresult, _, _ = syscall.SyscallN(
		selectedItem.vtbl.getDisplayName,
		uintptr(unsafe.Pointer(selectedItem)),
		shellItemFileSystemPath,
		uintptr(unsafe.Pointer(&pathPointer)),
	)
	if hresultFailed(hresult) {
		return "", hresultError("IShellItem.GetDisplayName", hresult)
	}
	if pathPointer == 0 {
		return "", errors.New("native folder picker returned an empty path")
	}
	defer procCoTaskMemFree.Call(pathPointer)

	selectedDir := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(pathPointer)))
	if strings.TrimSpace(selectedDir) == "" {
		return "", errors.New("native folder picker returned an empty path")
	}
	return selectedDir, nil
}

func setFileDialogText(dialog *fileDialog, method uintptr, value string) error {
	text, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return err
	}
	hresult, _, _ := syscall.SyscallN(
		method,
		uintptr(unsafe.Pointer(dialog)),
		uintptr(unsafe.Pointer(text)),
	)
	runtime.KeepAlive(text)
	if hresultFailed(hresult) {
		return hresultError("IFileDialog text setting", hresult)
	}
	return nil
}

func releaseFileDialog(dialog *fileDialog) {
	if dialog != nil && dialog.vtbl != nil {
		syscall.SyscallN(dialog.vtbl.release, uintptr(unsafe.Pointer(dialog)))
	}
}

func releaseShellItem(item *shellItem) {
	if item != nil && item.vtbl != nil {
		syscall.SyscallN(item.vtbl.release, uintptr(unsafe.Pointer(item)))
	}
}

func hresultFailed(hresult uintptr) bool {
	return int32(uint32(hresult)) < 0
}

func hresultError(operation string, hresult uintptr) error {
	return fmt.Errorf("%s failed: HRESULT 0x%08X", operation, uint32(hresult))
}
