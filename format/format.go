package format

import (
	"mse/av/avutil"
	"mse/format/aac"
	"mse/format/flv"
	"mse/format/mp4"
	"mse/format/rtmp"
	"mse/format/rtsp"
	"mse/format/ts"
)

func RegisterAll() {
	avutil.DefaultHandlers.Add(mp4.Handler)
	avutil.DefaultHandlers.Add(ts.Handler)
	avutil.DefaultHandlers.Add(rtmp.Handler)
	avutil.DefaultHandlers.Add(rtsp.Handler)
	avutil.DefaultHandlers.Add(flv.Handler)
	avutil.DefaultHandlers.Add(aac.Handler)
}
