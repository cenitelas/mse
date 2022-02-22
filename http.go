package main

import (
	"errors"
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
	"strconv"
	"strings"

	//"github.com/gorilla/websocket"
	"log"
	"mse/av"
	"mse/av/avutil"
	"mse/format/mp4f"
	"net/http"
	"time"
)

//var upGrader = websocket.Upgrader{
//	CheckOrigin: func(r *http.Request) bool {
//		return true
//	},
//}

func serveHTTP() {
	router := gin.Default()
	gin.SetMode(gin.DebugMode)
	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"*"},
		AllowHeaders: []string{"*"},
	}))
	router.POST("/player", player)
	router.POST("/remove", remove)
	router.GET("/ws-archive", func(c *gin.Context) {
		handler := websocket.Handler(wsArchive)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/online", func(c *gin.Context) {
		handler := websocket.Handler(online)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	router.GET("/online-http", onlineHttp)
	router.GET("/archive-http", httpArchive)
	router.GET("/archive", archive)
	err := router.Run(Config.Server.HTTPPort)
	if err != nil {
		log.Fatalln(err)
	}
}
func player(c *gin.Context) {
	var rtsp = struct {
		Rtsp string `json:"rtsp"`
		Rtmp string `json:"rtmp"`
	}{}

	if err := c.Bind(&rtsp); !errors.Is(err, nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	uRtsp := ""
	if len(rtsp.Rtsp) > 0 {
		uRtsp = Config.PushStream(rtsp.Rtsp, "rtsp")
	}
	uRtmp := ""
	if len(rtsp.Rtmp) > 0 {
		uRtmp = Config.PushStream(rtsp.Rtmp, "rtmp")
	}

	if len(uRtsp) > 0 && len(uRtmp) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"ws":        "ws://127.0.0.1" + Config.Server.HTTPPort + "/online?uuid=" + uRtsp,
			"http":      "http://127.0.0.1" + Config.Server.HTTPPort + "/online-http?uuid=" + uRtsp,
			"ws-rtmp":   "ws://127.0.0.1" + Config.Server.HTTPPort + "/online?uuid=" + uRtmp,
			"http-rtmp": "http://127.0.0.1" + Config.Server.HTTPPort + "/online-http?uuid=" + uRtmp,
		})
		return
	}

	if len(uRtsp) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"ws":   "ws://127.0.0.1" + Config.Server.HTTPPort + "/online?uuid=" + uRtsp,
			"http": "http://127.0.0.1" + Config.Server.HTTPPort + "/online-http?uuid=" + uRtsp,
		})
	}

	if len(uRtmp) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"ws-rtmp":   "ws://127.0.0.1" + Config.Server.HTTPPort + "/online?uuid=" + uRtmp,
			"http-rtmp": "http://127.0.0.1" + Config.Server.HTTPPort + "/online-http?uuid=" + uRtmp,
		})
	}

}
func remove(c *gin.Context) {
	var rtsp = struct {
		Rtsp string `json:"rtsp" binding:"required"`
	}{}

	if err := c.Bind(&rtsp); !errors.Is(err, nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	if Config.RemoveStream(rtsp.Rtsp) {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	} else {
		c.JSON(http.StatusOK, gin.H{"status": "error"})
	}
}
func archive(c *gin.Context) {
	var st = struct {
		Path      []string `form:"path[]" binding:"required"`
		Start     string   `form:"start" binding:"required"`
		End       string   `form:"end" binding:"required"`
		Ext       string   `form:"ext" binding:"required"`
		Name      string   `form:"name"`
		Duration  int32    `form:"duration"`
		FileStart string   `form:"file_start"`
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
		pathFiles, _, dur = files(st.Path, start, end, st.FileStart, true)
	} else {
		pathFiles, _, _ = files(st.Path, start, end, st.FileStart, false)
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
	//c.Writer.Header().Add("Video-Length", fmt.Sprintf("%d", length))

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
	queryPath := ws.Request().FormValue("path")
	if len(queryPath) == 0 {
		Logger.Error("Path required")
		return
	}
	paths := strings.Split(queryPath, ",")

	queryStart := ws.Request().FormValue("start")
	if queryStart == "" {
		Logger.Error("Start required")
		return
	}

	querySpeed, err := strconv.Atoi(ws.Request().FormValue("speed"))
	if err != nil {
		querySpeed = 1
	}

	queryFileStart := ws.Request().FormValue("file_start")

	start, err := time.Parse("2006-01-02 15:04:05", queryStart)
	if err != nil {
		Logger.Error(err.Error())
		return
	}
	defer ws.Close()
	err = ws.SetWriteDeadline(time.Now().Add(60 * time.Second))
	if err != nil {
		Logger.Error(err.Error())
		return
	}
	codecs := make(chan []av.CodecData)
	packet := make(chan av.Packet)
	status := make(chan bool)
	go ArchiveWorker(packet, status, paths, start, codecs, queryFileStart, querySpeed)
	mux := mp4f.NewMuxer(nil)

	PlayStreamArchive(packet, status, ws, mux, codecs)
	Logger.Success(fmt.Sprintf("Close stream %s", start.String()))

}
func httpArchive(c *gin.Context) {
	var st = struct {
		Path      []string `form:"path[]" binding:"required"`
		Start     string   `form:"start" binding:"required"`
		Speed     int      `form:"speed"`
		FileStart string   `form:"file_start"`
	}{Speed: 1}
	if err := c.Bind(&st); !errors.Is(err, nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start, err := time.Parse("2006-01-02 15:04:05", st.Start)
	if err != nil {
		log.Print(err)
		return
	}

	codecs := make(chan []av.CodecData)
	packet := make(chan av.Packet)
	status := make(chan bool)
	go ArchiveWorker(packet, status, st.Path, start, codecs, st.FileStart, st.Speed)
	mux := mp4f.NewMuxer(nil)

	PlayStreamArchiveHttp(packet, status, c, mux, codecs)
	Logger.Success(fmt.Sprintf("Close stream %s", start.String()))

}
func online(ws *websocket.Conn) {
	suuid := ws.Request().URL.Query().Get("uuid")
	defer Config.connectDecrease(suuid)
	defer ws.Close()
	err := ws.SetWriteDeadline(time.Now().Add(60 * time.Second))
	if err != nil {
		Logger.Error(err.Error())
		return
	}
	if !Config.ext(suuid) {
		Logger.Error(fmt.Sprintf("Stream %s not exists", suuid))
		ws.Close()
		return
	}
	Config.connectIncrease(suuid)
	Config.Run(suuid)
	cuuid, packet := Config.clAd(suuid)
	defer Config.clDe(suuid, cuuid)
	codecs := Config.coGe(suuid)
	muxer := mp4f.NewMuxer(nil)
	err = InitMuxer(muxer, codecs, ws)
	if err != nil {
		Logger.Error(err.Error())
		Config.connectDecrease(suuid)
		return
	}
	Logger.Info("Client connect to stream " + suuid)
	PlayStreamRTSP(suuid, ws, packet, muxer)
	Logger.Info("Client disconnect to stream " + suuid)
}
func onlineHttp(c *gin.Context) {
	var data = struct {
		Suuid string `form:"uuid" binding:"required"`
	}{}
	defer Config.connectDecrease(data.Suuid)
	if err := c.Bind(&data); !errors.Is(err, nil) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !Config.ext(data.Suuid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Stream not exists"})
		return
	}
	Config.connectIncrease(data.Suuid)
	Config.Run(data.Suuid)
	cuuid, packet := Config.clAd(data.Suuid)
	defer Config.clDe(data.Suuid, cuuid)

	codecs := Config.coGe(data.Suuid)
	muxer := mp4f.NewMuxer(nil)
	InitMuxerHttp(muxer, codecs, c)
	Logger.Info("Client connect to stream " + data.Suuid)
	PlayStreamRTSPHTTP(c, packet, muxer)
	Logger.Info("Client disconnect to stream " + data.Suuid)
}
