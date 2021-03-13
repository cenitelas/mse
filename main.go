package main

import (
	"github.com/deepch/vdk/format"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func init() {
	format.RegisterAll()
}

func main() {
	go serveHTTP()
	//go serveStreams()
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
