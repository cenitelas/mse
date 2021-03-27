package main

import (
	"fmt"
	"github.com/fatih/color"
	"time"
)

type logger struct {
	enable bool
}

var Logger = logger{enable: true}

func (l *logger) Info(text string) {
	c := color.New(color.FgYellow)
	c.Println(fmt.Sprintf("[MSE %s] INFO | %s", time.Now().Format("02-01-2006 15:04:05"), text))
}

func (l *logger) Success(text string) {
	c := color.New(color.FgHiGreen)
	c.Println(fmt.Sprintf("[MSE %s] SUCCESS | %s", time.Now().Format("02-01-2006 15:04:05"), text))
}

func (l *logger) Error(text string) {
	c := color.New(color.FgRed)
	c.Println(fmt.Sprintf("[MSE %s] ERROR | %s", time.Now().Format("02-01-2006 15:04:05"), text))
}
