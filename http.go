package main

import (
	"errors"
	"github.com/deepch/vdk/av/avutil"
	"github.com/deepch/vdk/format/flv"
	"log"
	"net/http"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/format/mp4f"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

func serveHTTP() {
	MP4Worker()
	router := gin.Default()
	gin.SetMode(gin.DebugMode)
	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"*"},
		AllowHeaders: []string{"*"},
	}))
	router.POST("/player", player)
	router.GET("/ws/:uuid", func(c *gin.Context) {
		handler := websocket.Handler(ws)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/wsMP4", func(c *gin.Context) {
		handler := websocket.Handler(wsMP4)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/test", archive)
	err := router.Run(Config.Server.HTTPPort)
	if err != nil {
		log.Fatalln(err)
	}
}

func player(c *gin.Context) {
	var rtsp = struct {
		Rtsp string `json:"rtsp"`
	}{}

	if err := c.Bind(&rtsp); !errors.Is(err, nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	uuid := Config.StreamExists(rtsp.Rtsp)

	if uuid != nil {
		c.JSON(http.StatusOK, gin.H{"ws": "ws://127.0.0.1" + Config.Server.HTTPPort + "/ws/live?uuid=" + *uuid})
	} else {
		uuid := Config.PushStream(rtsp.Rtsp)
		log.Println(uuid)
		c.JSON(http.StatusOK, gin.H{"ws": "ws://127.0.0.1" + Config.Server.HTTPPort + "/ws/live?uuid=" + uuid})
	}

}

func ws(ws *websocket.Conn) {
	defer ws.Close()
	suuid := ws.Request().FormValue("uuid")
	log.Println("Request", suuid)
	if !Config.ext(suuid) {
		log.Println("Stream Not Found")
		return
	}
	Config.RunIFNotRun(suuid)
	ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	cuuid, ch := Config.clAd(suuid)

	defer Config.clDe(suuid, cuuid)
	codecs := Config.coGe(suuid)
	if codecs == nil {
		log.Println("Codecs Error")
		return
	}
	for i, codec := range codecs {
		if codec.Type().IsAudio() && codec.Type() != av.AAC {
			log.Println("Track", i, "Audio Codec Work Only AAC")
		}
	}
	muxer := mp4f.NewMuxer(nil)
	err := muxer.WriteHeader(codecs)
	if err != nil {
		log.Println("muxer.WriteHeader", err)
		return
	}
	meta, init := muxer.GetInit(codecs)
	err = websocket.Message.Send(ws, append([]byte{9}, meta...))
	if err != nil {
		log.Println("websocket.Message.Send", err)
		return
	}
	err = websocket.Message.Send(ws, init)
	if err != nil {
		return
	}
	var start bool
	go func() {
		for {
			var message string
			err := websocket.Message.Receive(ws, &message)
			if err != nil {
				ws.Close()
				return
			}
		}
	}()
	noVideo := time.NewTimer(10 * time.Second)
	var timeLine = make(map[int8]time.Duration)
	for {
		select {
		case <-noVideo.C:
			log.Println("noVideo")
			return
		case pck := <-ch:
			if pck.IsKeyFrame {
				noVideo.Reset(10 * time.Second)
				start = true
			}
			if !start {
				continue
			}
			timeLine[pck.Idx] += pck.Duration
			pck.Time = timeLine[pck.Idx]
			ready, buf, _ := muxer.WritePacket(pck, false)
			if ready {
				err = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err != nil {
					return
				}
				err := websocket.Message.Send(ws, buf)
				if err != nil {
					return
				}
			}
		}
	}
}

func archive(c *gin.Context) {
	c.Writer.Header().Add("Content-type", "application/octet-stream")
	path := []string{"1.flv"}
	infile, _ := avutil.Open(path[0])
	streams, _ := infile.Streams()
	infile.Close()
	muxer := mp4f.NewMuxer(nil)
	err := muxer.WriteHeader(streams)

	if err != nil {
		log.Println("muxer.WriteHeader", err)
		return
	}
	_, init := muxer.GetInit(streams)
	c.Writer.Write(init)
	p := make(chan av.Packet)
	final := make(chan bool)

	go sendFile(p, path, final)

	for {
		select {
		case pck := <-p:
			ready, buf, errr := muxer.WritePacket(pck, false)
			if errr != nil {
				log.Println(errr)
			}
			if ready {
				c.Writer.Write(buf)
			}
		case f := <-final:
			if f {
				return
			}

		}
	}
}

func wsMP4(ws *websocket.Conn) {
	defer ws.Close()

	path := []string{"1.flv"}
	infile, _ := avutil.Open(path[0])
	streams, _ := infile.Streams()
	infile.Close()
	muxer := mp4f.NewMuxer(nil)
	err := muxer.WriteHeader(streams)
	if err != nil {
		log.Println("muxer.WriteHeader", err)
		return
	}
	meta, init := muxer.GetInit(streams)
	log.Println(meta)
	err = websocket.Message.Send(ws, append([]byte{9}, meta...))
	if err != nil {
		log.Println("websocket.Message.Send", err)
		return
	}
	err = websocket.Message.Send(ws, init)
	if err != nil {
		return
	}

	go func() {
		for {
			var message string
			err := websocket.Message.Receive(ws, &message)
			if err != nil {
				ws.Close()
				return
			}
		}
	}()

	c := make(chan av.Packet)

	final := make(chan bool)

	go stream(c, path, time.Duration(370*time.Second), final)

	for {
		select {
		case pck := <-c:
			ready, buf, errr := muxer.WritePacket(pck, false)
			if errr != nil {
				log.Println(errr)
			}
			if ready {
				err := ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
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
		case f := <-final:
			if f {
				log.Println("CLOSE")
				ws.Close()
				return
			}
		}
	}
}

func stream(p chan av.Packet, paths []string, timeStart time.Duration, final chan bool) {
	files := make(map[string]av.DemuxCloser)
	for _, path := range paths {
		infile, _ := avutil.Open(path)
		files[path] = infile
	}
	start := false
	streams, _ := files[paths[0]].Streams()
	var err error
	var totalTime time.Duration
	count := 0
	for {
		var pck av.Packet

		if pck, err = files[paths[count]].ReadPacket(); err != nil {
			log.Println("final", err)
			if count < len(paths)-1 {
				count += 1
				continue
			} else {
				break
			}
		}

		if pck.IsKeyFrame {
			start = true
		}
		if !start {
			continue
		}

		if count == 0 {
			_, t := flv.PacketToTag(pck, streams[pck.Idx])
			if timeStart > time.Duration(t)*time.Millisecond {
				continue
			}
		} else {

		}
		totalTime += 40 * time.Millisecond
		pck.Time = totalTime
		if start {
			p <- pck
			time.Sleep(40 * time.Millisecond)
		}
	}
}

func sendFile(p chan av.Packet, paths []string, final chan bool) {
	files := make(map[string]av.DemuxCloser)
	for _, path := range paths {
		infile, _ := avutil.Open(path)
		files[path] = infile
	}

	start := false
	var err error
	count := 0
	var totalTime time.Duration
	for {
		var pck av.Packet

		if pck, err = files[paths[count]].ReadPacket(); err != nil {
			log.Println("final", err)
			if count < len(paths)-1 {
				count += 1
				continue
			} else {
				break
			}
		}

		if pck.IsKeyFrame {
			start = true
		}
		if !start {
			continue
		}
		if start {
			pck.Time = totalTime
			p <- pck
			totalTime += 40 * time.Millisecond
		}
	}
	for _, path := range paths {
		files[path].Close()
	}
	final <- true
}
