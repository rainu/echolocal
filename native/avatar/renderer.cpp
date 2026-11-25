// EchoLocal native avatar renderer. See LICENSE and THIRD_PARTY.md.
#define CGLTF_IMPLEMENTATION
#include "vendor/cgltf.h"
#define STB_IMAGE_IMPLEMENTATION
#define STBI_ONLY_PNG
#define STBI_ONLY_JPEG
#include "turntable.hpp"
#include "viewport.hpp"
#include "pose.hpp"
#include "vendor/json.hpp"
#include "vendor/stb_image.h"
#include <EGL/egl.h>
#include <GLES3/gl3.h>
#include <algorithm>
#include <array>
#include <chrono>
#include <cmath>
#include <csignal>
#include <cstdio>
#include <cstring>
#include <fstream>
#include <map>
#include <stdexcept>
#include <string>
#include <sys/resource.h>
#include <thread>
#include <vector>
using J = nlohmann::json;
using M = std::array<float, 16>;
static constexpr int MaxJoints = 192;
static volatile sig_atomic_t stopped = 0;
static void stop(int) { stopped = 1; }
static double now() {
  using namespace std::chrono;
  return duration<double>(steady_clock::now().time_since_epoch()).count();
}
static void need(bool b, const char *s) {
  if (!b)
    throw std::runtime_error(s);
}
static M identity() {
  M m{};
  m[0] = m[5] = m[10] = m[15] = 1;
  return m;
}
static M mul(const M &a, const M &b) {
  M m{};
  for (int c = 0; c < 4; c++)
    for (int r = 0; r < 4; r++)
      for (int k = 0; k < 4; k++)
        m[c * 4 + r] += a[k * 4 + r] * b[c * 4 + k];
  return m;
}
static M inverse(M m) {
  M out = identity();
  for (int c = 0; c < 4; c++) {
    int p = c;
    for (int r = c + 1; r < 4; r++)
      if (fabs(m[c * 4 + r]) > fabs(m[c * 4 + p]))
        p = r;
    need(fabs(m[c * 4 + p]) > 1e-9, "singular transform");
    for (int k = 0; k < 4; k++) {
      std::swap(m[k * 4 + c], m[k * 4 + p]);
      std::swap(out[k * 4 + c], out[k * 4 + p]);
    }
    float d = m[c * 4 + c];
    for (int k = 0; k < 4; k++) {
      m[k * 4 + c] /= d;
      out[k * 4 + c] /= d;
    }
    for (int r = 0; r < 4; r++)
      if (r != c) {
        float a = m[c * 4 + r];
        for (int k = 0; k < 4; k++) {
          m[k * 4 + r] -= a * m[k * 4 + c];
          out[k * 4 + r] -= a * out[k * 4 + c];
        }
      }
  }
  return out;
}
static M quat(const float *q) {
  float x = q[0], y = q[1], z = q[2], w = q[3];
  M m = identity();
  m[0] = 1 - 2 * (y * y + z * z);
  m[1] = 2 * (x * y + z * w);
  m[2] = 2 * (x * z - y * w);
  m[4] = 2 * (x * y - z * w);
  m[5] = 1 - 2 * (x * x + z * z);
  m[6] = 2 * (y * z + x * w);
  m[8] = 2 * (x * z + y * w);
  m[9] = 2 * (y * z - x * w);
  m[10] = 1 - 2 * (x * x + y * y);
  return m;
}
static void qmul(float *out, const float *a, const float *b) {
  float q[4] = {a[3] * b[0] + a[0] * b[3] + a[1] * b[2] - a[2] * b[1],
                a[3] * b[1] - a[0] * b[2] + a[1] * b[3] + a[2] * b[0],
                a[3] * b[2] + a[0] * b[1] - a[1] * b[0] + a[2] * b[3],
                a[3] * b[3] - a[0] * b[0] - a[1] * b[1] - a[2] * b[2]};
  memcpy(out, q, sizeof q);
}
static void slerp(float *out, const float *a, const float *b, float t) {
  float dot = 0;
  for (int i = 0; i < 4; i++)
    dot += a[i] * b[i];
  float sign = dot < 0 ? -1 : 1;
  dot = fabs(dot);
  float x = 1 - t, y = t;
  if (dot < .9995f) {
    float angle = acosf(std::min(1.f, dot)), d = sinf(angle);
    x = sinf((1 - t) * angle) / d;
    y = sinf(t * angle) / d;
  }
  float len = 0;
  for (int i = 0; i < 4; i++) {
    out[i] = x * a[i] + y * sign * b[i];
    len += out[i] * out[i];
  }
  len = sqrtf(len);
  for (int i = 0; i < 4; i++)
    out[i] /= len;
}
using Q = std::array<float, 4>;
using V = std::array<float, 3>;
static Q qm(Q a, Q b) {
  Q q;
  qmul(q.data(), a.data(), b.data());
  return q;
}
static Q qi(Q a) { return {-a[0], -a[1], -a[2], a[3]}; }
static Q qn(Q a) {
  float n = 0;
  for (float v : a)
    n += v * v;
  need(n > 1e-12f, "zero quaternion");
  for (float &v : a)
    v /= sqrtf(n);
  return a;
}
static V unit(V a) {
  float n = 0;
  for (float v : a)
    n += v * v;
  need(n > 1e-12f, "zero direction");
  for (float &v : a)
    v /= sqrtf(n);
  return a;
}
static V cross(V a, V b) {
  return {a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2],
          a[0] * b[1] - a[1] * b[0]};
}
static V applyq(Q q, V v) {
  Q x = qm(qm(q, {v[0], v[1], v[2], 0}), qi(q));
  return {x[0], x[1], x[2]};
}
static Q fromTo(V a, V b) {
  a = unit(a);
  b = unit(b);
  float d = 1 + a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
  if (d < 1e-6f) {
    V v = fabs(a[0]) > fabs(a[2]) ? V{-a[1], a[0], 0} : V{0, -a[2], a[1]};
    v = unit(v);
    return {v[0], v[1], v[2], 0};
  }
  auto v = cross(a, b);
  return qn({v[0], v[1], v[2], d});
}
static Q worldq(cgltf_node *n) {
  Q q = {0, 0, 0, 1};
  std::vector<cgltf_node *> chain;
  for (; n; n = n->parent)
    chain.push_back(n);
  for (auto it = chain.rbegin(); it != chain.rend(); ++it) {
    need(!(*it)->has_matrix, "matrix-only animated bone unsupported");
    Q local;
    memcpy(local.data(), (*it)->rotation, 16);
    q = qm(q, local);
  }
  return qn(q);
}
static J loadjson(cgltf_data *d) {
  return J::parse(d->json, d->json + d->json_size);
}
static cgltf_data *load(const char *path) {
  std::ifstream f(path, std::ios::binary | std::ios::ate);
  need(bool(f), "model open failed");
  need(f.tellg() > 0 && f.tellg() < 64 * 1024 * 1024,
       "model file size outside 1..64 MiB");
  cgltf_options o{};
  cgltf_data *d = nullptr;
  need(cgltf_parse_file(&o, path, &d) == cgltf_result_success,
       "glTF parse failed");
  try {
    need(d->file_type == cgltf_file_type_glb,
         "only embedded GLB/VRM supported");
    for (size_t i = 0; i < d->buffers_count; i++)
      need(!d->buffers[i].uri, "external buffers not permitted");
    for (size_t i = 0; i < d->images_count; i++)
      need(!d->images[i].uri, "external images not permitted");
    need(cgltf_load_buffers(&o, d, path) == cgltf_result_success,
         "buffer load failed");
    need(cgltf_validate(d) == cgltf_result_success, "glTF validation failed");
    need(d->nodes_count < 4096 && d->meshes_count < 256 &&
             d->materials_count < 256,
         "scene budget exceeded");
    return d;
  } catch (...) {
    cgltf_free(d);
    throw;
  }
}
static cgltf_accessor *attr(cgltf_primitive &p, cgltf_attribute_type t) {
  for (size_t i = 0; i < p.attributes_count; i++)
    if (p.attributes[i].type == t && p.attributes[i].index == 0)
      return p.attributes[i].data;
  return nullptr;
}
struct Vertex {
  float p[3], n[3], uv[2], j[4], w[4];
};
struct Morph {
  std::vector<std::array<float, 3>> p, n;
};
struct Part {
  size_t node;
  cgltf_primitive *prim;
  std::vector<Vertex> base, working;
  std::vector<Morph> morph;
  std::vector<unsigned> indices;
  GLuint vao = 0, vbo = 0, ibo = 0;
};
struct Bind {
  int node = -1, mesh = -1, index = 0;
  float weight = 0;
};
struct Pose {
  float t[3], q[4], s[3];
};
struct Constraint {
  int source = -1, type = 0, axis = 0;
  float sign = 1, weight = 1;
};
struct Scene {
  cgltf_data *d = nullptr;
  J json;
  bool legacy = false;
  std::vector<Part> parts;
  std::vector<GLuint> textures;
  std::vector<Pose> rest;
  std::vector<M> world;
  std::vector<Constraint> constraint;
  std::vector<bool> active;
  std::map<std::string, int> bones;
  std::map<std::string, std::vector<Bind>> expressions;
  size_t triangles = 0, vertices = 0, textureBytes = 0, morphBytes = 0;
  explicit Scene(const char *path) {
    d = load(path);
    json = loadjson(d);
    auto ex = json.value("extensions", J::object());
    legacy = ex.contains("VRM");
    need(legacy || ex.contains("VRMC_vrm"), "VRM extension required");
    active.resize(d->nodes_count);
    auto selected =
        d->scene ? d->scene : (d->scenes_count ? &d->scenes[0] : nullptr);
    need(selected, "default scene required");
    std::vector<cgltf_node *> pending(selected->nodes,
                                      selected->nodes + selected->nodes_count);
    while (!pending.empty()) {
      auto *node = pending.back();
      pending.pop_back();
      size_t index = size_t(node - d->nodes);
      if (active[index])
        continue;
      active[index] = true;
      for (size_t k = 0; k < node->children_count; k++)
        pending.push_back(node->children[k]);
    }
    for (size_t i = 0; i < d->nodes_count; i++) {
      Pose p;
      memcpy(p.t, d->nodes[i].translation, sizeof p.t);
      memcpy(p.q, d->nodes[i].rotation, sizeof p.q);
      memcpy(p.s, d->nodes[i].scale, sizeof p.s);
      rest.push_back(p);
    }
    world.resize(d->nodes_count);
    if (legacy) {
      auto v = ex["VRM"];
      if (v.contains("humanoid"))
        for (auto &b : v["humanoid"]["humanBones"])
          bones[b["bone"].get<std::string>()] = b["node"].get<int>();
      if (v.contains("blendShapeMaster"))
        for (auto &g : v["blendShapeMaster"]["blendShapeGroups"]) {
          std::string key = g.value("presetName", "");
          for (auto &b : g["binds"])
            expressions[key].push_back({-1, b["mesh"].get<int>(),
                                        b["index"].get<int>(),
                                        b["weight"].get<float>() / 100});
        }
    } else {
      auto v = ex["VRMC_vrm"];
      for (auto &b : v["humanoid"]["humanBones"].items())
        bones[b.key()] = b.value()["node"].get<int>();
      if (v.contains("expressions")) {
        auto presets = v["expressions"].value("preset", J::object());
        for (auto &g : presets.items()) {
          expressions[g.key()];
          for (auto &b : g.value().value("morphTargetBinds", J::array()))
            expressions[g.key()].push_back({b["node"].get<int>(), -1,
                                            b["index"].get<int>(),
                                            b["weight"].get<float>()});
        }
      }
    }
    for (auto &b : bones)
      need(b.second >= 0 && size_t(b.second) < d->nodes_count,
           "invalid humanoid bone");
    constraint.resize(d->nodes_count);
    for (size_t i = 0; i < d->nodes_count; i++) {
      auto ext = json["nodes"][i].value("extensions", J::object());
      if (!ext.contains("VRMC_node_constraint"))
        continue;
      auto c = ext["VRMC_node_constraint"]["constraint"];
      auto &x = constraint[i];
      std::string kind = c.contains("rotation") ? "rotation"
                         : c.contains("roll")   ? "roll"
                                                : "aim";
      auto v = c.at(kind);
      x.source = v.at("source").get<int>();
      need(x.source >= 0 && size_t(x.source) < d->nodes_count,
           "constraint source invalid");
      x.weight = v.value("weight", 1.f);
      need(x.weight >= 0 && x.weight <= 1, "constraint weight invalid");
      x.type = kind == "rotation" ? 1 : kind == "roll" ? 2 : 3;
      if (x.type == 2) {
        std::string a = v.at("rollAxis");
        need(a == "X" || a == "Y" || a == "Z", "roll axis invalid");
        x.axis = a == "X" ? 0 : a == "Y" ? 1 : 2;
      }
      if (x.type == 3) {
        std::string a = v.at("aimAxis");
        need(a.size() > 0, "aim axis invalid");
        x.axis = a.back() == 'X' ? 0 : a.back() == 'Y' ? 1 : 2;
        x.sign = a.find("Negative") == 0 ? -1 : 1;
      }
    }
  }
  ~Scene() {
    for (auto &p : parts) {
      glDeleteBuffers(1, &p.vbo);
      glDeleteBuffers(1, &p.ibo);
      glDeleteVertexArrays(1, &p.vao);
    }
    for (auto t : textures)
      glDeleteTextures(1, &t);
    if (d)
      cgltf_free(d);
  }
  void transforms() {
    std::vector<int> mark(d->nodes_count);
    for (size_t i = 0; i < d->nodes_count; i++)
      calc(i, mark);
  }
  void calc(size_t i, std::vector<int> &mark) {
    if (mark[i] == 2)
      return;
    need(mark[i] != 1, "node cycle");
    mark[i] = 1;
    M local;
    cgltf_node_transform_local(&d->nodes[i], local.data());
    auto p = d->nodes[i].parent;
    if (p) {
      size_t k = p - d->nodes;
      calc(k, mark);
      world[i] = mul(world[k], local);
    } else
      world[i] = local;
    mark[i] = 2;
  }
  void upload() {
    for (size_t i = 0; i < d->textures_count; i++) {
      auto &tex = d->textures[i];
      need(tex.image && tex.image->buffer_view,
           "embedded PNG/JPEG texture required");
      auto *v = tex.image->buffer_view;
      need(v->size < 32 * 1024 * 1024, "texture file budget exceeded");
      auto *src = cgltf_buffer_view_data(v);
      int w = 0, h = 0, c = 0;
      need(src && stbi_info_from_memory(src, int(v->size), &w, &h, &c),
           "texture header failed");
      need(w > 0 && h > 0 && w <= 4096 && h <= 4096,
           "texture dimensions exceed 4096");
      textureBytes += size_t(w) * h * 4;
      need(textureBytes <= 128 * 1024 * 1024, "texture memory budget exceeded");
      auto *pix = stbi_load_from_memory(src, int(v->size), &w, &h, &c, 4);
      need(pix, "texture decode failed");
      GLuint t;
      glGenTextures(1, &t);
      textures.push_back(t);
      glBindTexture(GL_TEXTURE_2D, t);
      glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA, w, h, 0, GL_RGBA,
                   GL_UNSIGNED_BYTE, pix);
      stbi_image_free(pix);
      glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER,
                      GL_LINEAR_MIPMAP_LINEAR);
      glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
      glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S,
                      tex.sampler ? tex.sampler->wrap_s : GL_REPEAT);
      glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T,
                      tex.sampler ? tex.sampler->wrap_t : GL_REPEAT);
      glGenerateMipmap(GL_TEXTURE_2D);
    }
    for (size_t i = 0; i < d->nodes_count; i++) {
      auto &node = d->nodes[i];
      if (!active[i] || !node.mesh)
        continue;
      if (node.skin)
        need(node.skin->joints_count <= MaxJoints, "skin exceeds 192 joints");
      for (size_t k = 0; k < node.mesh->primitives_count; k++) {
        auto &p = node.mesh->primitives[k];
        need(p.type == cgltf_primitive_type_triangles &&
                 !p.has_draco_mesh_compression,
             "only uncompressed triangle meshes supported");
        auto *pos = attr(p, cgltf_attribute_type_position);
        need(pos && pos->count <= 250000,
             "position accessor or vertex budget invalid");
        Part x;
        x.node = i;
        x.prim = &p;
        x.base.resize(pos->count);
        vertices += pos->count;
        need(vertices <= 500000, "total vertex budget exceeded");
        auto *normal = attr(p, cgltf_attribute_type_normal),
             *uv = attr(p, cgltf_attribute_type_texcoord),
             *j = attr(p, cgltf_attribute_type_joints),
             *wt = attr(p, cgltf_attribute_type_weights);
        for (size_t a = 0; a < pos->count; a++) {
          auto &v = x.base[a];
          need(cgltf_accessor_read_float(pos, a, v.p, 3),
               "position read failed");
          if (normal)
            need(cgltf_accessor_read_float(normal, a, v.n, 3),
                 "normal read failed");
          else
            v.n[2] = 1;
          if (uv)
            need(cgltf_accessor_read_float(uv, a, v.uv, 2), "uv read failed");
          if (j && wt) {
            cgltf_uint joints[4];
            need(cgltf_accessor_read_uint(j, a, joints, 4),
                 "joint read failed");
            need(cgltf_accessor_read_float(wt, a, v.w, 4),
                 "weights read failed");
            float sum = 0;
            for (int b = 0; b < 4; b++) {
              v.j[b] = float(joints[b]);
              need(!node.skin || joints[b] < node.skin->joints_count,
                   "joint outside skin");
              sum += v.w[b];
            }
            if (sum > 0)
              for (float &w : v.w)
                w /= sum;
            else
              v.w[0] = 1;
          } else
            v.w[0] = 1;
        }
        size_t count = p.indices ? p.indices->count : pos->count;
        need(count % 3 == 0, "triangle index count");
        for (size_t a = 0; a < count; a++) {
          unsigned ix = p.indices
                            ? unsigned(cgltf_accessor_read_index(p.indices, a))
                            : unsigned(a);
          need(ix < pos->count, "index outside vertices");
          x.indices.push_back(ix);
        }
        triangles += count / 3;
        need(p.targets_count <= 128, "too many morph targets");
        for (size_t m = 0; m < p.targets_count; m++) {
          Morph morph;
          for (size_t at = 0; at < p.targets[m].attributes_count; at++) {
            auto &a = p.targets[m].attributes[at];
            auto *dest = a.type == cgltf_attribute_type_position ? &morph.p
                         : a.type == cgltf_attribute_type_normal ? &morph.n
                                                                 : nullptr;
            if (!dest)
              continue;
            morphBytes += pos->count * sizeof(std::array<float, 3>);
            need(morphBytes <= 192 * 1024 * 1024,
                 "morph memory budget exceeded");
            dest->resize(pos->count);
            need(a.data->count == pos->count, "morph size mismatch");
            need(cgltf_accessor_unpack_floats(a.data, (*dest)[0].data(),
                                              pos->count * 3) == pos->count * 3,
                 "morph read failed");
          }
          x.morph.push_back(std::move(morph));
        }
        glGenVertexArrays(1, &x.vao);
        glBindVertexArray(x.vao);
        glGenBuffers(1, &x.vbo);
        glBindBuffer(GL_ARRAY_BUFFER, x.vbo);
        glBufferData(GL_ARRAY_BUFFER, x.base.size() * sizeof(Vertex),
                     x.base.data(), GL_DYNAMIC_DRAW);
        int n[] = {3, 3, 2, 4, 4};
        size_t offsets[] = {offsetof(Vertex, p), offsetof(Vertex, n),
                            offsetof(Vertex, uv), offsetof(Vertex, j),
                            offsetof(Vertex, w)};
        for (int a = 0; a < 5; a++) {
          glEnableVertexAttribArray(a);
          glVertexAttribPointer(a, n[a], GL_FLOAT, GL_FALSE, sizeof(Vertex),
                                (void *)offsets[a]);
        }
        glGenBuffers(1, &x.ibo);
        glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, x.ibo);
        glBufferData(GL_ELEMENT_ARRAY_BUFFER, x.indices.size() * 4,
                     x.indices.data(), GL_STATIC_DRAW);
        parts.push_back(std::move(x));
      }
    }
    need(!parts.empty(), "no meshes");
    transforms();
  }
  void rotateBone(const std::string &name, float angle, int axis) {
    auto it = bones.find(name);
    if (it == bones.end())
      return;
    auto &n = d->nodes[it->second];
    Q q = {0, 0, 0, cosf(angle / 2)};
    q[axis] = sinf(angle / 2);
    Q parent = n.parent ? worldq(n.parent) : Q{0, 0, 0, 1};
    Q current;
    memcpy(current.data(), n.rotation, 16);
    auto changed = qm(qm(qm(qi(parent), q), parent), current);
    memcpy(n.rotation, changed.data(), 16);
    n.has_rotation = 1;
  }
  void reset() {
    for (size_t i = 0; i < rest.size(); i++) {
      memcpy(d->nodes[i].translation, rest[i].t, 12);
      memcpy(d->nodes[i].rotation, rest[i].q, 16);
      memcpy(d->nodes[i].scale, rest[i].s, 12);
    }
  }
  void animate(double t, bool idle) {
    reset();
    if (idle) {
      rotateBone("head", .10f * sinf(t * .8), 1);
      rotateBone("neck", .035f * sinf(t * 1.3), 2);
      rotateBone("chest", .025f * sinf(t * 2), 0);
      rotateBone("leftUpperArm", legacy ? .9f : -.9f, 2);
      rotateBone("rightUpperArm", legacy ? -.9f : .9f, 2);
    }
    transforms();
  }
  void solveConstraint(size_t i, std::vector<int> &mark) {
    if (mark[i] == 2)
      return;
    need(mark[i] != 1, "constraint dependency cycle");
    mark[i] = 1;
    auto &c = constraint[i];
    if (c.source >= 0) {
      solveConstraint(size_t(c.source), mark);
      auto &node = d->nodes[i];
      Q dst;
      memcpy(dst.data(), rest[i].q, 16);
      Q source;
      memcpy(source.data(), d->nodes[c.source].rotation, 16);
      Q srcRest;
      memcpy(srcRest.data(), rest[c.source].q, 16);
      Q target = dst;
      if (c.type == 1)
        target = qm(qm(dst, qi(srcRest)), source);
      if (c.type == 2) {
        auto delta = qm(qm(qm(qi(dst), source), qi(srcRest)), dst);
        V axis{};
        axis[c.axis] = 1;
        target = qm(qm(dst, fromTo(applyq(delta, axis), axis)), delta);
      }
      if (c.type == 3) {
        for (auto *p = node.parent; p; p = p->parent)
          solveConstraint(size_t(p - d->nodes), mark);
        for (auto *p = d->nodes[c.source].parent; p; p = p->parent)
          solveConstraint(size_t(p - d->nodes), mark);
        Q parent = node.parent ? worldq(node.parent) : Q{0, 0, 0, 1};
        M a, b;
        cgltf_node_transform_world(&node, a.data());
        cgltf_node_transform_world(&d->nodes[c.source], b.data());
        V direction = {b[12] - a[12], b[13] - a[13], b[14] - a[14]}, axis{};
        axis[c.axis] = c.sign;
        target = qm(
            qm(qm(qi(parent), fromTo(applyq(qm(parent, dst), axis), direction)),
               parent),
            dst);
      }
      slerp(node.rotation, dst.data(), target.data(), c.weight);
      node.has_rotation = 1;
    }
    mark[i] = 2;
  }
  void constraints() {
    std::vector<int> mark(d->nodes_count);
    for (size_t i = 0; i < constraint.size(); i++)
      solveConstraint(i, mark);
    transforms();
  }
  void morphs(float mouth, float blink) {
    for (auto &p : parts) {
      std::vector<float> weights(p.morph.size());
      auto &node = d->nodes[p.node];
      auto *mesh = node.mesh;
      for (size_t k = 0; k < weights.size(); k++)
        weights[k] = k < node.weights_count    ? node.weights[k]
                     : k < mesh->weights_count ? mesh->weights[k]
                                               : 0;
      for (auto &ex : expressions) {
        float val = ex.first == "aa" || ex.first == "a" ? mouth
                    : ex.first == "blink"               ? blink
                                                        : 0;
        if (val == 0)
          continue;
        for (auto &b : ex.second)
          if ((b.node == int(p.node) || b.mesh == int(mesh - d->meshes)) &&
              b.index >= 0 && size_t(b.index) < weights.size())
            weights[b.index] += val * b.weight;
      }
      if (p.morph.empty())
        continue;
      p.working = p.base;
      for (size_t k = 0; k < weights.size(); k++) {
        float w = std::clamp(weights[k], 0.f, 1.f);
        if (w == 0)
          continue;
        auto &m = p.morph[k];
        for (size_t i = 0; i < p.working.size(); i++)
          for (int c = 0; c < 3; c++) {
            if (!m.p.empty())
              p.working[i].p[c] += w * m.p[i][c];
            if (!m.n.empty())
              p.working[i].n[c] += w * m.n[i][c];
          }
      }
      glBindBuffer(GL_ARRAY_BUFFER, p.vbo);
      glBufferSubData(GL_ARRAY_BUFFER, 0, p.working.size() * sizeof(Vertex),
                      p.working.data());
    }
  }
};
// Expose only independent, active TRS joints. Aliased humanoid names must not
// reset the same node twice or create coincident competing controls.
static int poseNode(const Scene &scene,int id){
 if(id<0||id>=poseCount)return -1;
 auto it=scene.bones.find(poseNames[id]);if(it==scene.bones.end())return -1;
 int node=it->second;
 if(!scene.active[node]||scene.d->nodes[node].has_matrix)return -1;
 for(int before=0;before<id;before++){auto b=scene.bones.find(poseNames[before]);if(b!=scene.bones.end()&&b->second==node)return -1;}
 return node;
}
// VRMA humanoid retargeting. Rotation deltas are conjugated from source bind
// coordinates to target bind coordinates; hips translation uses height ratio.
// Expression/lookAt tracks and spring physics are outside this renderer
// contract.
struct Motion {
  struct Track {
    int target;
    cgltf_animation_path_type path;
    cgltf_interpolation_type interpolation;
    std::vector<float> time, values;
    Q basis, sourceRest;
    V targetRest{};
    float scale = 1;
  };
  cgltf_data *d = nullptr;
  std::vector<Track> tracks;
  float duration = 0;
  Motion(const char *path, Scene &scene) {
    if (!path)
      return;
    d = load(path);
    J j = loadjson(d);
    need(j.contains("extensions") &&
             j["extensions"].contains("VRMC_vrm_animation"),
         "animation requires VRMC_vrm_animation");
    need(d->animations_count > 0, "VRMA has no animation");
    auto human =
        j["extensions"]["VRMC_vrm_animation"]["humanoid"]["humanBones"];
    std::map<int, std::string> map;
    for (auto &x : human.items()) {
      int n = x.value()["node"].get<int>();
      need(n >= 0 && size_t(n) < d->nodes_count, "VRMA bone node invalid");
      map[n] = x.key();
    }
    auto &a = d->animations[0];
    bool sourceSupported=false;
    for (size_t i = 0; i < a.channels_count; i++) {
      auto &c = a.channels[i];
      if (!c.target_node)
        continue;
      auto it = map.find(int(c.target_node - d->nodes));
      if (it == map.end())
        continue;
      if (c.target_path != cgltf_animation_path_type_rotation &&
          !(it->second == "hips" &&
            c.target_path == cgltf_animation_path_type_translation))
        continue;
      sourceSupported=true;
      auto dst = scene.bones.find(it->second);
      if (dst == scene.bones.end())
        continue;
      auto *s = c.sampler;
      need(s && s->input && s->output && s->input->count > 0 &&
               s->input->count < 100000,
           "VRMA sampler budget invalid");
      Track t;
      t.target = dst->second;
      t.path = c.target_path;
      t.interpolation = s->interpolation;
      int components = t.path == cgltf_animation_path_type_rotation ? 4 : 3;
      t.time.resize(s->input->count);
      need(cgltf_accessor_unpack_floats(s->input, t.time.data(),
                                        t.time.size()) == t.time.size(),
           "VRMA times read failed");
      for (size_t k = 0; k < t.time.size(); k++)
        need(std::isfinite(t.time[k]) && t.time[k] >= 0 &&
                 (k == 0 || t.time[k] > t.time[k - 1]),
             "VRMA time sequence invalid");
      duration = std::max(duration, t.time.back());
      size_t mult =
          t.interpolation == cgltf_interpolation_type_cubic_spline ? 3 : 1;
      need(s->output->count == t.time.size() * mult, "VRMA key count invalid");
      t.values.resize(t.time.size() * mult * components);
      need(cgltf_accessor_unpack_floats(s->output, t.values.data(),
                                        t.values.size()) == t.values.size(),
           "VRMA values read failed");
      Q targetWorld = worldq(&scene.d->nodes[t.target]);
      Q sourceWorld = worldq(c.target_node);
      Q facing = scene.legacy ? Q{0, 1, 0, 0} : Q{0, 0, 0, 1};
      t.basis = qm(qm(qi(targetWorld), facing), sourceWorld);
      memcpy(t.sourceRest.data(), c.target_node->rotation, 16);
      memcpy(t.targetRest.data(), scene.rest[t.target].t, 12);
      if (t.path == cgltf_animation_path_type_translation) {
        memcpy(t.sourceRest.data(), c.target_node->translation, 12);
        M sw, tw;
        cgltf_node_transform_world(c.target_node, sw.data());
        cgltf_node_transform_world(&scene.d->nodes[t.target], tw.data());
        if (fabs(sw[13]) > .001)
          t.scale = fabs(tw[13] / sw[13]);
        Q parent = scene.d->nodes[t.target].parent
                       ? worldq(scene.d->nodes[t.target].parent)
                       : Q{0, 0, 0, 1};
        Q sp = c.target_node->parent ? worldq(c.target_node->parent)
                                     : Q{0, 0, 0, 1};
        t.basis = qm(qm(qi(parent), facing), sp);
      }
      tracks.push_back(std::move(t));
    }
    need(sourceSupported, "no supported humanoid tracks");
    // A valid default motion may have no targets on an incomplete/non-humanoid
    // model. Keep ordinary rendering/framing usable instead of rejecting it.
    if(tracks.empty()){puts("VRMA_UNMAPPED=1");return;}
    need(duration > 0, "no supported humanoid tracks");
    printf("VRMA_TRACKS=%zu\nVRMA_DURATION_S=%.3f\n", tracks.size(), duration);
  }
  ~Motion() {
    if (d)
      cgltf_free(d);
  }
  void apply(Scene &scene, double seconds) {
    if (!d || tracks.empty())
      return;
    float time = float(fmod(seconds, duration));
    for (auto &t : tracks) {
      int components = t.path == cgltf_animation_path_type_rotation ? 4 : 3;
      size_t right =
          std::upper_bound(t.time.begin(), t.time.end(), time) - t.time.begin();
      size_t a = right == 0 ? 0 : right - 1,
             b = std::min(a + 1, t.time.size() - 1);
      float dt = t.time[b] - t.time[a],
            u = dt > 0 ? std::clamp((time - t.time[a]) / dt, 0.f, 1.f) : 0;
      float out[4] = {0, 0, 0, 1};
      size_t mult =
          t.interpolation == cgltf_interpolation_type_cubic_spline ? 3 : 1;
      const float *v0 =
                      &t.values[(a * mult + (mult == 3 ? 1 : 0)) * components],
                  *v1 =
                      &t.values[(b * mult + (mult == 3 ? 1 : 0)) * components];
      if (t.interpolation == cgltf_interpolation_type_step || a == b)
        memcpy(out, v0, components * 4);
      else if (mult == 3) {
        float u2 = u * u, u3 = u2 * u;
        for (int k = 0; k < components; k++)
          out[k] = (2 * u3 - 3 * u2 + 1) * v0[k] +
                   (u3 - 2 * u2 + u) * dt * v0[components + k] +
                   (-2 * u3 + 3 * u2) * v1[k] +
                   (u3 - u2) * dt * v1[-components + k];
      } else if (components == 4)
        slerp(out, v0, v1, u);
      else
        for (int k = 0; k < 3; k++)
          out[k] = v0[k] * (1 - u) + v1[k] * u;
      auto &n = scene.d->nodes[t.target];
      if (components == 4) {
        Q q;
        memcpy(q.data(), out, 16);
        Q rest;
        memcpy(rest.data(), scene.rest[t.target].q, 16);
        auto delta = qm(qi(t.sourceRest), qn(q));
        auto result = qm(qm(qm(rest, t.basis), delta), qi(t.basis));
        memcpy(n.rotation, result.data(), 16);
        n.has_rotation = 1;
      } else {
        V delta{};
        for (int k = 0; k < 3; k++)
          delta[k] = out[k] - t.sourceRest[k];
        delta = applyq(t.basis, delta);
        for (int k = 0; k < 3; k++)
          n.translation[k] = t.targetRest[k] + delta[k] * t.scale;
        n.has_translation = 1;
      }
    }
    scene.transforms();
  }
};
static GLuint shader(GLenum kind, const char *src) {
  GLuint s = glCreateShader(kind);
  glShaderSource(s, 1, &src, nullptr);
  glCompileShader(s);
  GLint ok;
  glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
  if (!ok) {
    char b[2048];
    glGetShaderInfoLog(s, sizeof b, nullptr, b);
    throw std::runtime_error(b);
  }
  return s;
}
static GLuint program() {
  const char *vs = R"(#version 300 es
precision highp float;
layout(location=0) in vec3 p; layout(location=1) in vec3 n; layout(location=2) in vec2 uv; layout(location=3) in vec4 joints; layout(location=4) in vec4 weights;
uniform mat4 vp; uniform mat4 model; uniform mat4 bones[192];uniform int skinned;
out vec3 normal;out vec2 texcoord;
void main(){mat4 skin=mat4(1.);if(skinned==1)skin=weights.x*bones[int(joints.x)]+weights.y*bones[int(joints.y)]+weights.z*bones[int(joints.z)]+weights.w*bones[int(joints.w)];mat4 m=model*skin;normal=mat3(m)*n;texcoord=uv;gl_Position=vp*m*vec4(p,1.);})";
  const char *fs = R"(#version 300 es
precision highp float;
in vec3 normal;in vec2 texcoord;uniform sampler2D image;uniform vec4 color;uniform int textured;uniform int unlit;uniform float cutoff;out vec4 outColor;
void main(){vec4 c=color;if(textured==1)c*=texture(image,texcoord);if(c.a<cutoff)discard;float light=1.;if(unlit==0){float d=dot(normalize(normal),normalize(vec3(-.3,.8,1.)));light=.68+.32*smoothstep(-.1,.45,d);}outColor=vec4(c.rgb*light,c.a);})";
  GLuint v = shader(GL_VERTEX_SHADER, vs), f = shader(GL_FRAGMENT_SHADER, fs),
         p = glCreateProgram();
  glAttachShader(p, v);
  glAttachShader(p, f);
  glLinkProgram(p);
  glDeleteShader(v);
  glDeleteShader(f);
  GLint ok;
  glGetProgramiv(p, GL_LINK_STATUS, &ok);
  need(ok, "shader link failed");
  return p;
}

// Backgrounds arrive only as bounded, validated straight-alpha RGBA snapshots.
// The default clear color remains visible beneath transparent pixels.
struct Background {
 GLuint texture=0, prog=0, vao=0;
 int width=0,height=0;
 explicit Background(const char *name) {
  if (!name) return;
  FILE *f=fopen(name,"rb");
  if (!f) {puts("AVATAR_BACKGROUND_ERROR");return;}
  unsigned char header[12];
  bool ok=fread(header,1,12,f)==12 && memcmp(header,"BG01",4)==0;
  auto u32=[](const unsigned char *b)->uint32_t{return uint32_t(b[0]) | (uint32_t(b[1])<<8) | (uint32_t(b[2])<<16) | (uint32_t(b[3])<<24);};
  if(ok){width=int(u32(header+4));height=int(u32(header+8));}
  ok=ok && width>0 && width<=2048 && height>0 && height<=2048;
  std::vector<unsigned char> pixels;
  if(ok){pixels.resize(size_t(width)*height*4);ok=fread(pixels.data(),1,pixels.size(),f)==pixels.size() && fgetc(f)==EOF;}
  fclose(f);
  if(!ok){puts("AVATAR_BACKGROUND_ERROR");return;}
  const char *vs=R"(#version 300 es
precision highp float;
out vec2 uv;
void main(){vec2 p=vec2(float((gl_VertexID<<1)&2),float(gl_VertexID&2));uv=vec2(p.x,1.-p.y);gl_Position=vec4(p*2.-1.,0.,1.);})";
  const char *fs=R"(#version 300 es
precision highp float;
in vec2 uv;uniform sampler2D image;uniform vec2 crop;out vec4 outColor;
void main(){outColor=texture(image,(uv-.5)*crop+.5);})";
  GLuint v=shader(GL_VERTEX_SHADER,vs),fshader=shader(GL_FRAGMENT_SHADER,fs);
  prog=glCreateProgram();glAttachShader(prog,v);glAttachShader(prog,fshader);glLinkProgram(prog);
  glDeleteShader(v);glDeleteShader(fshader);
  GLint linked=0;glGetProgramiv(prog,GL_LINK_STATUS,&linked);
  need(linked,"background shader link failed");
  glGenVertexArrays(1,&vao);
  glGenTextures(1,&texture);glActiveTexture(GL_TEXTURE0);glBindTexture(GL_TEXTURE_2D,texture);
  glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_MIN_FILTER,GL_LINEAR);glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_MAG_FILTER,GL_LINEAR);
  glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_WRAP_S,GL_CLAMP_TO_EDGE);glTexParameteri(GL_TEXTURE_2D,GL_TEXTURE_WRAP_T,GL_CLAMP_TO_EDGE);
  glTexImage2D(GL_TEXTURE_2D,0,GL_RGBA,width,height,0,GL_RGBA,GL_UNSIGNED_BYTE,pixels.data());
  if(glGetError()!=GL_NO_ERROR){glDeleteTextures(1,&texture);texture=0;puts("AVATAR_BACKGROUND_ERROR");}
 }
 void draw(int w,int h) {
  if(!texture)return;
  float imageAspect=float(width)/height,viewAspect=float(w)/h;
  float cx=1,cy=1;if(imageAspect>viewAspect)cx=viewAspect/imageAspect;else cy=imageAspect/viewAspect;
  glUseProgram(prog);glUniform1i(glGetUniformLocation(prog,"image"),0);
  glUniform2f(glGetUniformLocation(prog,"crop"),cx,cy);
  glDisable(GL_DEPTH_TEST);glDepthMask(GL_FALSE);glDisable(GL_CULL_FACE);
  glEnable(GL_BLEND);glBlendFunc(GL_SRC_ALPHA,GL_ONE_MINUS_SRC_ALPHA);
  glActiveTexture(GL_TEXTURE0);glBindTexture(GL_TEXTURE_2D,texture);glBindVertexArray(vao);
  glDrawArrays(GL_TRIANGLES,0,3);
  glDisable(GL_BLEND);glDepthMask(GL_TRUE);glEnable(GL_DEPTH_TEST);
 }
 ~Background(){if(texture)glDeleteTextures(1,&texture);if(vao)glDeleteVertexArrays(1,&vao);if(prog)glDeleteProgram(prog);}
};

// Two round transparent apertures expose the application's accessible controls.
// The native layer never handles input, changes audio or owns conversation state.
struct ControlCutouts {
 GLuint prog=0,vao=0;
 ControlCutouts(){
 const char *vs=R"(#version 300 es
void main(){vec2 p=vec2(float((gl_VertexID<<1)&2),float(gl_VertexID&2));gl_Position=vec4(p*2.-1.,0.,1.);})";
 const char *fs=R"(#version 300 es
precision highp float;uniform vec2 center;out vec4 outColor;
void main(){if(distance(gl_FragCoord.xy,center)>39.)discard;outColor=vec4(0.);})";
 GLuint v=shader(GL_VERTEX_SHADER,vs),f=shader(GL_FRAGMENT_SHADER,fs);
 prog=glCreateProgram();glAttachShader(prog,v);glAttachShader(prog,f);glLinkProgram(prog);glDeleteShader(v);glDeleteShader(f);
 GLint linked=0;glGetProgramiv(prog,GL_LINK_STATUS,&linked);need(linked,"control shader link failed");glGenVertexArrays(1,&vao);
 }
 void draw(int w,int h){
 glViewport(0,0,w,h);glDisable(GL_DEPTH_TEST);glDepthMask(GL_FALSE);glDisable(GL_BLEND);glDisable(GL_CULL_FACE);
 glUseProgram(prog);glBindVertexArray(vao);
 glEnable(GL_SCISSOR_TEST);
 for(int dx:{-48,48}){glScissor(w/2+dx-39,17,78,78);glUniform2f(glGetUniformLocation(prog,"center"),w*.5f+dx,56);glDrawArrays(GL_TRIANGLES,0,3);}
 glDisable(GL_SCISSOR_TEST);
 glDepthMask(GL_TRUE);glEnable(GL_DEPTH_TEST);
 }
 ~ControlCutouts(){if(vao)glDeleteVertexArrays(1,&vao);if(prog)glDeleteProgram(prog);}
};

struct PoseOverlay {
 GLuint prog=0,vao=0;
 PoseOverlay(){
 const char *vs=R"(#version 300 es
 void main(){vec2 p=vec2(float((gl_VertexID<<1)&2),float(gl_VertexID&2));gl_Position=vec4(p*2.-1.,0.,1.);})";
 const char *fs=R"(#version 300 es
 precision highp float;uniform vec2 start,end;uniform vec4 color;uniform float radius;out vec4 outColor;
 void main(){vec2 d=end-start;float t=clamp(dot(gl_FragCoord.xy-start,d)/max(dot(d,d),.01),0.,1.);float line=distance(gl_FragCoord.xy,start+t*d);float tip=distance(gl_FragCoord.xy,end);if(line>3.&&tip>radius)discard;outColor=color;})";
 GLuint v=shader(GL_VERTEX_SHADER,vs),f=shader(GL_FRAGMENT_SHADER,fs);
 prog=glCreateProgram();glAttachShader(prog,v);glAttachShader(prog,f);glLinkProgram(prog);glDeleteShader(v);glDeleteShader(f);
 GLint ok=0;glGetProgramiv(prog,GL_LINK_STATUS,&ok);need(ok,"pose overlay shader failed");glGenVertexArrays(1,&vao);
 }
 void mark(float x,float y,float dx,float dy,float r,float red,float green,float blue,int w,int h){
 glUniform2f(glGetUniformLocation(prog,"start"),x,y);glUniform2f(glGetUniformLocation(prog,"end"),x+dx,y+dy);
 glUniform4f(glGetUniformLocation(prog,"color"),red,green,blue,1);glUniform1f(glGetUniformLocation(prog,"radius"),r);
 int left=std::max(0,int(std::min(x,x+dx)-r-4)),bottom=std::max(0,int(std::min(y,y+dy)-r-4));
 glScissor(left,bottom,std::max(0,std::min(w,int(std::max(x,x+dx)+r+4))-left),std::max(0,std::min(h,int(std::max(y,y+dy)+r+4))-bottom));
 glDrawArrays(GL_TRIANGLES,0,3);
 }
 void draw(Scene &scene,const M &view,int selected,int w,int h){
 glViewport(0,0,w,h);glDisable(GL_DEPTH_TEST);glDepthMask(GL_FALSE);glDisable(GL_BLEND);glDisable(GL_CULL_FACE);glEnable(GL_SCISSOR_TEST);
 glUseProgram(prog);glBindVertexArray(vao);
 for(int id=0;id<poseCount;id++){
 int node=poseNode(scene,id);if(node<0)continue;
 auto m=mul(view,scene.world[node]);float x=(m[12]*.5f+.5f)*w,y=(m[13]*.5f+.5f)*h;
 if(x<0||x>=w||y<0||y>=h||m[14]<-1||m[14]>1)continue;
 mark(x,y,0,0,id==selected?12:7,1,id==selected?.85f:1,.2f,w,h);
 }
 if(selected>=0 && selected<poseCount){
 auto it=scene.bones.find(poseNames[selected]);
 if(it!=scene.bones.end()){
 auto m=mul(view,scene.world[it->second]);float x=(m[12]*.5f+.5f)*w,y=(m[13]*.5f+.5f)*h;
 if(x>=0&&x<w&&y>=0&&y<h){
 mark(x,y,64,0,18,1,.25,.25,w,h);mark(x,y,0,64,18,.25,1,.25,w,h);mark(x,y,-45,-45,18,.3,.6,1,w,h);
 }}}
 glDisable(GL_SCISSOR_TEST);glDepthMask(GL_TRUE);glEnable(GL_DEPTH_TEST);
 }
 ~PoseOverlay(){if(vao)glDeleteVertexArrays(1,&vao);if(prog)glDeleteProgram(prog);}
};

struct EGL {
  EGLDisplay display = EGL_NO_DISPLAY;
  EGLContext context = EGL_NO_CONTEXT;
  EGLSurface surface = EGL_NO_SURFACE;
  EGL(int w, int h, void *window) {
    display = eglGetDisplay(EGL_DEFAULT_DISPLAY);
    need(display != EGL_NO_DISPLAY && eglInitialize(display, nullptr, nullptr),
         "EGL init failed");
    EGLint attrs[] = {EGL_SURFACE_TYPE,
                      window ? EGL_WINDOW_BIT : EGL_PBUFFER_BIT,
                      EGL_RENDERABLE_TYPE,
                      EGL_OPENGL_ES3_BIT,
                      EGL_RED_SIZE,
                      8,
                      EGL_GREEN_SIZE,
                      8,
                      EGL_BLUE_SIZE,
                      8,
                      EGL_ALPHA_SIZE,
                      8,
                      EGL_DEPTH_SIZE,
                      24,
                      EGL_NONE};
    EGLConfig cfg;
    EGLint count;
    need(eglChooseConfig(display, attrs, &cfg, 1, &count) && count,
         "EGL config failed");
    EGLint pb[] = {EGL_WIDTH, w, EGL_HEIGHT, h, EGL_NONE},
           ctx[] = {EGL_CONTEXT_CLIENT_VERSION, 3, EGL_NONE};
    context = eglCreateContext(display, cfg, EGL_NO_CONTEXT, ctx);
    surface = window ? eglCreateWindowSurface(
                           display, cfg, (EGLNativeWindowType)window, nullptr)
                     : eglCreatePbufferSurface(display, cfg, pb);
    need(surface != EGL_NO_SURFACE && context != EGL_NO_CONTEXT &&
             eglMakeCurrent(display, surface, surface, context),
         "EGL context/surface failed");
    eglSwapInterval(display, 1);
    printf("GPU=%s\nGL=%s\n", glGetString(GL_RENDERER),
           glGetString(GL_VERSION));
  }
  ~EGL() {
    if (display != EGL_NO_DISPLAY) {
      eglMakeCurrent(display, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
      if (surface != EGL_NO_SURFACE)
        eglDestroySurface(display, surface);
      if (context != EGL_NO_CONTEXT)
        eglDestroyContext(display, context);
      eglTerminate(display);
    }
  }
};
extern "C" void *avatar_window_open(int, int, int, int);
extern "C" void avatar_window_close();
static void screenshot(const char *path, int w, int h) {
  std::vector<unsigned char> pix(size_t(w) * h * 4);
  glReadPixels(0, 0, w, h, GL_RGBA, GL_UNSIGNED_BYTE, pix.data());
  FILE *f = fopen(path, "wb");
  need(f, "screenshot open failed");
  fprintf(f, "P6\n%d %d\n255\n", w, h);
  for (int y = h - 1; y >= 0; y--)
    for (int x = 0; x < w; x++)
      fwrite(&pix[(size_t(y) * w + x) * 4], 1, 3, f);
  fclose(f);
}
static std::vector<float> wavEnvelope(const char *path) {
  std::ifstream f(path, std::ios::binary);
  need(bool(f), "WAV open failed");
  char header[12] = {};
  f.read(header, 12);
  need(!memcmp(header, "RIFF", 4) && !memcmp(header + 8, "WAVE", 4),
       "WAV header invalid");
  unsigned rate = 0;
  uint16_t channels = 0, bits = 0, format = 0;
  std::vector<int16_t> pcm;
  while (f) {
    char tag[4];
    uint32_t len;
    f.read(tag, 4);
    f.read((char *)&len, 4);
    if (!f)
      break;
    need(len <= 64 * 1024 * 1024, "WAV chunk exceeds budget");
    std::vector<char> b(len);
    f.read(b.data(), len);
    need(bool(f), "truncated WAV");
    if (!memcmp(tag, "fmt ", 4)) {
      need(len >= 16, "WAV fmt invalid");
      memcpy(&format, b.data(), 2);
      memcpy(&channels, b.data() + 2, 2);
      memcpy(&rate, b.data() + 4, 4);
      memcpy(&bits, b.data() + 14, 2);
    } else if (!memcmp(tag, "data", 4)) {
      need(len % 2 == 0, "WAV PCM size invalid");
      pcm.resize(len / 2);
      memcpy(pcm.data(), b.data(), len);
    }
    if (len & 1)
      f.get();
  }
  need(format == 1 && bits == 16 && channels >= 1 && channels <= 2 &&
           rate >= 8000 && rate <= 96000 && !pcm.empty(),
       "requires PCM16 mono/stereo WAV");
  size_t block = std::max(1u, rate / 100) * channels;
  std::vector<float> out;
  for (size_t i = 0; i < pcm.size(); i += block) {
    double sum = 0;
    size_t n = std::min(block, pcm.size() - i);
    for (size_t k = 0; k < n; k++) {
      double a = pcm[i + k] / 32768.;
      sum += a * a;
    }
    out.push_back(std::clamp(float(sqrt(sum / n) * 5), 0.f, 1.f));
  }
  return out;
}
#include "control.hpp"
int main(int argc, char **argv) {
  int result = 1;
  const char *stage = "protocol_error";
  try {
    if (argc == 2 && std::string(argv[1]) == "--protocol-version") {
      puts("4");
      return 0;
    }
    if (argc == 2 && std::string(argv[1]) == "--self-test") {
      M a = identity();
      a[0] = 2;
      a[5] = 3;
      a[10] = 4;
      a[12] = 5;
      a[13] = -2;
      auto product = mul(a, inverse(a));
      auto id = identity();
      for (int k = 0; k < 16; k++)
        need(fabs(product[k] - id[k]) < 1e-5, "inverse/multiply self-test");
      auto rotated = applyq(fromTo({1, 0, 0}, {0, -1, 0}), {1, 0, 0});
      need(fabs(rotated[0]) < 1e-5 && fabs(rotated[1] + 1) < 1e-5,
           "aim self-test");
      auto opposite = applyq(fromTo({1, 0, 0}, {-1, 0, 0}), {1, 0, 0});
      need(fabs(opposite[0] + 1) < 1e-5, "opposite aim self-test");
      Q x = {0, 0, 0, 1}, y = {0, 0, 1, 0}, half;
      slerp(half.data(), x.data(), y.data(), .5);
      auto v = applyq(half, {1, 0, 0});
      need(fabs(v[0]) < 1e-5 && fabs(v[1] - 1) < 1e-5,
           "quaternion interpolation self-test");
      printf("SELF_TEST=PASS\n");
      return 0;
    }
    need(argc >= 2,
         "usage: echolocal-avatar model.vrm [--seconds N] [--display] [--out "
         "image.ppm] [--wav pcm16.wav] [--animation clip.vrma] [--static] "
         "[--mouth N]");
    int w = 640, h = 640, px = 640, py = 80, protocol = 0;
    double seconds = 5;
    bool show = false, idle = true, demoMouth = false, appMode = false, talkLayout = false, fullscreen = false;
    float fixedMouth = -1;
    const char *out = nullptr, *wav = nullptr, *animation = nullptr, *background = nullptr, *fullBackground = nullptr;
    for (int i = 2; i < argc; i++) {
      std::string a = argv[i];
      if (a == "--talk-layout")
        talkLayout = true;
      else if (a == "--fullscreen")
        fullscreen = true;
      else if (a == "--control")
        appMode = true;
      else if (a == "--display")
        show = true;
      else if (a == "--demo-mouth")
        demoMouth = true;
      else if (a == "--static")
        idle = false;
      else {
        need(i + 1 < argc, "missing option value");
        if (a == "--protocol")
          protocol = std::stoi(argv[++i]);
        else if (a == "--viewport") {
          need(avatarParseViewport(argv[++i],px,py,w,h),"invalid viewport");
        } else if (a == "--seconds")
          seconds = std::stod(argv[++i]);
        else if (a == "--out")
          out = argv[++i];
        else if (a == "--wav")
          wav = argv[++i];
        else if (a == "--fullscreen-background")
          fullBackground = argv[++i];
        else if (a == "--background")
          background = argv[++i];
        else if (a == "--animation")
          animation = argv[++i];
        else if (a == "--mouth")
          fixedMouth = std::stof(argv[++i]);
        else
          throw std::runtime_error("unknown option");
      }
    }
    need(std::isfinite(fixedMouth) && fixedMouth >= -1 && fixedMouth <= 1,
         "mouth must be -1..1");
    need(seconds > 0 && seconds <= 86400, "duration must be 1..86400 seconds");
    signal(SIGINT, stop);
    signal(SIGTERM, stop);
    need(!appMode || (protocol == 4 && !wav && !demoMouth && fixedMouth < 0),
         "app control cannot combine with other mouth sources");
    RendererControl controls(appMode);
    controls.fullscreen=fullscreen;
    double begin = now();
    stage = "gpu_failed";
    void *window = show ? avatar_window_open(w, h, px, py) : nullptr;
    need(!show || window, "display layer failed");
    {
      EGL egl(w, h, window);
      stage = "model_invalid";
      Scene scene(argv[1]);
      scene.upload();
      stage = "motion_invalid";
      Motion motion(animation, scene);
      stage = "gpu_failed";
      Background backdrop(background), fullBackdrop(fullBackground);
      ControlCutouts cutouts;
 PoseOverlay poseOverlay;
      GLuint prog = program();
      glUseProgram(prog);
      glUniform1i(glGetUniformLocation(prog, "image"), 0);
      glEnable(GL_DEPTH_TEST);
      glDepthFunc(GL_LEQUAL);
      glViewport(0, 0, w, h);
      // Fit the visible skinned bind pose, including authored morph
      // defaults/props.
      scene.morphs(0, 0);
      float lo[3] = {1e30, 1e30, 1e30}, hi[3] = {-1e30, -1e30, -1e30};
      for (auto &p : scene.parts) {
        auto &node = scene.d->nodes[p.node];
        std::vector<M> palette;
        if (node.skin)
          for (size_t k = 0; k < node.skin->joints_count; ++k) {
            M bind = identity();
            if (node.skin->inverse_bind_matrices)
              need(cgltf_accessor_read_float(node.skin->inverse_bind_matrices,
                                             k, bind.data(), 16),
                   "inverse bind read failed");
            palette.push_back(
                mul(scene.world[node.skin->joints[k] - scene.d->nodes], bind));
          }
        for (auto &v : p.working.empty() ? p.base : p.working)
          for (int r = 0; r < 3; ++r) {
            float value = 0;
            if (palette.empty()) {
              auto &m = scene.world[p.node];
              value = m[12 + r];
              for (int k = 0; k < 3; ++k)
                value += m[k * 4 + r] * v.p[k];
            } else
              for (int j = 0; j < 4; ++j) {
                auto &m = palette[size_t(v.j[j])];
                float a = m[12 + r];
                for (int k = 0; k < 3; ++k)
                  a += m[k * 4 + r] * v.p[k];
                value += v.w[j] * a;
              }
            need(std::isfinite(value), "non-finite model bounds");
            lo[r] = std::min(lo[r], value);
            hi[r] = std::max(hi[r], value);
          }
      }
      float extent =
          std::max(std::hypot(hi[0] - lo[0], hi[2] - lo[2]), hi[1] - lo[1]) *
          1.12f;
      need(extent > 0 && std::isfinite(extent), "invalid model bounds");
      M vp = identity();
      vp[0] = vp[5] = 2 / extent;
      vp[10] = -.1f;
      vp[12] = -(lo[0] + hi[0]) / extent;
      vp[13] = -(lo[1] + hi[1]) / extent;
      M rotate = identity();
      if (scene.legacy) {
        rotate[0] = rotate[10] = -1;
        vp[12] = -vp[12];
      }
      vp = mul(vp, rotate);
      glUniformMatrix4fv(glGetUniformLocation(prog, "vp"), 1, GL_FALSE,
                         vp.data());
      auto envelope = wav ? wavEnvelope(wav) : std::vector<float>{};
      double loaded = now() - begin;
      printf("VRM=%s\nNODES=%zu\nPARTS=%zu\nVERTICES=%zu\nTRIANGLES=%"
             "zu\nTEXTURE_RGBA_BYTES=%zu\nEXPRESSIONS=%zu\nLOAD_MS=%.3f\n",
             scene.legacy ? "0" : "1", scene.d->nodes_count, scene.parts.size(),
             scene.vertices, scene.triangles, scene.textureBytes,
             scene.expressions.size(), loaded * 1000);
      fflush(stdout);
      double start = now(), deadline = start, previous = 0;
      rusage baseline{};
      getrusage(RUSAGE_SELF, &baseline);
      std::vector<double> times, intervals;
      float mouth = 0;
      size_t frames = 0;
      bool captured = false;
      double nextStats = 0, nextJoints=0;
      while (!stopped && now() - start < seconds) {
        controls.read();
        if (controls.ended)
          break;
        const bool full = talkLayout && controls.fullscreen;
        const int viewX=talkLayout && !full?w/2:0, viewW=w-viewX;
        glViewport(viewX,0,viewW,h);
        M view =
            avatarFraming(mul(vp, avatarOrbit(appMode ? controls.yaw : 0,
                              appMode ? controls.pitch * (scene.legacy ? -1 : 1) : 0,
                              (lo[0] + hi[0]) * .5f, (lo[1] + hi[1]) * .5f,
                              (lo[2] + hi[2]) * .5f)), appMode ? controls.zoom : 1, appMode ? controls.panX : 0, appMode ? controls.panY : 0,viewW,h,full);
        // Overlay drawing binds its own shader; restore the model program before updating its uniforms.
        glUseProgram(prog);
        glUniformMatrix4fv(glGetUniformLocation(prog, "vp"), 1, GL_FALSE,
                           view.data());
        double frame = now(), t = frame - start;
        if (previous)
          intervals.push_back((frame - previous) * 1000);
        previous = frame;
        float target =
            fixedMouth >= 0 ? fixedMouth
            : envelope.empty()
                ? (demoMouth ? float(.5 + .5 * sin(t * 8)) : 0.f)
                : envelope[std::min(size_t(t * 100), envelope.size() - 1)];
        if (!envelope.empty() && t >= envelope.size() / 100.)
          target = 0;
        if (appMode)
          target = controls.mouth(t);
        mouth += (target - mouth) * (target > mouth ? .7f : .45f);
        float phase = float(fmod(t, 4.2));
        float blink = phase < .16f ? sinf(phase / .16f * 3.14159265f) : 0;
        scene.animate(t, appMode ? (controls.idle && !controls.poseActive) : (idle && !animation));
        if (!appMode || (controls.animation && !controls.poseActive))
          motion.apply(scene, t);
        if(appMode && controls.poseActive){
          for(int id=0;id<poseCount;id++){
            int index=poseNode(scene,id);if(index<0)continue;
            auto &node=scene.d->nodes[index];
            Q q;memcpy(q.data(),scene.rest[index].q,16);
            for(int axis=0;axis<3;axis++){float angle=controls.pose[id][axis]*3.14159265f/180;Q delta={0,0,0,cosf(angle/2)};delta[axis]=sinf(angle/2);q=qm(q,delta);}
            memcpy(node.rotation,q.data(),16);node.has_rotation=1;
          }
          scene.transforms();
        }else scene.constraints();
        scene.morphs(mouth, (appMode ? controls.blink : idle) ? blink : 0);
        glDisable(GL_SCISSOR_TEST);
        glClearColor(0,0,0,0);
        glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);
        glEnable(GL_SCISSOR_TEST);glScissor(viewX,0,viewW,h);
        glClearColor(.035f,.045f,.07f,1);
        glClear(GL_COLOR_BUFFER_BIT);
        (full?fullBackdrop:backdrop).draw(viewW,h);
        glUseProgram(prog);
        for (int pass = 0; pass < 2; pass++) {
          for (auto &p : scene.parts) {
            auto &node = scene.d->nodes[p.node];
            auto *mat = p.prim->material;
            bool blend = mat && mat->alpha_mode == cgltf_alpha_mode_blend;
            if (int(blend) != pass)
              continue;
            if (blend) {
              glEnable(GL_BLEND);
              glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA);
              glDepthMask(GL_FALSE);
            } else {
              glDisable(GL_BLEND);
              glDepthMask(GL_TRUE);
            }
            glDisable(GL_CULL_FACE);
            glUniformMatrix4fv(glGetUniformLocation(prog, "model"), 1, GL_FALSE,
                               scene.world[p.node].data());
            glUniform1i(glGetUniformLocation(prog, "skinned"),
                        node.skin ? 1 : 0);
            if (node.skin) {
              std::vector<M> palette(node.skin->joints_count);
              M inv = inverse(scene.world[p.node]);
              for (size_t k = 0; k < palette.size(); k++) {
                M bind = identity();
                if (node.skin->inverse_bind_matrices)
                  need(
                      cgltf_accessor_read_float(
                          node.skin->inverse_bind_matrices, k, bind.data(), 16),
                      "inverse bind read failed");
                palette[k] =
                    mul(mul(inv,
                            scene.world[node.skin->joints[k] - scene.d->nodes]),
                        bind);
              }
              glUniformMatrix4fv(glGetUniformLocation(prog, "bones"),
                                 int(palette.size()), GL_FALSE,
                                 palette[0].data());
            }
            float color[4] = {1, 1, 1, 1};
            cgltf_texture *tex = nullptr;
            if (mat) {
              if (mat->has_pbr_metallic_roughness) {
                memcpy(color, mat->pbr_metallic_roughness.base_color_factor,
                       16);
                tex = mat->pbr_metallic_roughness.base_color_texture.texture;
              }
              glUniform1f(glGetUniformLocation(prog, "cutoff"),
                          mat->alpha_mode == cgltf_alpha_mode_mask
                              ? mat->alpha_cutoff
                              : 0);
              glUniform1i(glGetUniformLocation(prog, "unlit"), mat->unlit);
            } else {
              glUniform1f(glGetUniformLocation(prog, "cutoff"), 0);
              glUniform1i(glGetUniformLocation(prog, "unlit"), 0);
            }
            glUniform4fv(glGetUniformLocation(prog, "color"), 1, color);
            glUniform1i(glGetUniformLocation(prog, "textured"), tex ? 1 : 0);
            if (tex) {
              glActiveTexture(GL_TEXTURE0);
              glBindTexture(GL_TEXTURE_2D,
                            scene.textures[tex - scene.d->textures]);
            }
            glBindVertexArray(p.vao);
            glDrawElements(GL_TRIANGLES, int(p.indices.size()), GL_UNSIGNED_INT,
                           nullptr);
          }
        }
        glDepthMask(GL_TRUE);
        glDisable(GL_SCISSOR_TEST);
        if(appMode && controls.poseEdit)poseOverlay.draw(scene,view,controls.selected,w,h);
        if(full)cutouts.draw(w,h);
        // Expose only the actual application volume card, on either edge and
        // in either Talk layout. The app retains slider input and its timer.
        if(talkLayout && controls.overlayW>0 && controls.overlayH>0) {
          int x=std::min(w,controls.overlayX), y=std::min(h,controls.overlayY);
          int rw=std::min(controls.overlayW,w-x), rh=std::min(controls.overlayH,h-y);
          glEnable(GL_SCISSOR_TEST);glScissor(x,h-y-rh,rw,rh);
          glClearColor(0,0,0,0);glClear(GL_COLOR_BUFFER_BIT);
          glDisable(GL_SCISSOR_TEST);
        }
        // The native layer must not cover the application's full-width keyboard.
        // Keep all pose/model state alive; transparent pixels expose native UI.
        if(appMode && !talkLayout && controls.hidden){
          glDisable(GL_SCISSOR_TEST);glClearColor(0,0,0,0);
          glClear(GL_COLOR_BUFFER_BIT);
        }
        glFinish();
        need(glGetError() == GL_NO_ERROR, "GLES rendering error");
        times.push_back((now() - frame) * 1000);
        if(appMode && !talkLayout && t>=nextJoints){
          for(int id=0;id<poseCount;id++){
            int node=poseNode(scene,id);if(node<0)continue;
            auto m=mul(view,scene.world[node]);float x=(m[12]*.5f+.5f)*w,y=(.5f-m[13]*.5f)*h;
            if(std::isfinite(x)&&std::isfinite(y)&&fabsf(x)<1e6&&fabsf(y)<1e6)
              printf("AVATAR_JOINT %d %.2f %.2f %d\n",id,x,y,int(x>=0&&x<w&&y>=0&&y<h&&m[14]>=-1&&m[14]<=1));
          }
          fflush(stdout);nextJoints=t+.1;
        }
        if (appMode && t >= nextStats) {
          printf("AVATAR_STATS %.3f %zu %.3f %.4f %.3f\n",
                 t > .01 ? frames / t : 0., frames, t, mouth, times.back());
          fflush(stdout);
          nextStats = t + 1.;
        }
        if (frames == 0) {
          printf("FIRST_FRAME_MS=%.3f\n", (now() - begin) * 1000);
          if (appMode) {
            puts("AVATAR_READY=4");
            fflush(stdout);
          }
        }
        frames++;
        if (out && !captured && t >= std::min(1., seconds * .5)) {
          screenshot(out, w, h);
          captured = true;
        }
        need(eglSwapBuffers(egl.display, egl.surface), "swap failed");
        deadline += 1. / 30;
        double wait = deadline - now();
        if (wait > 0)
          std::this_thread::sleep_for(std::chrono::duration<double>(wait));
        else if (wait < -.2)
          deadline = now();
      }
      double elapsed = now() - start;
      std::sort(times.begin(), times.end());
      auto pct = [&](double p) {
        return times.empty() ? 0.
                             : times[std::min(times.size() - 1,
                                              size_t((times.size() - 1) * p))];
      };
      rusage usage{};
      getrusage(RUSAGE_SELF, &usage);
      double cpu = (usage.ru_utime.tv_sec - baseline.ru_utime.tv_sec) +
                   (usage.ru_stime.tv_sec - baseline.ru_stime.tv_sec) +
                   (usage.ru_utime.tv_usec - baseline.ru_utime.tv_usec +
                    usage.ru_stime.tv_usec - baseline.ru_stime.tv_usec) /
                       1e6;
      std::sort(intervals.begin(), intervals.end());
      auto interval = [&](double p) {
        return intervals.empty()
                   ? 0.
                   : intervals[size_t((intervals.size() - 1) * p)];
      };
      printf("INTERVAL_MS_P95=%.3f\nINTERVAL_MS_P99=%.3f\nINTERVAL_MS_MAX=%."
             "3f\nCPU_CORES_AVERAGE=%.3f\n",
             interval(.95), interval(.99),
             intervals.empty() ? 0. : intervals.back(), cpu / elapsed);
      printf("FRAMES=%zu\nELAPSED_S=%.3f\nFPS=%.3f\nWORK_MS_P50=%.3f\nWORK_MS_"
             "P95=%.3f\nWORK_MS_P99=%.3f\nPEAK_RSS_KIB=%ld\n",
             frames, elapsed, frames / elapsed, pct(.5), pct(.95), pct(.99),
             usage.ru_maxrss);
      glDeleteProgram(prog);
    }
    avatar_window_close();
    result = 0;
  } catch (const std::exception &e) {
    printf("AVATAR_ERROR=%s\n", stage);
    fflush(stdout);
    fprintf(stderr, "echolocal-avatar: %s\n", e.what());
    avatar_window_close();
  }
  return result;
}
