// Video-only, local MP4/H.264 background. FFmpeg is the maintained demuxer and
// software decoder; this path never opens audio, device outputs or URLs.
#pragma once
extern "C" {
#include <libavformat/avformat.h>
#include <libavcodec/avcodec.h>
#include <libswscale/swscale.h>
#include <libavutil/display.h>
}
#include <atomic>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <cstdint>
#include <chrono>
#include <condition_variable>
#include <mutex>
#include <thread>
#include <sys/stat.h>
#include <unistd.h>
#include <cerrno>
#include <stdexcept>
#include <string>
#include <vector>

struct VideoDecoder {
 FILE *file=nullptr; AVFormatContext *format=nullptr; AVIOContext *io=nullptr;
 AVCodecContext *codec=nullptr; AVPacket *packet=nullptr; AVFrame *frame=nullptr;
 SwsContext *scale=nullptr; int stream=-1,width=0,height=0,sourceWidth=0,sourceHeight=0; int64_t size=0;
 double duration=0, first=-1, previous=-1; bool draining=false;
 std::atomic<bool> *cancel=nullptr;
 std::chrono::steady_clock::time_point deadline;
 static double clock(){return std::chrono::duration<double>(std::chrono::steady_clock::now().time_since_epoch()).count();}
 static void check(bool ok){if(!ok)throw std::runtime_error("background video unsupported or damaged");}
 static int interrupt(void *p){auto &d=*static_cast<VideoDecoder*>(p);return (d.cancel && d.cancel->load()) || std::chrono::steady_clock::now()>d.deadline;}
 static int read(void *p,uint8_t *dst,int n){auto &d=*static_cast<VideoDecoder*>(p);if(interrupt(p))return AVERROR_EXIT;size_t got=fread(dst,1,size_t(n),d.file);return got?int(got):AVERROR_EOF;}
 static int64_t seek(void *p,int64_t off,int whence){auto &d=*static_cast<VideoDecoder*>(p);if(whence==AVSEEK_SIZE)return d.size;whence&=~AVSEEK_FORCE;if(whence!=SEEK_SET && whence!=SEEK_CUR && whence!=SEEK_END)return AVERROR(EINVAL);if(fseeko(d.file,off,whence))return AVERROR(EINVAL);return ftello(d.file);}
 void limit(){deadline=std::chrono::steady_clock::now()+std::chrono::seconds(3);}
 void open(const char *name,std::atomic<bool> *stop=nullptr){
  cancel=stop;limit();av_log_set_level(AV_LOG_QUIET);
  file=fopen(name,"rb");check(file);struct stat st{};check(!fstat(fileno(file),&st) && S_ISREG(st.st_mode));size=st.st_size;
  check(size>=16 && size<=(64<<20));
  unsigned char header[12]{};check(fread(header,1,12,file)==12 && !memcmp(header+4,"ftyp",4));rewind(file);
  io=avio_alloc_context(static_cast<unsigned char*>(av_malloc(32768)),32768,0,this,read,nullptr,seek);check(io && io->buffer);
  format=avformat_alloc_context();check(format);format->pb=io;format->flags|=AVFMT_FLAG_CUSTOM_IO;
  format->interrupt_callback={interrupt,this};format->probesize=1<<20;format->max_analyze_duration=2000000;format->max_streams=8;
  AVDictionary *opts=nullptr;av_dict_set(&opts,"enable_drefs","0",0);av_dict_set(&opts,"use_absolute_path","0",0);
  int result=avformat_open_input(&format,nullptr,av_find_input_format("mov"),&opts);av_dict_free(&opts);check(result>=0);
  check(format->nb_streams<=8);
  for(unsigned i=0;i<format->nb_streams;i++){
   format->streams[i]->discard=AVDISCARD_ALL;
   if(format->streams[i]->codecpar->codec_type==AVMEDIA_TYPE_VIDEO){check(stream<0);stream=int(i);}
  }
  check(stream>=0);AVStream *s=format->streams[stream];auto *p=s->codecpar;
  check(p->codec_id==AV_CODEC_ID_H264 && p->width>0 && p->height>0 && p->width<=1280 && p->height<=800);
  AVRational sar=av_guess_sample_aspect_ratio(format,s,nullptr);
  const double pixelAspect=sar.num>0 && sar.den>0?av_q2d(sar):1.;
  check(std::isfinite(pixelAspect) && pixelAspect>=.125 && pixelAspect<=8.);
  for(int i=0;i<p->nb_coded_side_data;i++)if(p->coded_side_data[i].type==AV_PKT_DATA_DISPLAYMATRIX){
   check(p->coded_side_data[i].size>=9*sizeof(int32_t));
   check(std::abs(av_display_rotation_get(reinterpret_cast<int32_t*>(p->coded_side_data[i].data)))<.01);
  }
  AVRational fps=s->avg_frame_rate;check(fps.num>0 && fps.den>0 && av_q2d(fps)<=30.01);
  check(s->duration!=AV_NOPTS_VALUE && s->duration>0);duration=s->duration*av_q2d(s->time_base);
  check(duration>0 && duration<=120.001);
  codec=avcodec_alloc_context3(avcodec_find_decoder(AV_CODEC_ID_H264));check(codec);
  check(avcodec_parameters_to_context(codec,p)>=0);codec->thread_count=2;codec->max_pixels=1280*800;
  codec->err_recognition=AV_EF_EXPLODE;check(avcodec_open2(codec,avcodec_find_decoder(AV_CODEC_ID_H264),nullptr)>=0);
  sourceWidth=p->width;sourceHeight=p->height;
  // Normalize non-square pixels into a bounded square-pixel texture, without
  // upscaling beyond the encoded dimensions or changing the display aspect.
  width=sourceWidth;height=sourceHeight;
  if(pixelAspect>=1.)height=std::max(1,int(std::lround(height/pixelAspect)));
  else width=std::max(1,int(std::lround(width*pixelAspect)));
  s->discard=AVDISCARD_DEFAULT;
  packet=av_packet_alloc();frame=av_frame_alloc();check(packet && frame);
 }
 // Decode only one bounded picture ahead. Audio packets are discarded unopened.
 bool next(std::vector<unsigned char> &pixels,double &pts){
  limit();
  for(int count=0;count<512;count++){
   check(!interrupt(this));int result=avcodec_receive_frame(codec,frame);
   if(result>=0){
    check(frame->width==sourceWidth && frame->height==sourceHeight && frame->format==AV_PIX_FMT_YUV420P && !(frame->flags&AV_FRAME_FLAG_CORRUPT));
    check(frame->best_effort_timestamp!=AV_NOPTS_VALUE);
    double raw=frame->best_effort_timestamp*av_q2d(format->streams[stream]->time_base);
    if(first<0)first=raw;pts=raw-first;
    check(pts>=0 && pts<=duration+.05 && (previous<0 || pts>previous));previous=pts;
    scale=sws_getCachedContext(scale,sourceWidth,sourceHeight,static_cast<AVPixelFormat>(frame->format),width,height,AV_PIX_FMT_RGBA,SWS_BILINEAR,nullptr,nullptr,nullptr);check(scale);
    // Respect H.264 limited/full range and the stream's color matrix.
    int colors=frame->colorspace==AVCOL_SPC_BT709?SWS_CS_ITU709:SWS_CS_ITU601;
    const int *coeff=sws_getCoefficients(colors);sws_setColorspaceDetails(scale,coeff,frame->color_range==AVCOL_RANGE_JPEG,coeff,1,0,1<<16,1<<16);
    pixels.resize(size_t(width)*height*4);uint8_t *out[4]={pixels.data()};int lines[4]={width*4};
    check(sws_scale(scale,frame->data,frame->linesize,0,sourceHeight,out,lines)==height);av_frame_unref(frame);return true;
   }
   if(result==AVERROR_EOF)return false;
   check(result==AVERROR(EAGAIN) && !draining);
   bool sent=false;
   while(!sent){
    check(!interrupt(this));result=av_read_frame(format,packet);
    if(result==AVERROR_EOF){check(avcodec_send_packet(codec,nullptr)>=0);draining=true;break;}
    check(result>=0);
    if(packet->stream_index==stream){result=avcodec_send_packet(codec,packet);sent=true;}
    av_packet_unref(packet);check(result>=0);
   }
  }
  check(false);return false;
 }
 void restart(){limit();check(av_seek_frame(format,stream,format->streams[stream]->start_time==AV_NOPTS_VALUE?0:format->streams[stream]->start_time,AVSEEK_FLAG_BACKWARD)>=0);avcodec_flush_buffers(codec);av_packet_unref(packet);first=previous=-1;draining=false;}
 ~VideoDecoder(){sws_freeContext(scale);av_frame_free(&frame);av_packet_free(&packet);avcodec_free_context(&codec);avformat_close_input(&format);if(io){av_freep(&io->buffer);avio_context_free(&io);}if(file)fclose(file);}
};

struct BackgroundVideo {
 std::string name;int width=0,height=0;bool active=false;size_t consumed=0;
 std::atomic<bool> stop{false};std::thread worker;std::mutex mutex;std::condition_variable wake;
 std::vector<unsigned char> latest;size_t revision=0;bool failed=false;double latestPts=0,consumedPts=0;
 bool load(const char *path){
  name=path;try{VideoDecoder d;d.open(path);double pts;VideoDecoder::check(d.next(latest,pts));width=d.width;height=d.height;return true;}catch(...){return false;}
 }
 void start(){
  if(active)return;active=true;stop=false;
  worker=std::thread([this]{try{
   VideoDecoder d;d.open(name.c_str(),&stop);double origin=VideoDecoder::clock(),pts=0;std::vector<unsigned char> pixels;
   while(!stop){
    if(!d.next(pixels,pts)){
     {std::unique_lock<std::mutex> lock(mutex);wake.wait_until(lock,std::chrono::steady_clock::time_point(std::chrono::duration_cast<std::chrono::steady_clock::duration>(std::chrono::duration<double>(origin+d.duration))),[this]{return stop.load();});}
     if(stop)break;origin+=d.duration;d.restart();continue;
    }
    std::unique_lock<std::mutex> lock(mutex);
    wake.wait_until(lock,std::chrono::steady_clock::time_point(std::chrono::duration_cast<std::chrono::steady_clock::duration>(std::chrono::duration<double>(origin+pts))),[this]{return stop.load();});
    if(stop)break;latest.swap(pixels);latestPts=pts;revision++;
   }
  }catch(...){if(!stop){std::lock_guard<std::mutex> lock(mutex);failed=true;}}});
 }
 void hide(){if(!active)return;stop=true;wake.notify_all();if(worker.joinable())worker.join();active=false;}
 int update(bool visible,std::vector<unsigned char> &pixels){
  if(!visible){hide();return 0;}start();std::lock_guard<std::mutex> lock(mutex);
  if(failed)return -1;if(revision==consumed)return 0;pixels=latest;consumed=revision;consumedPts=latestPts;return 1;
 }
 ~BackgroundVideo(){hide();}
};
