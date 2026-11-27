#pragma once
#include <cstdint>
#include <cstdlib>
#include <cstring>
#include <cmath>
#include <cstdio>
#include <vector>
#include <algorithm>

// Descriptor-backed RGBA snapshots only. The daemon has already decoded the
// asset with libwebp. Keep one working frame in RAM and one texture per layer;
// do not retain/upload the complete animation on the GPU.
struct LayerFrames {
 FILE *file=nullptr;
 int width=0,height=0, count=1, loops=0, duration=0, current=-1;
 bool animated=false, visible=false;
 double started=0;
 size_t uploads=0;
 std::vector<int> ends;
 std::vector<unsigned char> pixels;
 static uint32_t u32(const unsigned char *b) {
  return uint32_t(b[0])|(uint32_t(b[1])<<8)|(uint32_t(b[2])<<16)|(uint32_t(b[3])<<24);
 }
 bool load(const char *name) {
  file=fopen(name,"rb"); if(!file)return false;
  unsigned char header[24]{};
  if(fread(header,1,12,file)!=12)return false;
  animated=memcmp(header,"BG02",4)==0;
  if(!animated && memcmp(header,"BG01",4)!=0)return false;
  width=int(u32(header+4));height=int(u32(header+8));
  if(width<=0 || height<=0 || width>2048 || height>2048)return false;
  const size_t size=size_t(width)*height*4;
  long expected=12+long(size);
  if(animated) {
   if(fread(header+12,1,12,file)!=12)return false;
   count=int(u32(header+12));loops=int(u32(header+16));duration=int(u32(header+20));
   if(width>1024 || height>1024 || count<2 || count>120 || loops<0 || loops>65535 ||
      duration<=0 || duration>30000 || size*size_t(count)>(32u<<20))return false;
   expected=24+long(size+4)*count;
   int sum=0;
   for(int i=0;i<count;i++) {
    if(fseek(file,24+long(size+4)*i,SEEK_SET)!=0 || fread(header,1,4,file)!=4)return false;
    const auto delay=u32(header);
    if(delay<40 || delay>10000)return false;
    sum+=int(delay); ends.push_back(sum);
   }
   if(sum!=duration)return false;
  }
  if(fseek(file,0,SEEK_END)!=0 || ftell(file)!=expected)return false;
  pixels.resize(size);
  return read(0);
 }
 bool read(int index) {
  if(index==current)return true;
  const long offset=animated?28+long(pixels.size()+4)*index:12;
  if(fseek(file,offset,SEEK_SET)!=0 || fread(pixels.data(),1,pixels.size(),file)!=pixels.size())return false;
  current=index;return true;
 }
 int frameAt(double elapsed) const {
  if(!animated)return 0;
  const double ms=std::max(0.,elapsed*1000.);
  if(loops && ms>=double(duration)*loops)return count-1;
  const int phase=int(fmod(ms,double(duration)));
  return int(std::upper_bound(ends.begin(),ends.end(),phase)-ends.begin());
 }
 // Hidden layers do no file reads, conversions or uploads. A newly visible
 // scene starts at frame zero, including after switching presentation modes.
 int select(double time,bool show) {
  if(!show){visible=false;return -1;}
  if(!visible){visible=true;started=time;}
  return frameAt(time-started);
 }
 ~LayerFrames(){if(file)fclose(file);}
 LayerFrames()=default;
 LayerFrames(const LayerFrames&)=delete;
 LayerFrames& operator=(const LayerFrames&)=delete;
};
