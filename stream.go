package main

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"

	//"github.com/gorilla/websocket"
	"log"
	"mse/av"
	"mse/av/avutil"
	"mse/codec/h264parser"
	"mse/codec/h265parser"
	"mse/format/mp4f"
	"mse/format/rtmp"
	"mse/format/rtsp"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

type Stream struct {
	Uuid         string
	URL          string
	Type         string
	RunLock      bool
	Codecs       []av.CodecData
	Cl           map[string]viewer
	CountConnect int
}

type Video struct {
	Data      av.DemuxCloser
	Codecs    []av.CodecData
	StartTime time.Time
	Size      int64
}

type viewer struct {
	c chan *av.Packet
}

func RTSPWorker(stream *Stream) {
	rtsp.DebugRtsp = false
	inRtsp, err := rtsp.DialTimeout(stream.URL, time.Duration(time.Second*20))
	if err != nil {
		Logger.Error(fmt.Sprintf("Error :%s", err.Error()))
		Config.RunUnlock(stream.Uuid)
		return
	}
	defer inRtsp.Close()
	codecs, err := inRtsp.Streams()
	if err != nil {
		Logger.Error(err.Error())
	}
	var codec av.CodecData

	if len(codecs) > 0 {
		codec = codecs[0]
	} else {
		Logger.Error("Codec error: len = 0")
		inRtsp.Close()
		Config.RunUnlock(stream.Uuid)
		return
	}
	stream.Codecs = []av.CodecData{codec}
	Logger.Success(fmt.Sprintf("Connect :%s", stream.URL))
	for {
		if !stream.RunLock {
			inRtsp.Close()
			return
		}

		if Config.connectGet(stream.Uuid) == 0 {
			inRtsp.Close()
			Config.RunUnlock(stream.Uuid)
			Logger.Info("Connect clients is 0")
			return
		}

		var pck av.Packet
		if pck, err = inRtsp.ReadPacket(); err != nil {
			inRtsp.Close()
			Logger.Error(err.Error())
			Logger.Info(fmt.Sprintf("Reconnect :%s", stream.URL))
			time.Sleep(3 * time.Second)
			go RTSPWorker(stream)
			return
		}
		if codecs[pck.Idx].Type().IsAudio() {
			continue
		}
		Config.cast(stream.Uuid, &pck)
	}
}

func RTMPWorker(stream *Stream) {
	inRtsp, err := rtmp.DialTimeout(stream.URL, time.Duration(time.Second*20))
	defer inRtsp.Close()
	if err != nil {
		Logger.Error(fmt.Sprintf("Error :%s", err.Error()))
		Config.RunUnlock(stream.Uuid)
		return
	}

	codecs, err := inRtsp.Streams()

	if err != nil {
		Logger.Error(err.Error())
	}
	stream.Codecs = codecs
	Logger.Success(fmt.Sprintf("Connect :%s", stream.URL))
	for {
		var pck av.Packet
		if !stream.RunLock {
			return
		}

		if Config.connectGet(stream.Uuid) == 0 {
			inRtsp.Close()
			Config.RunUnlock(stream.Uuid)
			Logger.Info("Connect clients is 0")
			return
		}

		if pck, err = inRtsp.ReadPacket(); err != nil {
			inRtsp.Close()
			Logger.Info(fmt.Sprintf("Reconnect :%s", stream.URL))
			time.Sleep(3 * time.Second)
			go RTMPWorker(stream)
			return
		}
		if codecs[pck.Idx].Type().IsAudio() {
			continue
		}
		Config.cast(stream.Uuid, &pck)
	}
}

func ArchiveWorker(packet chan av.Packet, socketStatus chan bool, paths []string, timeStart time.Time, codecControl chan []av.CodecData, fileStart string, speed int) {
	pathFiles := filesStream(paths, timeStart, fileStart)
	Logger.Success(fmt.Sprintf("Start stream %s in files:", timeStart.String()))
	next := func(pathFile string) *Video {
		in, _ := avutil.Open(pathFile)
		st, _ := in.Streams()
		_, fileName := path.Split(pathFile)
		startTime, _ := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fileName, ".flv", ""))
		v := Video{
			Data:      in,
			Codecs:    st,
			StartTime: startTime,
		}
		return &v
	}
	channel := next(pathFiles[0])
	defer channel.Data.Close()
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
	defer loaded.Data.Close()
	loader := false
	count := 0
	totalCodec := loaded.Codecs
	var prev av.Packet
	for {
		select {
		case <-socketStatus:
			loaded.Data.Close()
			channel.Data.Close()
			pathFiles = nil
			paths = nil
			channel = nil
			Logger.Info(fmt.Sprintf("Close stream archive"))
			return
		default:
			var pck av.Packet
			if count > len(pathFiles)-1 || (count < len(pathFiles) && timeStart.Add(5*time.Second).Before(channel.StartTime)) {
				if loader == false {
					loader = true
				}
				if Meta(totalCodec[0]) != Meta(loaded.Codecs[0]) {
					codecControl <- codecs
					totalCodec = codecs
					time.Sleep(100 * time.Millisecond)
					continue
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
				if Meta(totalCodec[0]) != Meta(channel.Codecs[0]) {
					codecControl <- channel.Codecs
					totalCodec = channel.Codecs
					time.Sleep(100 * time.Millisecond)
					continue
				}
				if pck, err = channel.Data.ReadPacket(); err != nil {
					channel.Data.Close()
					prev.Time = 0
					count += 1
					channel = next(pathFiles[count])
					continue
				}
			}

			if count < len(pathFiles) && timeStart.After(channel.StartTime.Add(pck.Time+(5*time.Second))) {
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

			timeStream += (pck.Time - prev.Time) / time.Duration(speed)
			timeStart = timeStart.Add(pck.Time - prev.Time)
			time.Sleep((pck.Time - prev.Time) / time.Duration(speed))
			prev = pck

			pck.Time = timeStream

			packet <- pck
		}
	}
	loaded.Data.Close()
	channel.Data.Close()
}

func PlayStreamRTSP(suuid string, ws *websocket.Conn, packet chan *av.Packet, muxer *mp4f.Muxer) {
	statusSocket := make(chan bool)
	go func() {
		for {
			var message string
			err := websocket.Message.Receive(ws, &message)
			if err != nil {
				statusSocket <- false
				ws.Close()
				return
			}
		}
	}()
	interval := time.NewTimer(30 * time.Second)

	//timeStream := time.Duration(0)
	//var prev av.Packet
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

			//if (pck.Time - prev.Time) < 0 {
			//	prev = pck
			//}
			//
			//if prev.Time == 0 {
			//	prev = pck
			//}
			//
			//timeStream += pck.Time - prev.Time
			//prev = pck
			//pck.Time = timeStream

			ready, buf, err := muxer.WritePacket(*pck, false)
			if err != nil {
				Logger.Error(err.Error())
				return
			}
			if ready {
				interval.Reset(30 * time.Second)
				err := ws.SetWriteDeadline(time.Now().Add(60 * time.Second))
				if err != nil {
					Logger.Error(err.Error())
					return
				}
				err = websocket.Message.Send(ws, buf)
				if err != nil {
					Logger.Error(err.Error())
					return
				}
			}
		case <-interval.C:
			if _, ok := Config.Streams[suuid]; !ok {
				return
			}

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
				return
			}
		}
	}()

	for {
		select {
		case codec := <-codecControl:
			InitMuxer(muxer, codec, ws)
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

func PlayStreamArchiveHttp(packet chan av.Packet, statusSocket chan bool, c *gin.Context, muxer *mp4f.Muxer, codecControl chan []av.CodecData) {
	for {
		select {
		case codec := <-codecControl:
			err := InitMuxerHttp(muxer, codec, c)
			if err != nil {
				log.Println(err)
			}
		case pck := <-packet:
			ready, buf, err := muxer.WritePacket(pck, false)
			if err != nil {
				log.Println(err)
			}
			if ready {
				c.Writer.Write(buf)
			}
		case <-statusSocket:
			return
		}
	}
}

func PlayStreamRTSPHTTP(c *gin.Context, packet chan *av.Packet, muxer *mp4f.Muxer) {
	var data = struct {
		Suuid string `form:"uuid" binding:"required"`
	}{}

	if err := c.Bind(&data); !errors.Is(err, nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	interval := time.NewTimer(30 * time.Second)

	//timeStream := time.Duration(0)
	//var prev av.Packet
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

			//if (pck.Time - prev.Time) < 0 {
			//	prev = pck
			//}
			//
			//if prev.Time == 0 {
			//	prev = pck
			//}
			//
			//timeStream += pck.Time - prev.Time
			//prev = pck
			//pck.Time = timeStream

			ready, buf, err := muxer.WritePacket(*pck, false)
			if err != nil {
				Logger.Error(err.Error())
				return
			}
			if ready {
				interval.Reset(30 * time.Second)
				c.Writer.Write(buf)
			}
		case <-interval.C:
			if _, ok := Config.Streams[data.Suuid]; !ok {
				return
			}
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
	if len(codec) <= 0 {
		return errors.New("codec or connect error")
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

func InitMuxerHttp(muxer *mp4f.Muxer, codec []av.CodecData, c *gin.Context) error {
	Config.mutex.Lock()
	defer Config.mutex.Unlock()
	err := muxer.WriteHeader(codec)
	if err != nil {
		Logger.Error(fmt.Sprintf("Muxer: %s", err.Error()))
		return err
	}

	if len(codec) <= 0 {
		return errors.New("codec or connect error")
	}

	_, init := muxer.GetInit(0)

	c.Writer.Write(init)
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
