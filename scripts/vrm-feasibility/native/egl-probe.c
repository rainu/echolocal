// Offscreen EGL/GLES capability check, not a VRM benchmark.
#include <EGL/egl.h>
#include <GLES3/gl3.h>
#include <stdio.h>
#include <time.h>

static double now(void) {
    struct timespec t;
    clock_gettime(CLOCK_MONOTONIC, &t);
    return t.tv_sec + t.tv_nsec / 1e9;
}
int main(void) {
    int status = 1;
    EGLDisplay d = eglGetDisplay(EGL_DEFAULT_DISPLAY);
    EGLContext c = EGL_NO_CONTEXT;
    EGLSurface s = EGL_NO_SURFACE;
    EGLint major, minor, n = 0;
    if (d == EGL_NO_DISPLAY || !eglInitialize(d, &major, &minor)) {
        fprintf(stderr, "eglInitialize failed: 0x%x\n", eglGetError());
        return 1;
    }
    printf("EGL_VERSION=%s\nEGL_VENDOR=%s\n", eglQueryString(d, EGL_VERSION), eglQueryString(d, EGL_VENDOR));
    EGLint cfgattrs[] = {EGL_SURFACE_TYPE, EGL_PBUFFER_BIT, EGL_RENDERABLE_TYPE, EGL_OPENGL_ES3_BIT,
        EGL_RED_SIZE,8,EGL_GREEN_SIZE,8,EGL_BLUE_SIZE,8,EGL_ALPHA_SIZE,8,EGL_NONE};
    EGLConfig cfg;
    if (!eglChooseConfig(d,cfgattrs,&cfg,1,&n) || n==0) goto done;
    EGLint pbattrs[] = {EGL_WIDTH,64,EGL_HEIGHT,64,EGL_NONE};
    EGLint ctxattrs[] = {EGL_CONTEXT_CLIENT_VERSION,3,EGL_NONE};
    s=eglCreatePbufferSurface(d,cfg,pbattrs);
    c=eglCreateContext(d,cfg,EGL_NO_CONTEXT,ctxattrs);
    if (s==EGL_NO_SURFACE || c==EGL_NO_CONTEXT || !eglMakeCurrent(d,s,s,c)) goto done;
    printf("GL_VERSION=%s\nGL_VENDOR=%s\nGL_RENDERER=%s\nGLSL_VERSION=%s\n",
        glGetString(GL_VERSION),glGetString(GL_VENDOR),glGetString(GL_RENDERER),glGetString(GL_SHADING_LANGUAGE_VERSION));
    GLint maxtex=0, vertexuniforms=0;
    glGetIntegerv(GL_MAX_TEXTURE_SIZE,&maxtex);
    glGetIntegerv(GL_MAX_VERTEX_UNIFORM_VECTORS,&vertexuniforms);
    printf("MAX_TEXTURE_SIZE=%d\nMAX_VERTEX_UNIFORM_VECTORS=%d\n",maxtex,vertexuniforms);
    glViewport(0,0,64,64);
    glClearColor(0.25f,0.5f,0.75f,1.0f);
    glClear(GL_COLOR_BUFFER_BIT);
    unsigned char pixel[4]={0};
    glReadPixels(32,32,1,1,GL_RGBA,GL_UNSIGNED_BYTE,pixel);
    if (glGetError()!=GL_NO_ERROR || pixel[0]<62 || pixel[0]>65 || pixel[1]<126 || pixel[1]>129 || pixel[2]<189 || pixel[2]>193 || pixel[3]!=255) {
        fprintf(stderr,"readback failed\n");goto done;
    }
    printf("RGBA_READBACK=%u,%u,%u,%u\n",pixel[0],pixel[1],pixel[2],pixel[3]);
    double begin=now();
    for (int i=0;i<120;i++) {glClear(GL_COLOR_BUFFER_BIT);glFinish();}
    printf("CLEAR_FINISH_120_MS=%.3f\n",1000*(now()-begin));
    if (glGetError()!=GL_NO_ERROR) goto done;
    status=0;
done:
    if(status) fprintf(stderr,"EGL/GLES probe failed: EGL error 0x%x\n",eglGetError());
    eglMakeCurrent(d,EGL_NO_SURFACE,EGL_NO_SURFACE,EGL_NO_CONTEXT);
    if(c!=EGL_NO_CONTEXT)eglDestroyContext(d,c);
    if(s!=EGL_NO_SURFACE)eglDestroySurface(d,s);
    eglTerminate(d);
    return status;
}
