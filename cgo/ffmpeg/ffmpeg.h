
#include <libavformat/avformat.h>
#include <libavcodec/avcodec.h>
#include <libavutil/avutil.h>
#include <libavresample/avresample.h>
#include <libavutil/opt.h>
#include <string.h>
#include <libswscale/swscale.h>


typedef struct {
	AVCodec *codec;
	AVCodecContext *codecCtx;
	AVFrame *frame;
	AVDictionary *options;
	int profile;
} FFCtx;

static inline int avcodec_profile_name_to_int(AVCodec *codec, const char *name) {
	const AVProfile *p;
	for (p = codec->profiles; p != NULL && p->profile != FF_PROFILE_UNKNOWN; p++)
		if (!strcasecmp(p->name, name))
			return p->profile;
	return FF_PROFILE_UNKNOWN;
}

static int64_t file_duration(const char *file_name) {
      AVFormatContext* pFormatCtx = avformat_alloc_context();
      avformat_open_input(&pFormatCtx, file_name, NULL, NULL);
      avformat_find_stream_info(pFormatCtx,NULL);
      int64_t duration = pFormatCtx->duration;
      avformat_close_input(&pFormatCtx);
      avformat_free_context(pFormatCtx);
      return duration>0 ? duration/AV_TIME_BASE : 0;
}
