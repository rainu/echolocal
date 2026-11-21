#pragma once
#include <array>
#include <cmath>
// Column-major world-space yaw around the fitted geometry center. The pivot
// remains fixed, including for models whose authored origin is off-center.
static std::array<float, 16> avatarYaw(float degrees, float cx, float cz) {
  const float angle = degrees * 3.14159265358979323846f / 180.f;
  const float c = std::cos(angle), s = std::sin(angle);
  return {c,
          0,
          -s,
          0,
          0,
          1,
          0,
          0,
          s,
          0,
          c,
          0,
          cx - c * cx - s * cz,
          0,
          cz + s * cx - c * cz,
          1};
}

// Rx(pitch) * Ry(yaw), pivoted on the fitted center, in model space.
// Pitch is reversed for legacy VRM0's front-facing correction.
static std::array<float,16> avatarOrbit(float yaw,float pitch,float cx,float cy,float cz) {
 auto m=avatarYaw(yaw,0,0);
 float a=pitch*3.14159265358979323846f/180.f,c=std::cos(a),s=std::sin(a);
 for(int col=0;col<3;++col){float y=m[col*4+1],z=m[col*4+2];m[col*4+1]=c*y-s*z;m[col*4+2]=s*y+c*z;}
 for(int row=0;row<3;++row)m[12+row]=(row==0?cx:row==1?cy:cz)-m[row]*cx-m[4+row]*cy-m[8+row]*cz;
 return m;
}
// Orthographic zoom about the preview center, leaving depth unchanged.
static std::array<float,16> avatarZoom(std::array<float,16> view,float zoom) {
 for(int col=0;col<4;++col){view[col*4]*=zoom;view[col*4+1]*=zoom;}
 return view;
}

// Preview-relative screen translation after zoom, independent of model rotation.
static std::array<float,16> avatarPan(std::array<float,16> view,float x,float y) {
 for(int col=0;col<4;++col){view[col*4]+=2*x*view[col*4+3];view[col*4+1]-=2*y*view[col*4+3];}
 return view;
}
