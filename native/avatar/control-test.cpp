#include <algorithm>
#include <cassert>
#include <cerrno>
#include <cstring>
static double stamp = 1;
static double now() { return stamp; }
#include "control.hpp"
#include "turntable.hpp"
#include "viewport.hpp"
int main() {
  RendererControl c(true);
  c.parse("K 1");assert(c.hidden);
  for(const char *bad:{"K 2","K -1","K 0 extra"}){c.parse(bad);assert(c.hidden);}
  c.parse("K 0");assert(!c.hidden);
  c.parse("P 7 30 -20 90");c.parse("E 1 7 1");
  assert(c.pose[7][0]==30 && c.pose[7][1]==-20 && c.pose[7][2]==90 && c.poseEdit && c.poseActive && c.selected==7);
  for(const char *bad:{"P -1 0 0 0","P 54 0 0 0","P 7 nan 0 0","P 7 91 0 0","P 7 0 0 0 extra","E 1 54 1","E 2 7 1","E 1 7 2","E 0 0 0 extra"}){c.parse(bad);assert(c.pose[7][0]==30 && c.poseEdit && c.selected==7);}
  c.parse("P 53 1 2 3");assert(c.pose[53][2]==3);
  c.parse("E 0 -1 1");assert(!c.poseEdit && c.poseActive);

  c.parse("S 1 0 1 1 0.7 0 90 0 1 0 0");
  assert(c.idle && !c.blink && c.animation && c.mouth(0) == .7f);
  c.parse("S 0 1 0 3 0.5 0.8 -45 0 1 0 0");
  assert(c.yaw == -45);
  assert(!c.idle && c.blink && !c.animation && c.mouth(0) == .8f);
  stamp += .3;
  assert(c.mouth(0) == 0);
  for (const char *s : {"S 1 1 1 9 0 0 0 0 1 0 0", "S 1 1 1 1 nan 0 0 0 1 0 0",
                        "S 1 1 1 1 0 2 0 0 1 0 0", "S 1 1 1 1 0 0 0 extra 0 1 0 0"}) {
    c.parse(s);
    assert(c.mode == 3);
  }
  for (const char *bad :
       {"S 1 1 1 0 0 0 nan 0 1 0 0", "S 1 1 1 0 0 0 inf 0 1 0 0", "S 1 1 1 0 0 0 181 0 1 0 0"}) {
    c.parse(bad);
    assert(c.yaw == -45);
  }
  for (float angle : {-180.f, -90.f, 0.f, 90.f, 180.f}) {
    auto m = avatarYaw(angle, 2, 3);
    auto x = m[0] * 2 + m[8] * 3 + m[12], z = m[2] * 2 + m[10] * 3 + m[14];
    assert(std::abs(x - 2) < 1e-5f && std::abs(z - 3) < 1e-5f);
  }
  auto side = avatarYaw(90, 2, 3);
  assert(std::abs((side[0] * 2 + side[8] * 4 + side[12]) - 3) < 1e-5f);
  assert(std::abs((side[2] * 2 + side[10] * 4 + side[14]) - 3) < 1e-5f);
  c.parse("S 1 1 1 0 0 0 40 -70 2.5 0 0");
  assert(c.pitch == -70 && c.zoom == 2.5f && c.yaw == 40);
  for(const char *bad:{"S 1 1 1 0 0 0 0 nan 1 0 0","S 1 1 1 0 0 0 0 86 1 0 0","S 1 1 1 0 0 0 0 0 0 0 0","S 1 1 1 0 0 0 0 0 10 0 0","S 1 1 1 0 0 0 0 0 0"}){c.parse(bad);assert(c.pitch == -70 && c.zoom == 2.5f && c.yaw == 40);}
  for(float zoom : {3.f, 5.f, 9.99f}) {
    char line[120];
    snprintf(line,sizeof(line),"S 0 0 0 0 0 0 25 -15 %.4f .2 -.3",zoom);
    c.parse(line);
    assert(c.zoom == zoom && c.panX == .2f && c.panY == -.3f && c.yaw == 25 && c.pitch == -15);
    auto base = avatarOrbit(25,-15,2,5,3);
    auto framed = avatarPan(avatarZoom(base,zoom),.2f,-.3f);
    for(auto dim : {std::array<int,2>{640,640},{640,800},{1280,800}}) {
      auto projected = avatarViewportAspect(framed,dim[0],dim[1]);
      // Width-relative framing: same x/y pixel scale in Lab, Talk and future fullscreen.
      assert(std::abs(projected[0]-base[0]*zoom) < 1e-5f);
      assert(std::abs(projected[1]*dim[1]-base[1]*zoom*dim[0]) < .01f);
      assert(projected[2] == base[2]);
    }
    for(const char *bad : {"S 0 0 0 0 0 0 0 0 10 0 0","S 0 0 0 0 0 0 0 0 9.991 0 0","S 0 0 0 0 0 0 0 0 .49 0 0","S 0 0 0 0 0 0 0 0 nan 0 0"}) {
      c.parse(bad); assert(c.zoom == zoom && c.panX == .2f && c.panY == -.3f);
    }
  }
  for(float yaw:{-180.f,0.f,90.f})for(float pitch:{-85.f,0.f,85.f}) {
   auto m=avatarOrbit(yaw,pitch,2,5,3);
   for(int r=0;r<3;++r){float v=m[r]*2+m[4+r]*5+m[8+r]*3+m[12+r];assert(std::abs(v-(r==0?2:r==1?5:3))<1e-5f);}
   auto z=avatarZoom(m,2);assert(z[0]==m[0]*2 && z[1]==m[1]*2 && z[2]==m[2]);
  }
  auto tilt=avatarOrbit(0,90,0,0,0);
  assert(std::abs(tilt[6]-1)<1e-5f && std::abs(tilt[9]+1)<1e-5f);
  c.parse("S 1 1 1 0 0 0 40 -70 2.5 0.25 -0.5");
  assert(c.panX==.25f && c.panY==-.5f);
  for(const char *bad:{"S 1 1 1 0 0 0 0 0 1 nan 0","S 1 1 1 0 0 0 0 0 1 0 inf","S 1 1 1 0 0 0 0 0 1 5.1 0","S 1 1 1 0 0 0 0 0 1 0 -5.1","S 1 1 1 0 0 0 0 0 1"}) {c.parse(bad);assert(c.panX==.25f && c.panY==-.5f);}
  auto m=avatarOrbit(35,20,2,5,3);
  auto moved=avatarPan(avatarZoom(m,2),.25f,-.5f);
  assert(std::abs(moved[12]-(2*m[12]+.5f))<1e-5f);
  assert(std::abs(moved[13]-(2*m[13]+1))<1e-5f);
  assert(moved[2]==m[2] && moved[0]==2*m[0]);
  int x=0,y=0,w=0,h=0;
  assert(avatarParseViewport("640,80,640",x,y,w,h) && x==640 && y==80 && w==640 && h==640);
  assert(avatarParseViewport("640,0,640,800",x,y,w,h) && x==640 && y==0 && w==640 && h==800);
  for(const char *bad:{"640,0,640,","640,0,640,800,x","640,0,640,0","-1,0,640,800","4096,0,640,800","0,0,999999999999999999,800","0,0,128,128junk"}) {
   assert(!avatarParseViewport(bad,x,y,w,h));
   assert(x==640 && y==0 && w==640 && h==800);
  }
  for(auto dimensions: {std::array<int,2>{640,800},{400,1280},{800,400},{640,640}}) {
   auto mapped=avatarViewportAspect(moved,dimensions[0],dimensions[1]);
   for(int col=0;col<4;++col) {
    assert(std::abs(mapped[col*4+1]*dimensions[1]-moved[col*4+1]*dimensions[0])<.001f);
    assert(mapped[col*4]==moved[col*4] && mapped[col*4+2]==moved[col*4+2]);
   }
  }
  c.parse("S 0 0 0 0 0 0 12 6 9.99 -5 5");
  assert(c.zoom == 9.99f && c.panX == -5 && c.panY == 5);
  for (int n=0;n<100;++n){c.parse("T 1");assert(c.fullscreen);c.parse("T 0");assert(!c.fullscreen);}
  for(const char *bad:{"T 2","T -1","T 1 junk","T NaN","T 1.5"}){c.parse(bad);assert(!c.fullscreen);}
  assert(c.zoom==9.99f && c.panX==-5 && c.panY==5);
  assert(std::abs(avatarFullscreenZoom(1,1280,800)-.625f)<.0001f);
  assert(std::abs(avatarFullscreenZoom(2,800,480)-1.2f)<.0001f);
  assert(avatarFullscreenZoom(1,800,1280)==1);
  assert(avatarFullscreenZoom(1,640,640)==1);
  // Regression: a face close-up configured in the square Lab used to vanish
  // because fullscreen scaled zoom but not the saved translation.
  for(auto dim : {std::array<int,2>{1280,800},{800,480},{640,480},{800,1280}}) {
    for(auto framing : {std::array<float,3>{4.68f,-.05f,1.68f},{1,0,0},{9.99f,-2,3}}) {
      auto base=avatarOrbit(25,-15,2,5,3);
      auto square=avatarPan(avatarZoom(base,framing[0]),framing[1],framing[2]);
      float fit=avatarFullscreenZoom(1,dim[0],dim[1]);
      auto full=avatarFraming(base,framing[0],framing[1],framing[2],dim[0],dim[1],true);
      auto split=avatarFraming(base,framing[0],framing[1],framing[2],dim[0],dim[1],false);
      assert(split==avatarViewportAspect(square,dim[0],dim[1]));
      // All affine terms keep the same location and scale per shorter-side pixel.
      for(int col=0;col<4;++col) {
        assert(std::abs(full[col*4]/fit-square[col*4])<.0001f);
        assert(std::abs(full[col*4+1]/(fit*float(dim[0])/dim[1])-square[col*4+1])<.0001f);
        assert(full[col*4+2]==square[col*4+2]);
      }
    }
  }
  int fds[2];
  assert(pipe(fds) == 0);
  assert(dup2(fds[0], STDIN_FILENO) >= 0);
  close(fds[0]);
  const char *s = "S 0 0 0 1 0.3 0 0 0 1 0 0\n";
  assert(write(fds[1], s, strlen(s)) > 0);
  c.read();
  assert(c.mode == 1 && c.mouth(0) == .3f && !c.ended);
  close(fds[1]);
  c.read();
  assert(c.ended);
  puts("CONTROL_TEST=PASS");
}
