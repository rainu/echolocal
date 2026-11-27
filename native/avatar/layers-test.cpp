#include "layers.hpp"
#include <cassert>
#include <unistd.h>
static void word(FILE *f,uint32_t n){unsigned char b[4]={static_cast<unsigned char>(n),static_cast<unsigned char>(n>>8),static_cast<unsigned char>(n>>16),static_cast<unsigned char>(n>>24)};assert(fwrite(b,1,4,f)==4);}
int main(){
 char name[]="/tmp/avatar-layer-XXXXXX"; int fd=mkstemp(name); assert(fd>=0);
 FILE *f=fdopen(fd,"wb");fwrite("BG02",1,4,f);for(auto n:{1,1,3,2,600})word(f,n);
 for(int delay:{100,200,300}){word(f,delay);word(f,0x800c18f0);}
 fclose(f);
 LayerFrames a;assert(a.load(name));assert(a.animated && a.current==0);
 assert(a.select(10,true)==0);assert(a.select(10.101,true)==1);
 assert(a.select(10.301,true)==2);assert(a.select(10.601,true)==0);
 assert(a.select(11.201,true)==2);assert(a.select(20,false)==-1);
 assert(a.select(30,true)==0);assert(a.select(30.101,true)==1);
 a.loops=0;assert(a.frameAt(6.301)==2);assert(a.frameAt(.099)==0);
 // No hidden read; resume resets both the clock and selected frame.
 const int before=a.current;assert(a.select(31,false)==-1);assert(a.current==before);
 assert(a.read(2));assert(a.pixels[3]==128);assert(a.read(0));
 f=fopen(name,"ab");fputc(0,f);fclose(f);LayerFrames bad;assert(!bad.load(name));
 unlink(name);
 puts("LAYER_CLOCK_AND_SNAPSHOT=PASS");
}
