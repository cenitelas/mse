package main

import (
	uuid2 "github.com/google/uuid"
	"mse/av"
)

var Config = ConfigST{Server: ServerST{HTTPPort: ":4090"}, Streams: map[string]*StreamST{}, AppCloser: make(chan bool)}

type ConfigST struct {
	Server    ServerST             `json:"server"`
	Streams   map[string]*StreamST `json:"streams"`
	AppCloser chan bool
}

type ServerST struct {
	HTTPPort string `json:"http_port"`
}

func (element *ConfigST) PushStream(url string) string {
	exists := element.StreamExists(url)
	if exists != nil {
		return exists.Uuid
	}
	stream := StreamST{
		Uuid:    uuid2.New().String(),
		URL:     url,
		Cl:      make(chan av.Packet),
		StatusC: make(chan string),
		Status:  "",
	}

	element.Streams[stream.Uuid] = &stream
	go RTSPWorker(&stream)
	return stream.Uuid
}

func loadConfig() *ConfigST {
	return nil
}

func (element *ConfigST) StreamExists(url string) *StreamST {
	for _, item := range element.Streams {
		if item.URL == url {
			return element.Streams[item.Uuid]
		}
	}
	return nil
}
