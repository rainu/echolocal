#pragma once
#include "turntable.hpp"
#include <array>
#include <charconv>
#include <cstring>
// Accept x,y,size for old Lab callers and x,y,width,height for a rectangle.
// Validate before assigning; reject overflow, trailing input and partial shapes.
static bool avatarParseViewport(const char *text,int &x,int &y,int &w,int &h) {
 std::array<int,4> values{};
 const char *end=text+std::strlen(text),*at=text;
 int count=0;
 while(at<end && count<4) {
  auto parsed=std::from_chars(at,end,values[count]);
  if(parsed.ec!=std::errc{} || parsed.ptr==at) return false;
  ++count;at=parsed.ptr;
  if(at==end) break;
  if(*at!=',') return false;
  ++at;
  if(at==end) return false;
 }
 if(at!=end || (count!=3 && count!=4)) return false;
 int width=values[2],height=count==3?width:values[3];
 if(values[0]<0 || values[1]<0 || width<128 || width>2048 ||
    height<128 || height>2048 || values[0]>4096-width || values[1]>4096-height) return false;
 x=values[0];y=values[1];w=width;h=height;
 return true;
}

// Map the complete square Lab composition into the centered fullscreen square.
// Apply this factor to BOTH zoom and pan: fullscreen offsets use the smaller
// panel dimension, so a Lab close-up cannot move offscreen when previewed.
static float avatarFullscreenZoom(float zoom, int w, int h) {
 return w>h ? zoom*float(h)/float(w) : zoom;
}

// One projection path for renderer and geometry regression tests.
static std::array<float,16> avatarFraming(std::array<float,16> base,
    float zoom,float x,float y,int w,int h,bool fullscreen) {
 const float scale=fullscreen?avatarFullscreenZoom(1,w,h):1;
 return avatarViewportAspect(avatarPan(avatarZoom(base,zoom*scale),x*scale,y*scale),w,h);
}
