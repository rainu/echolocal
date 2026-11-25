#!/usr/bin/env python3
"""Additional synthetic and hash-pinned public video regression inputs."""
import argparse, hashlib, json, pathlib, subprocess, urllib.request

PUBLIC = {
 "vc1.vc1": ("https://fate-suite.ffmpeg.org/vc1/SA00040.vc1","898035e63cfe0ddb1cc2c8749d7dafdfcc1b12fe5dac7e5b65941e6c4e41c860"),
 "indeo2.avi": ("https://fate-suite.ffmpeg.org/rt21/ISKATE.AVI","fc87eb8eb39c5296e01dcf97fe6a14b77af100b45be6ddff58e7a87b9ca46e9a"),
 "indeo3.mov": ("https://fate-suite.ffmpeg.org/iv32/cubes.mov","1c17bd44f55f3e6f0ee4f82c7c8bdea91160e7416363efc3a1bd7924a7728518"),
 "indeo4.avi": ("https://fate-suite.ffmpeg.org/iv41/indeo41-partial.avi","4d5b6afb804b76c72bcc9067145cab4559d2961fd24ee220756ba413963bc75a"),
 "indeo5.avi": ("https://fate-suite.ffmpeg.org/iv50/Educ_Movie_DeadlyForce.avi","70564f095c92527c5e55f503207d579c5a3e177f4592a9e146e19e98d081d464"),
}
# file, video encoder, audio encoder, dimensions, encoder/muxer options
MATRIX = [
 ("flv-flv1-mp3.flv","flv","libmp3lame","480x272",[]),
 ("avi-h263-adpcm-ms.avi","h263","adpcm_ms","352x288",[]),
 ("3gp-h263-aac.3gp","h263","aac","352x288",[]),
 ("avi-cinepak-pcm8.avi","cinepak","pcm_u8","320x240",["-pix_fmt","rgb24"]),
 ("mov-rpza-pcm16be.mov","rpza","pcm_s16be","320x240",["-pix_fmt","rgb555le"]),
 ("mov-svq1-ima4.mov","svq1","adpcm_ima_qt","320x240",["-pix_fmt","yuv410p"]),
 ("avi-msvideo1-ima-wav.avi","msvideo1","adpcm_ima_wav","320x240",["-pix_fmt","rgb555le"]),
 ("avi-msrle-alaw.avi","msrle","pcm_alaw","320x240",["-pix_fmt","pal8"]),
 ("avi-dv-pcm16.avi","dvvideo","pcm_s16le","720x576",["-pix_fmt","yuv420p"]),
 ("mkv-ffv1-flac.mkv","ffv1","flac","480x272",["-pix_fmt","yuv420p"]),
 ("avi-huffyuv-pcm24.avi","huffyuv","pcm_s24le","480x272",["-pix_fmt","yuv422p"]),
 ("avi-ffvhuff-pcm32.avi","ffvhuff","pcm_s32le","480x272",["-pix_fmt","yuv422p"]),
 ("ogg-theora-vorbis.ogv","libtheora","libvorbis","480x272",[]),
 ("webm-vp8-opus.webm","libvpx","libopus","480x272",["-deadline","realtime","-cpu-used","8"]),
 ("webm-vp9-opus.webm","libvpx-vp9","libopus","480x272",["-deadline","realtime","-cpu-used","8"]),
 ("mkv-hevc-dts.mkv","libx265","dca","480x272",["-pix_fmt","yuv420p","-bf","0","-x265-params","pools=none:frame-threads=1:log-level=error","-strict","-2","-b:a","768k"]),
 ("mp4-h264-eac3.mp4","libx264","eac3","480x272",["-preset","ultrafast","-movflags","+faststart"]),
 ("mov-mpeg4-alac.mov","mpeg4","alac","480x272",[]),
 ("mkv-mpeg4-pcm-f32.mkv","mpeg4","pcm_f32le","480x272",[]),
 ("mkv-mpeg4-pcm-f64.mkv","mpeg4","pcm_f64le","480x272",[]),
 ("asf-wmv2-wmav1.wmv","wmv2","wmav1","480x272",[]),
 ("ts-mpeg2-eac3.ts","mpeg2video","eac3","480x272",[]),
 ("ts-h264-mp2.ts","libx264","mp2","480x272",["-preset","ultrafast"]),
 ("ts-hevc-aac.ts","libx265","aac","480x272",["-pix_fmt","yuv420p","-bf","0","-x265-params","pools=none:frame-threads=1:log-level=error"]),
]
def run(cmd):
 subprocess.run(["ffmpeg","-hide_banner","-loglevel","error","-y",*cmd],check=True)
def info(target,cmd,**extra):
 data=json.loads(subprocess.check_output(["ffprobe","-v","error","-show_streams","-of","json",str(target)]))
 return dict(file=target.name,sha256=hashlib.sha256(target.read_bytes()).hexdigest(),bytes=target.stat().st_size,
  streams=[{k:s.get(k)for k in ("codec_type","codec_name","profile","width","height","pix_fmt","avg_frame_rate","sample_rate","channels")}for s in data["streams"]],
  generator=cmd,**extra)
def main():
 p=argparse.ArgumentParser();p.add_argument("out",type=pathlib.Path);p.add_argument("--seconds",type=int,default=4);p.add_argument("--public",action="store_true");a=p.parse_args()
 a.out.mkdir(parents=True,exist_ok=True);manifest=[];failures=[]
 for name,video,audio,size,opts in MATRIX:
  target=a.out/name
  cmd=["-f","lavfi","-i",f"color=black:size={size}:rate=25,drawbox=x=0:y=0:w=iw:h=ih:color=white:t=fill:enable='lt(mod(t,1),0.08)'",
   "-f","lavfi","-i",r"aevalsrc=if(lt(mod(t\,1)\,0.08)\,0.35*sin(2*PI*880*t)\,0):s=48000",
   "-t",str(a.seconds),"-c:v",video,"-b:v","700k","-c:a",audio,"-ac","2","-b:a","128k",*opts,str(target)]
  try:run(cmd);manifest.append(info(target,cmd[:-1]));print(name,"GENERATED",flush=True)
  except subprocess.CalledProcessError:failures.append(name);print(name,"GENERATOR_FAILED",flush=True)
 if a.public:
  source=a.out/"public-sources";source.mkdir(exist_ok=True)
  for name,(url,digest)in PUBLIC.items():
   target=source/name
   if not target.exists():urllib.request.urlretrieve(url,target)
   if hashlib.sha256(target.read_bytes()).hexdigest()!=digest:raise RuntimeError("public source hash mismatch")
   dest=a.out/("public-"+name.replace(".vc1",".avi"))
   args=(["-fflags","+genpts","-r","25","-stream_loop","-1"]if name.endswith(".vc1")else [])
   cmd=[*args,"-i",str(target),"-f","lavfi","-i","sine=frequency=880:sample_rate=48000","-t",str(a.seconds),
    "-map","0:v:0","-map","1:a:0","-c:v","copy","-c:a","pcm_s16le","-ac","2",str(dest)]
   try:run(cmd);manifest.append(info(dest,[],public_source=dict(url=url,sha256=digest),remux=cmd[:-1]));print(dest.name,"REMUXED",flush=True)
   except subprocess.CalledProcessError:failures.append(dest.name)
 (a.out/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n")
 (a.out/"generation-failures.json").write_text(json.dumps(failures)+"\n")
 # Public sources are downloads for local verification, not a redistribution grant.
 if failures:raise SystemExit(1)
if __name__=="__main__":main()
