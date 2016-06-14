// +build darwin

package main

//#include "process_darwin.h"
import "C"

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)
import "sync"

// This lock is what verifies that C calling back into Go is only
// modifying data once at a time.
var darwinLock sync.Mutex
var darwinProcs []DarwinProcess

// DarwinProcess struc that represent a process
type DarwinProcess struct {
	pid       int
	ppid      int
	binary    string
	startTime uint64
	zombie    bool
}

// Pid returns process pid
func (p *DarwinProcess) Pid() int {
	return p.pid
}

// PPid returns process ppid
func (p *DarwinProcess) PPid() int {
	return p.ppid
}

// Executable returns the executable name
func (p *DarwinProcess) Executable() string {
	return p.binary
}

// StartTime returns process Start time
func (p *DarwinProcess) StartTime() uint64 {
	return p.startTime
}

// Zombie returns process Start time
func (p *DarwinProcess) Zombie() bool {
	return p.zombie
}

//export go_darwin_append_proc4
func go_darwin_append_proc4(pid C.pid_t, ppid C.pid_t, comm *C.char, startTime C.long, isZombie C.int) {
	proc := DarwinProcess{
		pid:       int(pid),
		ppid:      int(ppid),
		binary:    C.GoString(comm),
		startTime: uint64(startTime),
		zombie:    false,
	}

	darwinProcs = append(darwinProcs, proc)
}

//return the start time and a bool indicating if the process is Zombie
func getProcessStartTime(pid int) (uint64, bool) {
	p, err := getProcess(pid)
	if err != nil {
		fmt.Println("process error:", err)
		return 0, false
	}
	return p.StartTime(), p.Zombie()
}

func getProcess(pid int) (DarwinProcess, error) {
	processes, err := processes()
	if err != nil {
		fmt.Println("process error:", err)
	} else {
		for _, p := range processes {
			if p.Pid() == pid {
				return p, nil
			}
		}
	}

	return DarwinProcess{}, errors.New("Process not found")
}
func binaryContainsMagicKey(pid int, key string) bool {
	for _, proc := range darwinProcs {
		if proc.pid == pid {
			if binPath, err := getFullPath(proc.pid); err == nil {
				dataBytes, err := ioutil.ReadFile(binPath)
				if err != nil {
					return false
				}
				return strings.Contains(string(dataBytes), key)
			}
		}
	}
	return false
}

func processes() ([]DarwinProcess, error) {
	fmt.Println("processes")
	darwinLock.Lock()
	defer darwinLock.Unlock()
	darwinProcs = make([]DarwinProcess, 0, 50)

	_, err := C.darwinProcesses()
	if err != nil {
		return nil, err
	}

	for id, proc := range darwinProcs {
		if path, err := getFullPath(proc.pid); err == nil {
			darwinProcs[id].binary = path
		}
	}

	return darwinProcs, nil
}

func getFullPath(pid int) (string, error) {
	var mib [4]int32

	mib = [4]int32{1 /* CTL_KERN */, 38 /* KERN_PROCARGS */, int32(pid), -1}

	n := uintptr(0)
	// Get length.
	_, _, errNum := syscall.Syscall6(syscall.SYS___SYSCTL, uintptr(unsafe.Pointer(&mib[0])), 4, 0, uintptr(unsafe.Pointer(&n)), 0, 0)
	if errNum != 0 {
		return "", errNum
	}
	if n == 0 { // This shouldn't happen.
		return "", nil
	}
	buf := make([]byte, n)
	_, _, errNum = syscall.Syscall6(syscall.SYS___SYSCTL, uintptr(unsafe.Pointer(&mib[0])), 4, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0, 0)
	if errNum != 0 {
		return "", errNum
	}
	if n == 0 { // This shouldn't happen.
		return "", nil
	}

	for i, v := range buf {
		if v == 0 {
			buf = buf[:i]
			break
		}
	}

	execPath := string(buf)
	var err error
	// execPath will not be empty due to above checks.
	// Try to get the absolute path if the execPath is not rooted.
	if execPath[0] != '/' {
		execPath, err = getAbs(execPath)
		if err != nil {
			return execPath, err
		}
	}
	// For darwin KERN_PROCARGS may return the path to a symlink rather than the
	// actual executable.
	if execPath, err = filepath.EvalSymlinks(execPath); err != nil {
		return execPath, err
	}
	return execPath, nil
}

func getAbs(execPath string) (string, error) {
	initCwd, initCwdErr := os.Getwd()
	if initCwdErr != nil {
		return execPath, initCwdErr
	}
	// The execPath may begin with a "../" or a "./" so clean it first.
	// Join the two paths, trailing and starting slashes undetermined, so use
	// the generic Join function.
	return filepath.Join(initCwd, filepath.Clean(execPath)), nil
}
