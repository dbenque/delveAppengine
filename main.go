package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/derekparker/delve/service"
	"github.com/derekparker/delve/service/rpc1"
)

//DebuggedPID PID of process currently attached tot he debugger
var DebuggedPID = 0

//PidChan Used to PID the PID to whcih we need to attach the debugger
var PidChan = make(chan int)

var port int
var delaySeconds int
var magicKey string

func main() {
	flag.IntVar(&port, "port", 2345, "Port used by the Delve server")
	flag.IntVar(&delaySeconds, "delay", 3, "Time delay in seconds between each appengine process scan")
	flag.StringVar(&magicKey, "key", "", "Magic key to identify a specific module bianry (default is empty string)")
	flag.Parse()

	// Monitor the appengine modules processes
	go func() {
		checkAppengineModuleProcess()
		for range time.Tick(time.Duration(delaySeconds) * time.Second) {
			checkAppengineModuleProcess()
		}
	}()

	// Wait for a PID and attach a new debugger to it
	var stopChan chan bool
	for pid := range PidChan {
		if pid != DebuggedPID && pid != 0 {
			if stopChan != nil {
				stopChan <- true
				waitForFreePort()
			}
			DebuggedPID = pid
			stopChan = attachDelveServer(DebuggedPID)
		}
	}
}

//checkAppengineModuleProcess llok after the Appengine module process and push the latest new PID into channel
func checkAppengineModuleProcess() {
	processes, err := processes()
	if err != nil {
		log.Fatalln(err.Error())
	}

	// check each process
	pchan := make(chan int)
	go func() {
		var wg sync.WaitGroup
		defer close(pchan)
		for _, p := range processes {
			if p.Executable() == "_go_app" {
				wg.Add(1)
				go func(pid int) {
					defer wg.Done()
					if len(magicKey) == 0 || binaryContainsMagicKey(pid, magicKey) {
						pchan <- pid
					}
				}(p.Pid())
			}
		}
		wg.Wait()
	}()

	//build the slice of PIDs
	pids := sort.IntSlice{}
	for pid := range pchan {
		pids = append(pids, pid)
	}

	// keep the youngest one
	if len(pids) > 0 {
		if len(pids) == 1 {
			if pids[0] == DebuggedPID { // already attached to that one
				return
			}
		}
		PidChan <- getRecentProcess(pids)
	}
}

func waitForFreePort() {
	var errCon error
	var conn net.Conn
	for errCon == nil {
		conn, errCon = net.Dial("tcp", fmt.Sprintf(":%d", port))
		if errCon == nil {
			log.Println("Old server still listening.")
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func attachDelveServer(attachPid int) chan bool {
	stopChan := make(chan bool)
	var wgServerRunning sync.WaitGroup
	wgServerRunning.Add(1)
	go func() {
		defer close(stopChan)

		// Make a TCP listener
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		for err != nil {
			log.Printf("Couldn't start listener: %s\n", err)
			time.Sleep(1 * time.Second)
		}
		defer listener.Close()

		// Create and start a debugger server
		server := rpc1.NewServer(&service.Config{
			Listener:    listener,
			ProcessArgs: []string{},
			AttachPid:   attachPid,
			AcceptMulti: true,
		}, true)
		if err := server.Run(); err != nil {
			log.Println(err.Error())
		} else {
			defer server.Stop(false)
		}
		wgServerRunning.Done()
		<-stopChan
	}()

	//wait for the server to be running
	wgServerRunning.Wait()
	return stopChan
}

//getRecentProcess within these PIDs which one is the latest one ?
func getRecentProcess(pids sort.IntSlice) int {
	if len(pids) == 0 {
		return 0
	}
	tmax := uint64(0)
	pid := 0
	for _, p := range pids {
		t, zombie := getProcessStartTime(p)
		if zombie {
			continue
		}
		if t > tmax {
			pid = p
			tmax = t
		}
	}
	return pid
}
