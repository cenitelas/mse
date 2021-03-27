package main

import (
	"errors"
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
	"log"
	"mse/av"
	"mse/av/avutil"
	"mse/format/mp4f"
	"net/http"
	"path"
	"strings"
	"time"
)

func serveHTTP() {
	router := gin.Default()
	gin.SetMode(gin.DebugMode)
	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"*"},
		AllowHeaders: []string{"*"},
	}))
	router.POST("/player", player)
	//router.GET("/ws/:uuid", func(c *gin.Context) {
	//	handler := websocket.Handler(ws)
	//	handler.ServeHTTP(c.Writer, c.Request)
	//})
	router.GET("/ws-archive", func(c *gin.Context) {
		handler := websocket.Handler(wsMP4)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/test", func(c *gin.Context) {
		handler := websocket.Handler(test)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/archive", archive)
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

	uuid := Config.PushStream(rtsp.Rtsp)

	c.JSON(http.StatusOK, gin.H{"ws": "ws://127.0.0.1" + Config.Server.HTTPPort + "/test?uuid=" + uuid})

}

//func ws(ws *websocket.Conn) {
//	defer ws.Close()
//	suuid := ws.Request().FormValue("uuid")
//	log.Println("Request", suuid)
//	if !Config.ext(suuid) {
//		log.Println("Stream Not Found")
//		return
//	}
//	Config.RunIFNotRun(suuid)
//	ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
//	cuuid, ch := Config.clAd(suuid)
//
//	defer Config.clDe(suuid, cuuid)
//	codecs := Config.coGe(suuid)
//	if codecs == nil {
//		log.Println("Codecs Error")
//		return
//	}
//	for i, codec := range codecs {
//		if codec.Type().IsAudio() && codec.Type() != av.AAC {
//			log.Println("Track", i, "Audio Codec Work Only AAC")
//		}
//	}
//	muxer := mp4f.NewMuxer(nil)
//	err := muxer.WriteHeader(codecs)
//	if err != nil {
//		log.Println("muxer.WriteHeader", err)
//		return
//	}
//	meta, init := muxer.GetInit(0)
//	err = websocket.Message.Send(ws, append([]byte{9}, meta...))
//	if err != nil {
//		log.Println("websocket.Message.Send", err)
//		return
//	}
//	err = websocket.Message.Send(ws, init)
//	if err != nil {
//		return
//	}
//	var start bool
//	go func() {
//		for {
//			var message string
//			err := websocket.Message.Receive(ws, &message)
//			if err != nil {
//				ws.Close()
//				return
//			}
//		}
//	}()
//	noVideo := time.NewTimer(10 * time.Second)
//	var timeLine = make(map[int8]time.Duration)
//	for {
//		select {
//		case <-noVideo.C:
//			log.Println("noVideo")
//			return
//		case pck := <-ch:
//			if pck.IsKeyFrame {
//				noVideo.Reset(10 * time.Second)
//				start = true
//			}
//			if !start {
//				continue
//			}
//			timeLine[pck.Idx] += pck.Duration
//			pck.Time = timeLine[pck.Idx]
//			ready, buf, _ := muxer.WritePacket(pck, false)
//			if ready {
//				err = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
//				if err != nil {
//					return
//				}
//				err := websocket.Message.Send(ws, buf)
//				if err != nil {
//					return
//				}
//			}
//		}
//	}
//}

func archive(c *gin.Context) {

	var st = struct {
		Path     []string `form:"path[]" binding:"required"`
		Start    string   `form:"start" binding:"required"`
		End      string   `form:"end" binding:"required"`
		Ext      string   `form:"ext" binding:"required"`
		Name     string   `form:"name"`
		Duration int32    `form:"duration"`
	}{
		Name:     "video",
		Duration: 0,
	}
	if err := c.Bind(&st); !errors.Is(err, nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start, err := time.Parse("2006-01-02 15:04:05", st.Start)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	end, err := time.Parse("2006-01-02 15:04:05", st.End)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pathFiles, length := files(st.Path, start, end)
	if len(pathFiles) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not found"})
		return
	}

	format := "mp4"
	if st.Ext == "avi" {
		format = "x-msvideo"
	}

	c.Writer.Header().Add("Content-type", fmt.Sprintf("video/%s", format))
	c.Writer.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.%s\"", st.Name, st.Ext))
	c.Writer.Header().Add("Content-Length", fmt.Sprintf("%d", length))
	var dur time.Duration
	if st.Duration == 0 {
		dur, err = durations(pathFiles)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
		dur = time.Duration(st.Duration) * time.Second
	}

	loaded, _ := avutil.Open(pathFiles[0])
	streams, _ := loaded.Streams()
	loaded.Close()

	muxer := mp4f.NewMuxer(nil)
	err = muxer.WriteHeader(streams)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, init := muxer.GetInit(int32(dur.Milliseconds()))
	c.Writer.Write(init)

	p := make(chan av.Packet)
	cls := make(chan bool)

	go send(p, pathFiles, cls, start, end)

	for {
		select {
		case pck := <-p:
			ready, buf, err := muxer.WritePacket(pck, false)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if ready {
				c.Writer.Write(buf)
			}
		case f := <-cls:
			if f {
				return
			}

		}
	}
}

func wsMP4(ws *websocket.Conn) {
	defer ws.Close()
	queryPath := ws.Request().FormValue("path")

	if len(queryPath) == 0 {
		log.Fatal("Path required")
		return
	}

	paths := strings.Split(queryPath, ",")

	queryStart := ws.Request().FormValue("start")
	if queryStart == "" {
		log.Print("Start required")
		return
	}

	start, err := time.Parse("2006-01-02 15:04:05", queryStart)
	if err != nil {
		log.Print(err)
		return
	}

	pathFiles := filesStream(paths, start)

	final := make(chan bool)

	go func() {
		for {
			var message string
			err := websocket.Message.Receive(ws, &message)
			if err != nil {
				final <- true
				ws.Close()
				return
			}
		}
	}()

	c := make(chan av.Packet)

	newStream := make(chan []av.CodecData)

	mux := mp4f.NewMuxer(nil)

	go stream(c, pathFiles, start, final, newStream)

	for {
		select {
		case pck := <-c:
			ready, buf, errr := mux.WritePacket(pck, false)
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
		case newS := <-newStream:
			err = mux.WriteHeader(newS)
			if err != nil {
				log.Fatal("muxer.WriteHeader", err)
				return
			}
			meta, init := mux.GetInit(0)

			err = websocket.Message.Send(ws, append([]byte{9}, meta...))
			if err != nil {
				log.Fatal("websocket.Message.Send", err)
				return
			}
			err = websocket.Message.Send(ws, init)
			if err != nil {
				return
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

type Vidos struct {
	Data      av.DemuxCloser
	Streams   []av.CodecData
	StartTime time.Time
	Size      int64
}

func stream(p chan av.Packet, paths []string, timeStart time.Time, final chan bool, newStream chan []av.CodecData) {

	files := make(map[string]Vidos)
	for _, pathFile := range paths {
		in, _ := avutil.Open(pathFile)
		st, _ := in.Streams()
		_, fileName := path.Split(pathFile)
		startTime, _ := time.Parse("2006-01-02T15-04-05", strings.ReplaceAll(fileName, ".flv", ""))
		v := Vidos{
			Data:      in,
			Streams:   st,
			StartTime: startTime,
		}
		files[pathFile] = v
	}
	keyframe := false
	var timeStream time.Duration
	var err error
	infile, _ := avutil.Open("test.flv")
	stream, _ := infile.Streams()
	loaded := Vidos{
		Data:    infile,
		Streams: stream,
	}
	loader := false
	count := 0
	iteration := 0
	var prev av.Packet
	for {
		select {
		case f := <-final:
			if f {
				loaded.Data.Close()
				for _, pathFile := range paths {
					files[pathFile].Data.Close()
				}
				log.Println("CLOSE")
				return
			}
		default:
			var pck av.Packet
			if timeStart.Add(5*time.Second).Before(files[paths[count]].StartTime) || count > len(paths)-1 {
				if loader == false {
					iteration = 0
					loader = true
				}
				if pck, err = loaded.Data.ReadPacket(); err != nil {
					prev.Time = 0
					loaded.Data.Close()
					infile, _ := avutil.Open("test.flv")
					stream, _ := infile.Streams()
					loaded = Vidos{
						Data:    infile,
						Streams: stream,
					}
					continue
				}
			} else {
				if loader == true {
					loader = false
					iteration = 0
				}
				if pck, err = files[paths[count]].Data.ReadPacket(); err != nil {
					files[paths[count]].Data.Close()
					prev.Time = 0
					count += 1
					iteration = 0
					continue
				}
			}

			if iteration == 0 {
				if loader {
					newStream <- loaded.Streams
				} else {
					newStream <- files[paths[count]].Streams
				}
			}
			iteration++

			if timeStart.After(files[paths[count]].StartTime.Add(pck.Time + (5 * time.Second))) {
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

			p <- pck
		}
	}
	loaded.Data.Close()
}

func test(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetWriteDeadline(time.Now().Add(60 * time.Second))

	suuid := ws.Request().FormValue("uuid")
	log.Println(suuid)
	if suuid == "" {
		ws.Close()
		return
	}

	stream := Config.Streams[suuid]
	if stream == nil {
		ws.Close()
		return
	}
	log.Println(stream.Status)
	if stream.Status != "connect" {
		<-stream.StatusC
	}
	log.Println(stream.Status)
	muxer := mp4f.NewMuxer(nil)
	err := muxer.WriteHeader(stream.Codecs)
	if err != nil {
		log.Println("muxer.WriteHeader", err)
		return
	}
	meta, init := muxer.GetInit(0)
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
	interval := time.NewTimer(10 * time.Second)
	reconnectFun := func() {
		interval.Reset(10 * time.Second)
		stream.SetStatus("reconnect")
	}

	timeStream := time.Duration(0)
	var prev av.Packet
	keyframe := false
	for {
		select {
		case pck := <-stream.Cl:

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
				log.Println("CHECK " + timeStream.String())
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

		case status := <-stream.StatusC:
			if status == "error" {
				ws.Close()
				return
			}
		}
	}
}
