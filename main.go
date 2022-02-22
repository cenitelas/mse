//+build linux darwin windows
package main

import (
	"log"
	"mse/format"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func init() {
	format.RegisterAll()
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() + 1)
	log.Println("COUNT PROCS = ", runtime.GOMAXPROCS(0))
	go serveHTTP()
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Println(sig)
		done <- true
	}()
	log.Println("Server Start Awaiting Signal")
	<-done
	log.Println("Exiting")
}
