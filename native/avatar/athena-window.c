// Athena/API-28 Fire OS surface adapter, based on libcountertop
// native/surface/surface.c. libcountertop attribution and AGPL-3.0 license are
// retained in THIRD_PARTY.md. Deliberately separate from the portable renderer;
// no framework/zygote needed.
#include <dlfcn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
static void *client, *control, *surface;
static uint32_t transaction[64];
static void (*dec)(const void *, const void *);
static void (*clear)(void *);
static void (*tx_destroy)(void *);
static void *symbol(const char *name) {
  void *p = dlsym(RTLD_DEFAULT, name);
  if (!p)
    fprintf(stderr, "surface ABI symbol missing: %s\n", name);
  return p;
}
#define FN(ret, name, args, sym)                                               \
  ret(*name) args = (ret(*) args)symbol(sym);                                  \
  if (!name)                                                                   \
  goto fail
void avatar_window_close(void) {
  if (control) {
    if (clear)
      clear(control);
    if (dec)
      dec(control, &control);
    control = NULL;
  }
  if (surface) {
    if (dec)
      dec(surface, &surface);
    surface = NULL;
  }
  if (client) {
    if (dec)
      dec(client, client);
    client = NULL;
  }
  if (tx_destroy) {
    tx_destroy(transaction);
    tx_destroy = NULL;
    memset(transaction, 0, sizeof transaction);
  }
}
void *avatar_window_open(int w, int h, int x, int y) {
  if (!dlopen("libutils.so", RTLD_NOW | RTLD_GLOBAL) ||
      !dlopen("libbinder.so", RTLD_NOW | RTLD_GLOBAL) ||
      !dlopen("libgui.so", RTLD_NOW | RTLD_GLOBAL)) {
    fprintf(stderr, "SurfaceComposer libraries unavailable\n");
    return NULL;
  }
  FN(void, ps_self, (void **), "_ZN7android12ProcessState4selfEv");
  FN(void, ps_start, (void *), "_ZN7android12ProcessState15startThreadPoolEv");
  FN(void *, ctor, (void *), "_ZN7android21SurfaceComposerClientC1Ev");
  FN(int, check, (const void *),
     "_ZNK7android21SurfaceComposerClient9initCheckEv");
  FN(void, inc, (const void *, const void *),
     "_ZNK7android7RefBase9incStrongEPKv");
  dec = (void (*)(const void *, const void *))symbol(
      "_ZNK7android7RefBase9decStrongEPKv");
  clear = (void (*)(void *))symbol("_ZN7android14SurfaceControl5clearEv");
  FN(void *, str_ctor, (void *, const char *), "_ZN7android7String8C1EPKc");
  FN(void, str_destroy, (void *), "_ZN7android7String8D1Ev");
  FN(void, create,
     (void **, void *, const void *, uint32_t, uint32_t, int32_t, uint32_t,
      void *, int32_t, int32_t),
     "_ZN7android21SurfaceComposerClient13createSurfaceERKNS_7String8EjjijPNS_"
     "14SurfaceControlEii");
  FN(void, get_surface, (void **, const void *),
     "_ZNK7android14SurfaceControl10getSurfaceEv");
  FN(void *, tx_copy, (void *, const void *),
     "_ZN7android21SurfaceComposerClient11TransactionC1ERKS1_");
  FN(int, apply, (void *, int),
     "_ZN7android21SurfaceComposerClient11Transaction5applyEb");
  FN(void *, layer, (void *, void *const *, int32_t),
     "_ZN7android21SurfaceComposerClient11Transaction8setLayerERKNS_2spINS_"
     "14SurfaceControlEEEi");
  FN(void *, position, (void *, void *const *, float, float),
     "_ZN7android21SurfaceComposerClient11Transaction11setPositionERKNS_2spINS_"
     "14SurfaceControlEEEff");
  FN(void *, show, (void *, void *const *),
     "_ZN7android21SurfaceComposerClient11Transaction4showERKNS_2spINS_"
     "14SurfaceControlEEE");
  if (!dec || !clear)
    goto fail;
  void *ps = NULL;
  ps_self(&ps);
  if (ps)
    ps_start(ps);
  client = calloc(1, 512);
  if (!client)
    goto fail;
  ctor(client);
  inc(client, client);
  if (check(client))
    goto fail;
  uint32_t name[16] = {0};
  str_ctor(name, "EchoLocal Avatar");
  create(&control, client, name, w, h, 1, 0, NULL, 0, 0);
  str_destroy(name);
  if (!control)
    goto fail;
  // Fire OS 7 Transaction layout, as used by the installed native surface
  // helper.
  uint32_t from[64] = {0};
  float one = 1;
  memcpy((char *)from + 0x14, &one, 4);
  from[0x28 / 4] = 0x40;
  tx_copy(transaction, from);
  // This vendor libgui does not export TransactionD1. One transaction per
  // short-lived viewer; its storage and Binder references die with this
  // process.
  layer(transaction, &control, 2000000000);
  position(transaction, &control, (float)x, (float)y);
  show(transaction, &control);
  if (apply(transaction, 1))
    goto fail;
  get_surface(&surface, control);
  if (!surface)
    goto fail;
  for (int off = 0; off < 64; off += 4)
    if (*(uint32_t *)((char *)surface + off) == 0x5f776e64)
      return (char *)surface + off;
fail:
  avatar_window_close();
  return NULL;
}
