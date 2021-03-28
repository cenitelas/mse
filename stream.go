package main

import (
	"fmt"
	"golang.org/x/net/websocket"
	"log"
	"mse/av"
	"mse/av/avutil"
	"mse/codec/h264parser"
	"mse/codec/h265parser"
	"mse/format/mp4f"
	"mse/format/rtsp"
	"path"
	"strings"
	"time"
)

type Stream struct {
	Uuid    string
	URL     string
	RunLock bool
	Codecs  []av.CodecData
	Cl      map[string]viewer
	Status  chan string
}

type Video struct {
	Data      av.DemuxCloser
	Codecs    []av.CodecData
	StartTime time.Time
	Size      int64
}

type viewer struct {
	c chan av.Packet
}

func RTSPWorker(stream *Stream) {
	inRtsp, err := rtsp.Dial(stream.URL)
	if err != nil {
		Logger.Error(fmt.Sprintf("Error :%s", err.Error()))
		Config.RunUnlock(stream.Uuid)
		return
	}
	defer inRtsp.Close()
	codec, _ := inRtsp.Streams()
	stream.Codecs = codec

	Logger.Success(fmt.Sprintf("Connect :%s", stream.URL))
	for {
		select {
		case status := <-stream.Status:
			switch status {
			case "reconnect":
				inRtsp.Close()
				Logger.Info(fmt.Sprintf("Reconnect :%s", stream.URL))
				go RTSPWorker(stream)
				return
			case "close":
				inRtsp.Close()
				Logger.Info(fmt.Sprintf("Close :%s", stream.URL))
				Config.RunUnlock(stream.Uuid)
				return
			}
		default:
			var pck av.Packet

			if pck, err = inRtsp.ReadPacket(); err != nil {
				inRtsp.Close()
				Logger.Info(fmt.Sprintf("Reconnect :%s", stream.URL))
				go RTSPWorker(stream)
				return
			}

			Config.cast(stream.Uuid, pck)
		}
	}
}

func ArchiveWorker(packet chan av.Packet, socketStatus chan bool, paths []string, timeStart time.Time, codecControl chan []av.CodecData) {
	pathFiles := filesStream(paths, timeStart)
	files := make(map[string]Video)
	for _, pathFile := range pathFiles {
		in, _ := avutil.Open(pathFile)
		st, _ := in.Streams()
		_, fileName := path.Split(pathFile)
		startTime, _ := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fileName, ".flv", ""))
		v := Video{
			Data:      in,
			Codecs:    st,
			StartTime: startTime,
		}
		files[pathFile] = v
	}
	keyframe := false
	var timeStream time.Duration
	var err error
	infile, _ := avutil.Open("test.flv")
	codecs, _ := infile.Streams()
	codecControl <- codecs
	loaded := Video{
		Data:   infile,
		Codecs: codecs,
	}
	loader := false
	count := 0
	totalCodec := loaded.Codecs
	var prev av.Packet
	for {
		select {
		case <-socketStatus:
			loaded.Data.Close()
			for _, pathFile := range pathFiles {
				files[pathFile].Data.Close()
			}
			Logger.Info(fmt.Sprintf("Close stream archive"))
			return
		default:
			var pck av.Packet
			if count > len(paths)-1 || (count < len(paths) && timeStart.Add(5*time.Second).Before(files[pathFiles[count]].StartTime)) {
				if loader == false {
					loader = true
				}
				if Meta(totalCodec[0]) != Meta(loaded.Codecs[0]) {
					codecControl <- codecs
					totalCodec = codecs
					time.Sleep(100 * time.Millisecond)
				}

				if pck, err = loaded.Data.ReadPacket(); err != nil {
					prev.Time = 0
					loaded.Data.Close()
					infile, _ = avutil.Open("test.flv")
					codecs, _ = infile.Streams()
					loaded = Video{
						Data:   infile,
						Codecs: codecs,
					}
					continue
				}
			} else {
				if loader == true {
					loader = false

				}
				if Meta(totalCodec[0]) != Meta(files[pathFiles[count]].Codecs[0]) {
					codecControl <- files[pathFiles[count]].Codecs
					totalCodec = files[pathFiles[count]].Codecs
					time.Sleep(100 * time.Millisecond)
				}
				if pck, err = files[pathFiles[count]].Data.ReadPacket(); err != nil {
					files[pathFiles[count]].Data.Close()
					prev.Time = 0
					count += 1
					continue
				}
			}

			if timeStart.After(files[pathFiles[count]].StartTime.Add(pck.Time + (5 * time.Second))) {
				continue
			}

			if pck.IsKeyFrame {
				keyframe = true
			}

			if !keyframe {
				continue
			}

			if (pck.Time - prev.Time) < 0 {
				prev = pck
			}

			if prev.Time == 0 {
				prev = pck
			}

			timeStream += pck.Time - prev.Time
			timeStart = timeStart.Add(pck.Time - prev.Time)
			time.Sleep(pck.Time - prev.Time)
			prev = pck

			pck.Time = timeStream

			packet <- pck
		}
	}
	loaded.Data.Close()
}

func PlayStreamRTSP(ws *websocket.Conn, workerStatus chan string, packet chan av.Packet, muxer *mp4f.Muxer) {
	statusSocket := make(chan bool)
	go func() {
		for {
			var message string
			err := websocket.Message.Receive(ws, &message)
			if err != nil {
				statusSocket <- false
				ws.Close()
			}
		}
	}()
	interval := time.NewTimer(10 * time.Second)
	reconnectFun := func() {
		interval.Reset(10 * time.Second)
		workerStatus <- "reconnect"
	}

	timeStream := time.Duration(0)
	var prev av.Packet
	keyframe := false

	for {
		select {

		case pck := <-packet:
			if pck.IsKeyFrame {
				keyframe = true
			}

			if !keyframe {
				continue
			}

			if (pck.Time - prev.Time) < 0 {
				prev = pck
			}

			if prev.Time == 0 {
				prev = pck
			}

			timeStream += pck.Time - prev.Time
			prev = pck
			pck.Time = timeStream

			ready, buf, err := muxer.WritePacket(pck, false)
			if err != nil {
				log.Println(err)
			}
			if ready {
				interval.Reset(10 * time.Second)
				err := ws.SetWriteDeadline(time.Now().Add(60 * time.Second))
				if err != nil {
					log.Println(err)
					return
				}
				err = websocket.Message.Send(ws, buf)
				if err != nil {
					log.Println(err)
					return
				}
			}
		case <-interval.C:
			reconnectFun()

		case <-statusSocket:
			return
		}
	}
}

func PlayStreamArchive(packet chan av.Packet, statusSocket chan bool, ws *websocket.Conn, muxer *mp4f.Muxer, codecControl chan []av.CodecData) {
	go func() {
		for {
			var message string
			err := websocket.Message.Receive(ws, &message)
			if err != nil {
				statusSocket <- false
				ws.Close()
			}
		}
	}()

	for {
		select {
		case codec := <-codecControl:
			InitMuxer(muxer, codec, ws)
			continue
		case pck := <-packet:
			ready, buf, err := muxer.WritePacket(pck, false)
			if err != nil {
				log.Println(err)
			}
			if ready {
				err := ws.SetWriteDeadline(time.Now().Add(60 * time.Second))
				if err != nil {
					log.Println(err)
					return
				}
				err = websocket.Message.Send(ws, buf)
				if err != nil {
					log.Println(err)
					return
				}
			}
		case <-statusSocket:
			return
		}
	}
}

func InitMuxer(muxer *mp4f.Muxer, codec []av.CodecData, ws *websocket.Conn) error {
	Config.mutex.Lock()
	defer Config.mutex.Unlock()
	err := muxer.WriteHeader(codec)
	if err != nil {
		Logger.Error(fmt.Sprintf("Muxer: %s", err.Error()))
		return err
	}
	meta, init := muxer.GetInit(0)

	err = websocket.Message.Send(ws, append([]byte{9}, meta...))
	if err != nil {
		Logger.Error(fmt.Sprintf("Websocket: %s", err.Error()))
		return err
	}
	err = websocket.Message.Send(ws, init)
	if err != nil {
		Logger.Error(fmt.Sprintf("Websocket: %s", err.Error()))
		return err
	}
	return nil
}

func Meta(codec av.CodecData) string {
	if codec.Type() == av.H264 {
		codec := codec.(h264parser.CodecData)
		return fmt.Sprintf("avc1.%02X%02X%02X", codec.RecordInfo.AVCProfileIndication, codec.RecordInfo.ProfileCompatibility, codec.RecordInfo.AVCLevelIndication)
	} else if codec.Type() == av.H265 {
		codec := codec.(h265parser.CodecData)
		return fmt.Sprintf("hvc1.%02X%02X%02X", codec.RecordInfo.AVCProfileIndication, codec.RecordInfo.ProfileCompatibility, codec.RecordInfo.AVCLevelIndication)

	} else if codec.Type() == av.AAC {
		return "mp4a.40.2"

	}
	return ""
}
