#include <cstring>
#include "video.hpp"
#include <cassert>
#include <cstdio>
int main(int argc,char **argv){
 setvbuf(stdout,nullptr,_IONBF,0);assert(argc>=2 && argc<=4);
 try{
  VideoDecoder d;d.open(argv[1]);double pts=0;std::vector<unsigned char> pixels;
  std::vector<unsigned char> first,last;int count=0;double lastPts=-1;
  while(d.next(pixels,pts)){assert(pts>lastPts);lastPts=pts;if(count==0)first=pixels;last=pixels;count++;}
  assert(count>1 && pixels.size()==size_t(d.width)*d.height*4);
  if(argc==4){int w=0,h=0;assert(sscanf(argv[3],"%dx%d",&w,&h)==2 && d.width==w && d.height==h);}
  d.restart();assert(d.next(pixels,pts) && pts==0 && pixels==first);
  printf("VIDEO_DECODE_LOOP=PASS frames=%d width=%d height=%d duration=%.3f\n",count,d.width,d.height,d.duration);
  if(argc>=3){FILE *out=fopen(argv[2],"wb");assert(out);fwrite("BG01",1,4,out);uint32_t dims[2]={uint32_t(d.width),uint32_t(d.height)};fwrite(dims,4,2,out);fwrite(pixels.data(),1,pixels.size(),out);fclose(out);}
  BackgroundVideo video;assert(video.load(argv[1]));std::vector<unsigned char> picture;
  video.update(true,picture);std::this_thread::sleep_for(std::chrono::milliseconds(450));video.update(true,picture);
  video.update(false,picture);size_t revision=video.revision;assert(!video.active && revision>1);
  std::this_thread::sleep_for(std::chrono::milliseconds(150));assert(video.revision==revision);
  video.update(true,picture);
  for(int i=0;i<60 && video.consumed<=revision;i++){std::this_thread::sleep_for(std::chrono::milliseconds(50));video.update(true,picture);}
  video.update(false,picture);
  assert(video.revision>revision && video.latestPts<.2);
  printf("VIDEO_VISIBILITY_RESTART=PASS\n");
  // Deliberately late output skips stale conversion, never slows the clock.
  VideoDecoder late;late.open(argv[1]);
  const double origin=VideoDecoder::clock()-.4;
  assert(late.next(picture,pts,origin) && pts>=.32 && late.dropped>0);
  printf("VIDEO_LATE_FRAME_CATCHUP=PASS\n");
 }catch(const std::exception &e){puts("VIDEO_TEST=FAIL");return 1;}
}
