package main

import (
	"crypto/rand"
	"fmt"
	uuid2 "github.com/google/uuid"
	"mse/av"
	"sync"
	"time"
)

var Config = ConfigST{Server: ServerST{HTTPPort: ":4090"}, Streams: map[string]*Stream{}}

type ConfigST struct {
	Server  ServerST           `json:"server"`
	Streams map[string]*Stream `json:"streams"`
	mutex   sync.RWMutex
}

type ServerST struct {
	HTTPPort string `json:"http_port"`
}

func (element *ConfigST) Run(uuid string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	if tmp, ok := element.Streams[uuid]; ok {
		if !tmp.RunLock {
			tmp.RunLock = true
			element.Streams[uuid] = tmp
			go RTSPWorker(tmp)
		}
	}
}

func (element *ConfigST) PushStream(url string) string {
	exists := element.StreamExists(url)
	if exists != nil {
		return exists.Uuid
	}
	stream := Stream{
		Uuid:   uuid2.New().String(),
		URL:    url,
		Cl:     make(map[string]viewer),
		Status: make(chan string),
	}
	element.Streams[stream.Uuid] = &stream
	return stream.Uuid
}

func (element *ConfigST) PushStreamArchive(stream Stream) string {
	element.Streams[stream.Uuid] = &stream
	return stream.Uuid
}

func loadConfig() *ConfigST {
	return nil
}

func (element *ConfigST) StreamExists(url string) *Stream {
	for _, item := range element.Streams {
		if item.URL == url {
			return element.Streams[item.Uuid]
		}
	}
	return nil
}

func (element *ConfigST) RunUnlock(uuid string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	if tmp, ok := element.Streams[uuid]; ok {
		if tmp.RunLock {
			tmp.RunLock = false
			element.Streams[uuid] = tmp
		}
	}
}

func (element *ConfigST) HasViewer(uuid string) bool {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	if tmp, ok := element.Streams[uuid]; ok && len(tmp.Cl) > 0 {
		return true
	}
	return false
}

func (element *ConfigST) cast(uuid string, pck av.Packet) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	for _, v := range element.Streams[uuid].Cl {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
}

func (element *ConfigST) ext(uuid string) bool {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	_, ok := element.Streams[uuid]
	return ok
}

func (element *ConfigST) coAd(uuid string, codecs []av.CodecData) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	t := element.Streams[uuid]
	t.Codecs = codecs
	element.Streams[uuid] = t
}

func (element *ConfigST) coGe(suuid string) []av.CodecData {
	for i := 0; i < 100; i++ {
		element.mutex.RLock()
		tmp, ok := element.Streams[suuid]
		element.mutex.RUnlock()
		if !ok {
			return nil
		}
		if tmp.Codecs != nil {
			return tmp.Codecs
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func (element *ConfigST) clAd(uuid string) (string, chan av.Packet, chan string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	cuuid := pseudoUUID()
	ch := make(chan av.Packet, 100)
	element.Streams[uuid].Cl[cuuid] = viewer{c: ch}
	return cuuid, ch, element.Streams[uuid].Status
}

func (element *ConfigST) list() (string, []string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	var res []string
	var fist string
	for k := range element.Streams {
		if fist == "" {
			fist = k
		}
		res = append(res, k)
	}
	return fist, res
}

func (element *ConfigST) clDe(uuid, cuuid string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	delete(element.Streams[uuid].Cl, cuuid)
}

func pseudoUUID() (uuid string) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	uuid = fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return
}
