package main

import (
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/derekparker/delve/service"
	"github.com/derekparker/delve/service/rpc"
	"github.com/mitchellh/go-ps"
)

func main() {
	debuggedPID := 0
	pidchan := make(chan int)
	ticker := time.NewTicker(3 * time.Second)

	// Monitor the appengine modules process
	go func() {
		for range ticker.C {
			processes, err := ps.Processes()
			if err != nil {
				log.Fatalln(err.Error())
			}
			pids := sort.IntSlice{}
			for _, p := range processes {
				if p.Executable() == "_go_app" {
					pids = append(pids, p.Pid())
				}
			}
			if len(pids) > 0 {
				if len(pids) == 1 {
					if pids[0] == debuggedPID { // already attached to that one
						continue
					}
				}
				pidchan <- getRecentProcess(pids)
			}
		}
	}()

	// Wait for a PID and attach a new debugger to it
	var stopChan chan bool
	for pid := range pidchan {
		if pid != debuggedPID && pid != 0 {
			if stopChan != nil {
				stopChan <- true

				//wait for the port to be free
				var errCon error
				var conn net.Conn
				for errCon == nil {
					conn, errCon = net.Dial("tcp", ":2345")
					if errCon == nil {
						log.Println("Old server still listening.")
						conn.Close()
						time.Sleep(1 * time.Second)
					}
				}
			}
			debuggedPID = pid
			stopChan = attachDelveServer(debuggedPID)
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
		listener, err := net.Listen("tcp", ":2345")
		for err != nil {
			log.Printf("Couldn't start listener: %s\n", err)
			time.Sleep(1 * time.Second)
		}
		defer listener.Close()

		// Create and start a debugger server
		server := rpc.NewServer(&service.Config{
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
		t := getProcessStartTime(p)
		if t > tmax {
			pid = p
			tmax = t
		}
	}
	return pid
}
