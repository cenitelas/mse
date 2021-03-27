package main

import (
	"fmt"
	"log"
	"mse/av"
	"mse/format/rtsp"
)

type StreamST struct {
	Uuid    string
	URL     string
	Codecs  []av.CodecData
	Cl      chan av.Packet
	StatusC chan string
	Status  string
}

func RTSPWorker(stream *StreamST) {
	log.Println("WORKER START")
	inRtsp, err := rtsp.Dial(stream.URL)
	if err != nil {
		Logger.Error(fmt.Sprintf("Error :%s", err.Error()))
		stream.SetStatus("error")
		return
	}
	streams, _ := inRtsp.Streams()
	stream.Codecs = streams

	stream.SetStatus("connect")
	Logger.Success(fmt.Sprintf("Connect :%s", stream.URL))
	for {
		select {
		case status := <-stream.StatusC:
			switch status {
			case "error":
				Logger.Error(fmt.Sprintf("Error connect: %s", stream.URL))
				inRtsp.Close()
				return
			case "close":
				Logger.Error(fmt.Sprintf("Close stream: %s", stream.URL))
				inRtsp.Close()
				return
			default:
				break
			}
		default:
			var pck av.Packet

			if pck, err = inRtsp.ReadPacket(); err != nil {
				Logger.Info(fmt.Sprintf("Reconnect :%s", stream.URL))
				inRtsp.Close()
				go RTSPWorker(stream)
				return
			}

			stream.Cl <- pck
		}
	}
	inRtsp.Close()
	log.Println("WORKER CLOSE")
}

func (stream *StreamST) SetStatus(status string) {
	stream.StatusC <- status
	stream.Status = status
}
