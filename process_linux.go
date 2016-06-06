// +build linux

package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

func getProcessStartTime(pid int) uint64 {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	dataBytes, err := ioutil.ReadFile(statPath)
	if err != nil {
		return 0
	}

	// First, parse out the image name (can contain space char)
	data := string(dataBytes)
	binStart := strings.IndexRune(data, '(') + 1
	binEnd := strings.IndexRune(data[binStart:], ')')

	fields := strings.Split(data[binStart+binEnd+2:], " ")
	// http://man7.org/linux/man-pages/man5/proc.5.html
	//(field 22 is starttime (index 21)) and we have already shifted by two elements

	startTime, _ := strconv.ParseUint(fields[21-2], 10, 64)
	return startTime
}

func binaryContainsMagicKey(pid int, key string) bool {
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	dataBytes, err := ioutil.ReadFile(exePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(dataBytes), key)
}
