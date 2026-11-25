#!/usr/bin/env python3
"""Generate redistributable synthetic A/V fixtures; never inspect user media."""
import argparse
import hashlib
import json
import pathlib
import subprocess
import urllib.request

MATRIX = [
 ("asf-wmv1-wma2.wmv", "wmv1", "wmav2", []),
 ("asf-wmv2-wma2.wmv", "wmv2", "wmav2", []),
 ("asf-mpeg4-wma2.asf", "mpeg4", "wmav2", []),
 ("avi-mpeg4-sp-mp3.avi", "mpeg4", "libmp3lame", []),
 ("avi-mpeg4-asp-mp2.avi", "mpeg4", "mp2", ["-flags", "+mv4+qpel", "-bf", "2"]),
 ("avi-msmpeg4v2-pcm8.avi", "msmpeg4v2", "pcm_u8", []),
 ("avi-msmpeg4v3-pcm16.avi", "msmpeg4", "pcm_s16le", []),
 ("avi-h264-mp3.avi", "libx264", "libmp3lame", ["-preset", "ultrafast", "-bf", "0"]),
 ("ps-mpeg1-mp2.mpg", "mpeg1video", "mp2", []),
 ("ps-mpeg2-ac3.mpg", "mpeg2video", "ac3", []),
 ("flv-h264-aac.flv", "libx264", "aac", ["-preset", "ultrafast", "-bf", "0"]),
 ("mov-mjpeg-ac3.mov", "mjpeg", "ac3", ["-pix_fmt", "yuvj420p"]),
 ("mp4-mpeg4-aac.mp4", "mpeg4", "aac", ["-movflags", "+faststart"]),
 ("mp4-h264-aac-lc.mp4", "libx264", "aac", ["-preset", "ultrafast", "-movflags", "+faststart"]),
]

def main():
 p=argparse.ArgumentParser();p.add_argument("out",type=pathlib.Path);p.add_argument("--seconds",type=int,default=8)
 p.add_argument("--public",action="store_true",help="download hash-pinned public reference clips and remux short test excerpts")
 p.add_argument("--motion",action="store_true",help="use moving testsrc2 instead of the compact black/white pulse chart")
 a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True)
 manifest=[]
 for name,video,audio,extra in MATRIX:
  target=a.out/name
  cmd=["ffmpeg","-hide_banner","-loglevel","error","-y","-f","lavfi","-i",
       ("testsrc2=size=480x270:rate=24" if a.motion else "color=black:size=480x270:rate=24")+",drawbox=x=0:y=0:w=iw:h=ih:color=white:t=fill:enable='lt(mod(t,1),0.08)'",
       "-f","lavfi","-i",r"aevalsrc=if(lt(mod(t\,1)\,0.08)\,0.35*sin(2*PI*880*t)\,0):s=48000",
       "-t",str(a.seconds),"-c:v",video,"-b:v","700k","-c:a",audio,"-ac","2","-b:a","128k",*extra,str(target)]
  subprocess.run(cmd,check=True)
  probe=json.loads(subprocess.check_output(["ffprobe","-v","error","-show_streams","-show_format","-of","json",str(target)]))
  manifest.append({"file":name,"sha256":hashlib.sha256(target.read_bytes()).hexdigest(),
     "bytes":target.stat().st_size,"generator":cmd[1:-1],"streams":[{k:s.get(k) for k in
     ("codec_type","codec_name","profile","width","height","pix_fmt","r_frame_rate","sample_rate","channels","bit_rate")} for s in probe["streams"]]})
 if a.public:
  public(a.out,a.seconds,manifest)
 (a.out/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n")
 limits=a.out/"boundaries";limits.mkdir(exist_ok=True)
 subprocess.run(["ffmpeg","-hide_banner","-loglevel","error","-y","-i",str(a.out/"mp4-h264-aac-lc.mp4"),"-c","copy","-timecode","00:00:00:00",str(limits/"data.mp4")],check=True)
 for name,size,rate in [("oversize.avi","1600x900","24"),("overrate.avi","320x180","60")]:
  subprocess.run(["ffmpeg","-hide_banner","-loglevel","error","-y","-f","lavfi","-i",f"color=black:size={size}:rate={rate}","-f","lavfi","-i","anullsrc=r=48000:cl=stereo","-t","1","-c:v","mpeg4","-c:a","pcm_s16le",str(limits/name)],check=True)

def public(out,seconds,manifest):
 # These are public reference downloads, not an assertion of redistribution
 # rights. Do not commit/rehost their binaries; preserve source attribution.
 sources={
  "svq3.mov":("https://fate-suite.ffmpeg.org/svq3/Vertical400kbit.sorenson3.mov","2231f6e48999d9734aa74d9d70515a16a248013008e4921a66367e8a2467de78"),
  "vp6.flv":("https://fate-suite.ffmpeg.org/flash-vp6/clip1024.flv","f61d4a1696000cbb6d1e6a8bd7e4682656da3ad017c49fd6d7c47a7f28d8aefe"),
  "wmv3.wmv":("https://samples.ffmpeg.org/V-codecs/WMV9/nokia_n90.wmv","a4145c073333b2b54d0f8dafaa75366e2e72b9d84f38d5a01294ed3fcf5b7e98"),
  "qdm2.mov":("https://fate-suite.ffmpeg.org/qt-surge-suite/surge-2-16-B-QDM2.mov","05ab23edd7337b4202a4ad08b52c8370120a5040585c9995736de72adce3df3d"),
  "heaac.mp4":("https://fate-suite.ffmpeg.org/aac/al_sbr_cm_48_2.mp4","7e7c0390c950eda21775d1a193e05ee703257bed85602884e7d30a8b3c65be90"),
 }
 root=out/"public-sources";root.mkdir(exist_ok=True)
 for name,(url,digest) in sources.items():
  target=root/name
  if not target.exists():urllib.request.urlretrieve(url,target)
  if hashlib.sha256(target.read_bytes()).hexdigest()!=digest:raise RuntimeError("Reference fixture hash mismatch: "+name)
 (root/"sources.json").write_text(json.dumps(sources,indent=2)+"\n")
 jobs=[
  ("mov-svq3-qdm2.mov",["-i",str(root/"svq3.mov"),"-i",str(root/"qdm2.mov"),"-map","0:v:0","-map","1:a:0","-c","copy"]),
  ("flv-vp6-mp3.flv",["-i",str(root/"vp6.flv"),"-i",str(out/"avi-mpeg4-sp-mp3.avi"),"-map","0:v:0","-map","1:a:0","-c","copy"]),
  ("asf-wmv3-wma2.wmv",["-i",str(root/"wmv3.wmv"),"-map","0:v:0","-map","0:a:0","-c","copy"]),
  ("avi-wmv3-pcm16.avi",["-i",str(root/"wmv3.wmv"),"-map","0:v:0","-map","0:a:0","-r","25","-c:v","copy","-c:a","pcm_s16le"]),
  ("mp4-h264-heaac.mp4",["-i",str(out/"mp4-h264-aac-lc.mp4"),"-i",str(root/"heaac.mp4"),"-map","0:v:0","-map","1:a:0","-c","copy","-movflags","+faststart"]),
 ]
 for name,args in jobs:
  target=out/name
  subprocess.run(["ffmpeg","-hide_banner","-loglevel","error","-y",*args,"-t",str(seconds),str(target)],check=True)
  probe=json.loads(subprocess.check_output(["ffprobe","-v","error","-show_streams","-of","json",str(target)]))
  manifest.append({"file":name,"sha256":hashlib.sha256(target.read_bytes()).hexdigest(),"bytes":target.stat().st_size,"public_sources":sources,"remux":args,"streams":[{k:s.get(k) for k in ("codec_type","codec_name","profile","width","height","sample_rate","channels","avg_frame_rate","bit_rate")} for s in probe["streams"]]})

if __name__=="__main__":main()
