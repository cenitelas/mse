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
	router.GET("/ws-archive", func(c *gin.Context) {
		handler := websocket.Handler(wsArchive)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/online", func(c *gin.Context) {
		handler := websocket.Handler(online)
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

	c.JSON(http.StatusOK, gin.H{"ws": "ws://127.0.0.1" + Config.Server.HTTPPort + "/online?uuid=" + uuid})

}

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
	var pathFiles []string
	//var length int

	var dur time.Duration
	if st.Duration == 0 {
		pathFiles, _, dur = files(st.Path, start, end, true)
	} else {
		pathFiles, _, _ = files(st.Path, start, end, false)
		dur = time.Duration(st.Duration) * time.Second
	}
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
	//c.Writer.Header().Add("Content-Length", fmt.Sprintf("%d", length))

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

func wsArchive(ws *websocket.Conn) {
	defer ws.Close()
	queryPath := ws.Request().FormValue("path")
	ws.SetWriteDeadline(time.Now().Add(5 * time.Second))

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

	codecs := make(chan []av.CodecData)
	packet := make(chan av.Packet)
	status := make(chan bool)
	go ArchiveWorker(packet, status, paths, start, codecs)
	mux := mp4f.NewMuxer(nil)

	PlayStreamArchive(packet, status, ws, mux, codecs)

}

func online(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetWriteDeadline(time.Now().Add(60 * time.Second))

	suuid := ws.Request().FormValue("uuid")
	log.Println(suuid)
	if suuid == "" {
		ws.Close()
		return
	}

	Config.Run(suuid)
	ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	cuuid, packet, status := Config.clAd(suuid)

	defer Config.clDe(suuid, cuuid)

	codecs := Config.coGe(suuid)
	muxer := mp4f.NewMuxer(nil)
	err := InitMuxer(muxer, codecs, ws)
	if err != nil {
		return
	}

	Logger.Info("Client connect to stream " + suuid)
	PlayStreamRTSP(ws, status, packet, muxer)
	Logger.Info("Client disconnect to stream " + suuid)
}
