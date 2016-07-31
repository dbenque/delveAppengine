// +build linux

package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/mitchellh/go-ps"
)

// LinuxProcess representation of linux process
type LinuxProcess struct {
	p         ps.Process
	startTime uint64
	zombie    bool
	done      bool
}

// Pid is the process ID for this process.
func (l *LinuxProcess) Pid() int {
	return l.p.Pid()
}

// PPid is the parent process ID for this process.
func (l *LinuxProcess) PPid() int {
	return l.p.PPid()
}

// Executable name running this process. This is not a path to the
// executable.
func (l *LinuxProcess) Executable() string {
	return l.p.Executable()
}

// StartTime returns process Start time
func (l *LinuxProcess) StartTime() uint64 {
	if l.done {
		return l.startTime
	}

	l.startTime, l.zombie = getProcessStartTime(l.Pid())
	l.done = true
	return l.startTime
}

// Zombie returns if the process is a zombie process
func (l *LinuxProcess) Zombie() bool {
	if l.done {
		return l.zombie
	}

	l.startTime, l.zombie = getProcessStartTime(l.Pid())
	l.done = true
	return l.zombie
}

func processes() ([]Process, error) {
	result := []Process{}
	ps, err := ps.Processes()
	if err != nil {
		return result, err
	}

	for _, p := range ps {
		result = append(result, &LinuxProcess{done: false, p: p})
	}
	return result, nil
}

//return the start time and a bool indicating if the process is Zombie
func getProcessStartTime(pid int) (uint64, bool) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	dataBytes, err := ioutil.ReadFile(statPath)
	if err != nil {
		return 0, false
	}

	// First, parse out the image name (can contain space char)
	data := string(dataBytes)
	binStart := strings.IndexRune(data, '(') + 1
	binEnd := strings.IndexRune(data[binStart:], ')')

	fields := strings.Split(data[binStart+binEnd+2:], " ")
	// http://man7.org/linux/man-pages/man5/proc.5.html
	//(field 22 is starttime (index 21)) and we have already shifted by two elements

	startTime, _ := strconv.ParseUint(fields[21-2], 10, 64)
	return startTime, fields[0] == "Z"
}

func binaryContainsMagicKey(pid int, key string) bool {
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	dataBytes, err := ioutil.ReadFile(exePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(dataBytes), key)
}
