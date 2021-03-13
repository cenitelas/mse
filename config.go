package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	uuid2 "github.com/google/uuid"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"github.com/deepch/vdk/av"
)

//Config global
var Config = ConfigST{Server: ServerST{HTTPPort: ":4090"}, Streams: map[string]StreamST{}}

//ConfigST struct
type ConfigST struct {
	mutex   sync.RWMutex
	Server  ServerST            `json:"server"`
	Streams map[string]StreamST `json:"streams"`
}

//ServerST struct
type ServerST struct {
	HTTPPort string `json:"http_port"`
}

//StreamST struct
type StreamST struct {
	URL      string `json:"url"`
	Status   bool   `json:"status"`
	OnDemand bool   `json:"on_demand"`
	RunLock  bool   `json:"-"`
	Codecs   []av.CodecData
	Cl       map[string]viewer
}

type StreamMP4 struct {
	Codecs []av.CodecData
	Cl     map[string]viewer
}

type viewer struct {
	c chan av.Packet
}

var Mp4stream = StreamMP4{Cl: make(map[string]viewer)}

func (element *ConfigST) RunIFNotRun(uuid string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	if tmp, ok := element.Streams[uuid]; ok {
		if tmp.OnDemand && !tmp.RunLock {
			tmp.RunLock = true
			element.Streams[uuid] = tmp
			go RTSPWorkerLoop(uuid, tmp.URL, tmp.OnDemand)
		}
	}
}

func (element *ConfigST) RunUnlock(uuid string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	if tmp, ok := element.Streams[uuid]; ok {
		if tmp.OnDemand && tmp.RunLock {
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

func (element *ConfigST) PushStream(url string) string {
	stream := StreamST{
		URL:      url,
		Cl:       make(map[string]viewer),
		OnDemand: true,
	}
	uuid := uuid2.New().String()
	element.Streams[uuid] = stream
	return uuid
}

func loadConfig() *ConfigST {
	var tmp ConfigST
	data, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatalln(err)
	}
	err = json.Unmarshal(data, &tmp)
	if err != nil {
		log.Fatalln(err)
	}
	for i, v := range tmp.Streams {
		v.Cl = make(map[string]viewer)
		tmp.Streams[i] = v
	}
	log.Println(tmp.Streams["H264_PCMALAW"])
	return &tmp
}

func (element *ConfigST) StreamExists(url string) *string {
	for i, item := range element.Streams {
		if item.URL == url {
			return &i
		}
	}
	return nil
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

func (element *StreamMP4) castMP4(pck av.Packet) {
	for _, v := range element.Cl {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
}

func (element *StreamMP4) coAdMP4(codecs []av.CodecData) {
	element.Codecs = codecs
}

func (element *ConfigST) ext(suuid string) bool {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	_, ok := element.Streams[suuid]
	return ok
}

func (element *ConfigST) coAd(suuid string, codecs []av.CodecData) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	t := element.Streams[suuid]
	t.Codecs = codecs
	element.Streams[suuid] = t
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

func (element *StreamMP4) coGeMP4() []av.CodecData {
	for i := 0; i < 100; i++ {
		if element.Codecs != nil {
			return element.Codecs
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func (element *ConfigST) clAd(suuid string) (string, chan av.Packet) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	cuuid := pseudoUUID()
	ch := make(chan av.Packet, 100)
	element.Streams[suuid].Cl[cuuid] = viewer{c: ch}
	return cuuid, ch
}

func (element *StreamMP4) clAdMP4() (string, chan av.Packet) {
	cuuid := pseudoUUID()
	ch := make(chan av.Packet, 100)
	element.Cl[cuuid] = viewer{c: ch}
	return cuuid, ch
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
func (element *ConfigST) clDe(suuid, cuuid string) {
	element.mutex.Lock()
	defer element.mutex.Unlock()
	delete(element.Streams[suuid].Cl, cuuid)
}

func (element *StreamMP4) clDeMP4(cuuid string) {
	delete(element.Cl, cuuid)
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
