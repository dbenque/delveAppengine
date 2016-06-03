package main

import (
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/derekparker/delve/service/rpc"
	"github.com/mitchellh/go-ps"

	"github.com/derekparker/delve/service"
)

func main() {
	debuggedPID := 0
	pidchan := make(chan int)
	ticker := time.NewTicker(3 * time.Second)
	// Monitor the appengine modules
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
					if pids[0] == debuggedPID {
						continue
					}
				}
				pidchan <- getRecentProcess(pids)
			}
		}
	}()

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
						log.Println("still able to dial")
						conn.Close()
						time.Sleep(1 * time.Second)
					}
				}

			}
			debuggedPID = pid
			stopChan = executeDlv(debuggedPID)
		}
	}
}

func executeDlv(attachPid int) chan bool {
	stopChan := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer close(stopChan)

		// Make a TCP listener
		listener, err := net.Listen("tcp", ":2345")
		for err != nil {
			log.Printf("couldn't start listener: %s\n", err)
			time.Sleep(1 * time.Second)
		}
		defer listener.Close()

		// Create and start a debugger server
		server := rpc.NewServer(&service.Config{
			Listener:    listener,
			ProcessArgs: []string{},
			AttachPid:   attachPid,
			AcceptMulti: false,
		}, true)
		if err := server.Run(); err != nil {
			log.Println(err.Error())
		} else {
			log.Printf("DLV Server started for PID %d\n", attachPid)
			defer server.Stop(false)
		}
		wg.Done()
		<-stopChan
	}()

	//wait for the server to be running
	wg.Wait()

	return stopChan
}
