#pragma once
// Bounded scalar-only stdin protocol for the versioned local avatar control.
#include <cerrno>
#include <array>
#include <cmath>
#include <cstdio>
#include <poll.h>
#include <string>
#include <unistd.h>
struct RendererControl {
  unsigned long long motionRevision=0;
  int motionFD=0; bool motionPlay=true;
  bool enabled = false, idle = true, blink = true, animation = true,
       ended = false;
  int mode = 0;
  int overlayX=0, overlayY=0, overlayW=0, overlayH=0;
 bool poseEdit=false, poseActive=false; int selected=-1;
 std::array<std::array<float,3>,54> pose{};
  bool fullscreen = false, hidden=false;
  float fixed = .5f, level = 0, yaw = 0, pitch = 0, zoom = 1, panX = 0, panY = 0;
  double updated = 0;
  std::string line;
  explicit RendererControl(bool on) : enabled(on) {}
  void parse(const std::string &s) {
    int i, b, a, m;
    unsigned long long revision; int fd; char motionTail;
    if(sscanf(s.c_str(),"A %llu %d %c",&revision,&fd,&motionTail)==2 && revision>motionRevision && revision<=2147483647 && (fd==0 || (fd>=3 && fd<=1048576))) { motionRevision=revision; motionFD=fd; return; }
    if(sscanf(s.c_str(),"N %d %c",&i,&motionTail)==1 && (i==0 || i==1)) { motionPlay=i; return; }
    char overlayTail;
    if (sscanf(s.c_str(), "V %5d %5d %5d %5d %c", &i, &b, &a, &m, &overlayTail)==4 && i>=0 && b>=0 && a>=0 && m>=0 && a<=4096 && m<=4096 && i<=4096-a && b<=4096-m) { overlayX=i; overlayY=b; overlayW=a; overlayH=m; return; }
    float rx,ry,rz; char trailing;
    if(sscanf(s.c_str(),"E %d %d %d %c",&i,&b,&a,&trailing)==3 && i>=0&&i<=1&&b>=-1&&b<54&&a>=0&&a<=1){poseEdit=i;selected=b;poseActive=a;return;}
    if(sscanf(s.c_str(),"P %d %f %f %f %c",&i,&rx,&ry,&rz,&trailing)==4&&i>=0&&i<54&&std::isfinite(rx)&&std::isfinite(ry)&&std::isfinite(rz)&&fabsf(rx)<=90&&fabsf(ry)<=90&&fabsf(rz)<=90){pose[i]={rx,ry,rz};return;}
    char tail;
 if(sscanf(s.c_str(),"K %d %c",&i,&tail)==1&&(i==0||i==1)){hidden=i;return;}
    if (sscanf(s.c_str(), "T %d %c", &i, &tail) == 1 && (i==0 || i==1)) { fullscreen=i; return; }
    float f, l, y, p, z, x, t;
    char extra;
    if (sscanf(s.c_str(), "S %d %d %d %d %f %f %f %f %f %f %f %c", &i, &b, &a, &m, &f, &l,
               &y, &p, &z, &x, &t, &extra) != 11 ||
        i < 0 || i > 1 || b < 0 || b > 1 || a < 0 || a > 1 || m < 0 || m > 3 ||
        !std::isfinite(f) || !std::isfinite(l) || f < 0 || f > 1 || l < 0 ||
        l > 1 || !std::isfinite(y) || y < -180 || y > 180 || !std::isfinite(p) || p < -85 || p > 85 ||
        !std::isfinite(z) || z < .5f || z > 9.99f || !std::isfinite(x) || x < -5 || x > 5 ||
 !std::isfinite(t) || t < -5 || t > 5)
      return;
    idle = i;
    blink = b;
    animation = a;
    mode = m;
    fixed = f;
    level = l;
    yaw = y;
    pitch = p;
    zoom = z;
 panX = x; panY = t;
    updated = now();
  }
  void read() {
    if (!enabled)
      return;
    // Frame-budget bound even when input is flooded or malformed.
    for (int n = 0; n < 2048; ++n) {
      pollfd p{STDIN_FILENO, POLLIN, 0};
      if (poll(&p, 1, 0) <= 0)
        break;
      if (p.revents & (POLLERR | POLLNVAL)) {
        ended = true;
        break;
      }
      if (!(p.revents & (POLLIN | POLLHUP)))
        break;
      char c;
      ssize_t k = ::read(STDIN_FILENO, &c, 1);
      if (k == 0) {
        ended = true;
        break;
      }
      if (k < 0) {
        if (errno != EINTR && errno != EAGAIN)
          ended = true;
        break;
      }
      if (c == '\n') {
        parse(line);
        line.clear();
      } else if (line.size() < 120)
        line += c;
      else {
        ended = true;
        break;
      }
    }
  }
  float mouth(double t) const {
    if (mode == 1)
      return fixed;
    if (mode == 2)
      return float(.5 + .5 * std::sin(t * 8));
    if (mode == 3 && now() - updated < .2)
      return level;
    return 0;
  }
};
