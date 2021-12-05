package main

import (
	"fmt"
	"github.com/fatih/color"
	"log"
	"os"
	"time"
)

type logger struct {
	enable bool
}

var Logger = logger{enable: true}

func (l *logger) Info(text string) {
	c := color.New(color.FgYellow)
	c.Println(fmt.Sprintf("[MSE %s] INFO | %s", time.Now().Format("02-01-2006 15:04:05"), text))
	saveToFile(fmt.Sprintf("[MSE %s] INFO | %s\n", time.Now().Format("02-01-2006 15:04:05"), text))
}

func (l *logger) Success(text string) {
	c := color.New(color.FgHiGreen)
	c.Println(fmt.Sprintf("[MSE %s] SUCCESS | %s", time.Now().Format("02-01-2006 15:04:05"), text))
	saveToFile(fmt.Sprintf("[MSE %s] SUCCESS | %s\n", time.Now().Format("02-01-2006 15:04:05"), text))
}

func (l *logger) Error(text string) {
	c := color.New(color.FgRed)
	c.Println(fmt.Sprintf("[MSE %s] ERROR | %s", time.Now().Format("02-01-2006 15:04:05"), text))
	saveToFile(fmt.Sprintf("[MSE %s] ERROR | %s\n", time.Now().Format("02-01-2006 15:04:05"), text))
}

func saveToFile(text string) {

	file, err := os.OpenFile("./log.txt", os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		os.Create("./log.txt")
		saveToFile(text)
		return
	}

	stat, err := file.Stat()

	if err != nil {
		log.Println("Unable stat file:", err)
		return
	}

	if stat.Size() > 10*1024*1024 {
		file.Close()
		fileClear, _ := os.OpenFile("./log.txt", os.O_APPEND|os.O_WRONLY, os.ModeAppend)
		fileClear.Truncate(0)
		fileClear.Seek(0, 0)
		fileClear.WriteString(text)
		fileClear.Close()
		return
	}
	file.WriteString(text)
	file.Close()
}
