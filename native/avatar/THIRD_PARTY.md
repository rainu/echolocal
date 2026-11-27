# Licenses and provenance

This **experimental viewer** is distributed under AGPL-3.0-only (see `LICENSE`),
separately from EchoLocal's root MIT license. In particular, the Fire OS surface
adapter is derived from the AGPL-licensed libcountertop implementation. Do not
relabel this directory or copy its adapter into an MIT-only distribution.

| Dependency/reference | Revision | License / notice |
| --- | --- | --- |
| cgltf | v1.15 | MIT; copyright Johannes Kuhlmann, notice retained in `vendor/cgltf.h` |
| stb_image | f0569113c93ad095470c54bf34a17b36646bbbb5 | MIT option; copyright Sean Barrett, notice retained in `vendor/stb_image.h` |
| nlohmann/json | v3.11.3 | MIT; copyright Niels Lohmann, `vendor/json-LICENSE.MIT` |
| libcountertop surface adapter | rainu/libcountertop e3fd8ed21859270023ff62f0c50a2398b1539038, `native/surface/surface.c` | AGPL-3.0; adapter derives the API-28 Fire OS transaction construction, layer creation and native-window discovery approach; full license in `LICENSE` |
| three-vrm constraint algorithms | pixiv/three-vrm v3.5.5, `packages/three-vrm-node-constraint/src/VRM{Rotation,Roll,Aim}Constraint.ts` | MIT; notice below; adapted to native quaternion math |

Vendored source integrity is recorded in `vendor/SHA256SUMS`. There is no
JavaScript/browser dependency in this native executable. No OpenClaw-Pet code,
model, bundle, pose or motion asset is copied.

## Adapted three-vrm algorithm notice

MIT License

Copyright (c) 2019-2026 pixiv Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Test assets (not distributed in Git)

`download-samples.py` pins public files and validates hashes; a renderer license
does not grant any rights to user avatars. Embedded metadata was inspected
before on-device use. Samples are kept only in the private test artifact store.

- **VRM1_Constraint_Twist_Sample v1.0.1**: (c) 2022 pixiv Inc.;
  [VRM Public License 1.0](https://vrm.dev/licenses/1.0/). Metadata permits
  everyone to perform, corporate commercial usage, redistribution and
  modification redistribution. Attribution is not required, but is provided.
- **Seed-san v1**: model by **VirtualCast, Inc.**;
  [VRM Public License 1.0](https://vrm.dev/licenses/1.0/). Metadata permits
  everyone to perform, corporate commercial usage, redistribution and
  modification redistribution; **credit is required**. Preserve this credit
  in any derived screenshots, videos or demonstrations.
- **Alicia Solid v1.10**: **(c) DWANGO Co., Ltd.**; custom
  [Niconi Solid-chan terms](https://3d.nicovideo.jp/alicia/rule.html), not MIT.
  Embedded metadata permits everyone and commercial use but disallows violent
  and sexual usage. The linked Japanese terms were read: they permit individual
  derivative creation/use, require non-infringing/non-misleading usage and
  prohibit political/religious/antisocial uses. Used only for a neutral private
  rendering test; not included in a redistributable asset bundle.

The generated VRMA clips and PCM tone are newly authored test fixtures. They
contain no third-party motion, voice, transcript or captured room audio.

## Scene image decoder (daemon, not the native helper)

github.com/gen2brain/webp v0.6.4 supplies libwebp compiled/transpiled to Go.
No system library, codec subprocess, audio stream or server-side transcoding
is required. The following notices cover the bundled daemon decoder:

MIT License

Copyright (c) 2024 gen2brain

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

Copyright (c) 2010, Google Inc. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

  * Redistributions of source code must retain the above copyright
    notice, this list of conditions and the following disclaimer.

  * Redistributions in binary form must reproduce the above copyright
    notice, this list of conditions and the following disclaimer in
    the documentation and/or other materials provided with the
    distribution.

  * Neither the name of Google nor the names of its contributors may
    be used to endorse or promote products derived from this software
    without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
