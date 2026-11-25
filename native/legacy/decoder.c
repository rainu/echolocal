// SPDX-License-Identifier: AGPL-3.0-or-later
// Isolated FFmpeg demux/decoder. No URLs, credentials, files or network protocols.
// fd 3 requests source ranges; fd 4 supplies them. stdout is bounded ELV1 records.
#include <errno.h>
#include <libavcodec/avcodec.h>
#include <libavcodec/bsf.h>
#include <libavformat/avformat.h>
#include <libavutil/channel_layout.h>
#include <libavutil/intreadwrite.h>
#include <libavutil/opt.h>
#include <libswresample/swresample.h>
#include <libswscale/swscale.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>
#include <unistd.h>

#define RAW 0x41424752U
#define AVC 0x34363248U
static int64_t pos, size, start_us, origin;
static int seekable, vw, vh;
static struct SwsContext *scale;
static SwrContext *resample;
static AVBSFContext *bsf;
static AVCodecContext *vc, *ac;
static AVFormatContext *fmt;
static int vi = -1, ai = -1;
static int64_t next_audio_pts = AV_NOPTS_VALUE;

static int transfer(int fd, void *p, size_t n, int writing) {
    while (n) {
	ssize_t k = writing ? write(fd, p, n) : read(fd, p, n);
	if (k < 0 && errno == EINTR)
	    continue;
	if (k <= 0)
	    return -1;
	p = (char *)p + k;
	n -= k;
    }
    return 0;
}
static void record(uint32_t kind, int64_t pts, const void *data, uint32_t n) {
    uint8_t h[24] = {0};
    AV_WL32(h, 0x31564c45);
    AV_WL32(h + 4, kind);
    AV_WL32(h + 8, n);
    AV_WL64(h + 16, pts);
    if (transfer(1, h, sizeof h, 1) || (n && transfer(1, (void *)data, n, 1)))
	exit(0);
}
static void fail(const char *code) {
    record(4, 0, code, strlen(code));
    exit(1);
}
static int input(void *opaque, uint8_t *buf, int n) {
    (void)opaque;
    uint8_t req[16] = {0}, ans[4];
    if (n > 65536)
	n = 65536;
    AV_WL64(req, pos);
    AV_WL32(req + 8, n);
    if (transfer(3, req, sizeof req, 1) || transfer(4, ans, sizeof ans, 0))
	return AVERROR_EXIT;
    int got = (int32_t)AV_RL32(ans);
    if (got < 0)
	return AVERROR(EIO);
    if (got > n)
	return AVERROR_INVALIDDATA;
    if (!got)
	return AVERROR_EOF;
    if (transfer(4, buf, got, 0))
	return AVERROR_EXIT;
    pos += got;
    return got;
}
static int64_t seek(void *opaque, int64_t off, int whence) {
    (void)opaque;
    if (whence == AVSEEK_SIZE)
	return size > 0 ? size : AVERROR(ENOSYS);
    if (!seekable)
	return AVERROR(ENOSYS);
    whence &= ~AVSEEK_FORCE;
    int64_t base = whence == SEEK_SET	? 0
		   : whence == SEEK_CUR ? pos
		   : whence == SEEK_END ? size
					: -1;
    if (base < 0 || off < -base || (off > 0 && base > INT64_MAX - off))
	return AVERROR(EINVAL);
    pos = base + off;
    return pos;
}
static int64_t timestamp(int64_t pts, int stream) {
    if (pts == AV_NOPTS_VALUE)
	fail("MISSING_TIMESTAMPS");
    return av_rescale_q(pts, fmt->streams[stream]->time_base, AV_TIME_BASE_Q) - origin;
}
static AVCodecContext *decoder(int stream) {
    AVCodecParameters *p = fmt->streams[stream]->codecpar;
    const AVCodec *codec = avcodec_find_decoder(p->codec_id);
    if (!codec)
	fail(p->codec_type == AVMEDIA_TYPE_VIDEO ? "UNSUPPORTED_VIDEO_CODEC"
						 : "UNSUPPORTED_AUDIO_CODEC");
    AVCodecContext *c = avcodec_alloc_context3(codec);
    if (!c || avcodec_parameters_to_context(c, p) < 0)
	fail("DECODER_MEMORY");
    c->thread_count = 2;
    c->max_pixels = 1280 * 720;
    if (avcodec_open2(c, codec, NULL) < 0)
	fail("UNSUPPORTED_CODEC_PROFILE");
    return c;
}
static void frames(AVCodecContext *c, int video, AVPacket *p) {
    if (avcodec_send_packet(c, p) < 0)
	fail("INVALID_COMPRESSED_DATA");
    AVFrame *f = av_frame_alloc();
    if (!f)
	fail("DECODER_MEMORY");
    int err;
    while ((err = avcodec_receive_frame(c, f)) >= 0) {
	int64_t stamp = video ? f->best_effort_timestamp : f->pts;
	int64_t pts;
	if (!video && stamp == AV_NOPTS_VALUE && next_audio_pts != AV_NOPTS_VALUE)
	    pts = next_audio_pts;
	else
	    pts = timestamp(stamp, video ? vi : ai);
	if (!video)
	    next_audio_pts = pts + av_rescale(f->nb_samples, AV_TIME_BASE, f->sample_rate);
	if (video) {
	    if (f->width != c->width || f->height != c->height || f->width < 1 || f->height < 1 ||
		(int64_t)f->width * f->height > 1280 * 720)
		fail("VIDEO_RESOURCE_LIMIT");
	    if (pts < start_us) {
		av_frame_unref(f);
		continue;
	    }
	    scale = sws_getCachedContext(scale, f->width, f->height, f->format, vw, vh,
					 AV_PIX_FMT_RGBA, SWS_BILINEAR, NULL, NULL, NULL);
	    uint8_t *pixels = av_malloc((size_t)vw * vh * 4);
	    if (!scale || !pixels)
		fail("DECODER_MEMORY");
	    uint8_t *out[4] = {pixels};
	    int lines[4] = {vw * 4};
	    if (sws_scale(scale, (const uint8_t *const *)f->data, f->linesize, 0, f->height, out,
			  lines) != vh)
		fail("VIDEO_CONVERSION_FAILED");
	    record(3, pts, pixels, vw * vh * 4);
	    av_free(pixels);
	} else {
	    if (!resample) {
		AVChannelLayout stereo = AV_CHANNEL_LAYOUT_STEREO;
		if (f->sample_rate < 8000 || f->sample_rate > 192000 ||
		    f->ch_layout.nb_channels < 1 || f->ch_layout.nb_channels > 8)
		    fail("AUDIO_RESOURCE_LIMIT");
		if (swr_alloc_set_opts2(&resample, &stereo, AV_SAMPLE_FMT_S16, 48000, &f->ch_layout,
					f->format, f->sample_rate, 0, NULL) < 0 ||
		    swr_init(resample) < 0)
		    fail("AUDIO_CONVERSION_FAILED");
	    }
	    int cap = swr_get_out_samples(resample, f->nb_samples);
	    if (cap < 0 || cap > 65536)
		fail("AUDIO_RESOURCE_LIMIT");
	    uint8_t *pcm = av_malloc((size_t)cap * 4);
	    if (!pcm)
		fail("DECODER_MEMORY");
	    int64_t delay = swr_get_delay(resample, 48000);
	    uint8_t *out[] = {pcm};
	    int n =
		swr_convert(resample, out, cap, (const uint8_t **)f->extended_data, f->nb_samples);
	    if (n < 0)
		fail("AUDIO_CONVERSION_FAILED");
	    pts -= av_rescale(delay, AV_TIME_BASE, 48000);
	    int skip = pts < start_us ? (int)av_rescale(start_us - pts, 48000, AV_TIME_BASE) : 0;
	    if (skip < n)
		record(2, pts + av_rescale(skip, AV_TIME_BASE, 48000), pcm + skip * 4,
		       (n - skip) * 4);
	    av_free(pcm);
	}
	av_frame_unref(f);
    }
    av_frame_free(&f);
    if (err != AVERROR(EAGAIN) && err != AVERROR_EOF)
	fail("DECODE_FAILED");
}
static void compressed(AVPacket *p) {
    if (av_bsf_send_packet(bsf, p) < 0)
	fail("H264_PACKET_FAILED");
    AVPacket *out = av_packet_alloc();
    while (av_bsf_receive_packet(bsf, out) >= 0) {
	record(3,
	       timestamp(out->pts == AV_NOPTS_VALUE && fmt->streams[vi]->codecpar->video_delay == 0
			     ? out->dts
			     : out->pts,
			 vi),
	       out->data, out->size);
	av_packet_unref(out);
    }
    av_packet_free(&out);
}
int main(int argc, char **argv) {
    if (argc != 4)
	return 2;
    size = strtoll(argv[1], NULL, 10);
    seekable = atoi(argv[2]);
    start_us = strtoll(argv[3], NULL, 10);
    if (size < -1 || start_us < 0)
	return 2;
    struct rlimit memory = {512U << 20, 512U << 20}, cores = {0, 0};
    setrlimit(RLIMIT_AS, &memory);
    setrlimit(RLIMIT_CORE, &cores);
    av_log_set_level(AV_LOG_QUIET);
    av_max_alloc(64U << 20);
    fmt = avformat_alloc_context();
    uint8_t *buf = av_malloc(65536);
    if (!fmt || !buf)
	fail("DECODER_MEMORY");
    fmt->pb = avio_alloc_context(buf, 65536, 0, NULL, input, NULL, seek);
    if (!fmt->pb)
	fail("DECODER_MEMORY");
    fmt->pb->seekable = seekable ? AVIO_SEEKABLE_NORMAL : 0;
    fmt->flags |= AVFMT_FLAG_CUSTOM_IO | AVFMT_FLAG_GENPTS;
    fmt->probesize = 4U << 20;
    fmt->max_analyze_duration = 5 * AV_TIME_BASE;
    if (avformat_open_input(&fmt, NULL, NULL, NULL) < 0 || avformat_find_stream_info(fmt, NULL) < 0)
	fail("UNSUPPORTED_OR_DAMAGED_CONTAINER");
    vi = av_find_best_stream(fmt, AVMEDIA_TYPE_VIDEO, -1, -1, NULL, 0);
    ai = av_find_best_stream(fmt, AVMEDIA_TYPE_AUDIO, -1, vi, NULL, 0);
    if (vi < 0)
	fail("NO_USABLE_VIDEO_TRACK");
    AVCodecParameters *v = fmt->streams[vi]->codecpar;
    AVRational rate = fmt->streams[vi]->avg_frame_rate;
    if (!rate.num || !rate.den)
	rate = av_guess_frame_rate(fmt, fmt->streams[vi], NULL);
    int hardware = v->codec_id == AV_CODEC_ID_H264;
    int max_pixels = hardware ? 1920 * 1080 : 1280 * 720;
    if (v->width < 1 || v->height < 1 || (int64_t)v->width * v->height > max_pixels ||
	(rate.den && av_q2d(rate) > (hardware ? 60 : 30.1)))
	fail("VIDEO_RESOURCE_LIMIT");
    vw = v->width;
    vh = v->height;
    if (hardware) {
	const AVBitStreamFilter *filter = av_bsf_get_by_name("h264_mp4toannexb");
	if (!filter || av_bsf_alloc(filter, &bsf) < 0 ||
	    avcodec_parameters_copy(bsf->par_in, v) < 0)
	    fail("H264_PACKET_FAILED");
	bsf->time_base_in = fmt->streams[vi]->time_base;
	if (av_bsf_init(bsf) < 0)
	    fail("H264_PACKET_FAILED");
    } else {
	vc = decoder(vi);
	AVRational sar = av_guess_sample_aspect_ratio(fmt, fmt->streams[vi], NULL);
	double aspect = (double)vw / vh * (sar.num && sar.den ? av_q2d(sar) : 1);
	vh = vh > 480 ? 480 : vh;
	vw = (int)(vh * aspect + 0.5);
	if (vw > 854) {
	    vw = 854;
	    vh = (int)(vw / aspect + 0.5);
	}
	if (vw < 1 || vh < 1)
	    fail("VIDEO_RESOURCE_LIMIT");
    }
    if (ai >= 0)
	ac = decoder(ai);
    origin = fmt->start_time == AV_NOPTS_VALUE ? 0 : fmt->start_time;
    if (start_us &&
	(!seekable || av_seek_frame(fmt, -1, start_us + origin, AVSEEK_FLAG_BACKWARD) < 0))
	fail("SEEK_UNSUPPORTED");
    uint8_t meta[24];
    AV_WL32(meta, vw);
    AV_WL32(meta + 4, vh);
    AV_WL32(meta + 8, hardware ? AVC : RAW);
    AV_WL32(meta + 12, ai >= 0);
    AV_WL64(meta + 16, fmt->duration == AV_NOPTS_VALUE ? 0 : fmt->duration);
    record(1, 0, meta, sizeof meta);
    AVPacket *p = av_packet_alloc();
    if (!p)
	fail("DECODER_MEMORY");
    int err;
    while ((err = av_read_frame(fmt, p)) >= 0) {
	if (p->stream_index == vi) {
	    if (hardware)
		compressed(p);
	    else
		frames(vc, 1, p);
	} else if (p->stream_index == ai)
	    frames(ac, 0, p);
	av_packet_unref(p);
    }
    if (err != AVERROR_EOF)
	fail("MEDIA_READ_FAILED");
    if (hardware)
	compressed(NULL);
    else
	frames(vc, 1, NULL);
    if (ac)
	frames(ac, 0, NULL);
    if (resample) {
	uint8_t tail[16384], *out[] = {tail};
	int n = swr_convert(resample, out, 4096, NULL, 0);
	// Tail carries no independent timestamp: the Go timeline appends it.
	if (n > 0)
	    record(2, INT64_MIN, tail, n * 4);
    }
    record(5, 0, NULL, 0);
    av_packet_free(&p);
    avcodec_free_context(&vc);
    avcodec_free_context(&ac);
    av_bsf_free(&bsf);
    swr_free(&resample);
    sws_freeContext(scale);
    AVIOContext *io = fmt->pb;
    avformat_close_input(&fmt);
    av_freep(&io->buffer);
    avio_context_free(&io);
    return 0;
}
