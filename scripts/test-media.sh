#!/usr/bin/env bash
# Makes the small test videos the integration tests play, with FFmpeg:
#   h264_1080p.mp4       1080p H.264 with AAC audio
#   chapters.mkv         three chapters and two subtitle tracks
#   sbs_half_3d.mp4      half side-by-side 3D
#   vr360_equirect.mp4   360° equirectangular
# Usage: scripts/test-media.sh <folder>
set -euo pipefail
out="${1:?usage: $0 <folder>}"
mkdir -p "$out"
cd "$out"
ff() { ffmpeg -hide_banner -loglevel error -y "$@"; }

ff -f lavfi -i testsrc2=size=1920x1080:rate=30 -f lavfi -i sine=frequency=440:sample_rate=48000 \
	-t 20 -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac h264_1080p.mp4

cat >chapters.txt <<'META'
;FFMETADATA1
title=Chapter test
[CHAPTER]
TIMEBASE=1/1000
START=0
END=15000
title=Opening
[CHAPTER]
TIMEBASE=1/1000
START=15000
END=40000
title=Middle part
[CHAPTER]
TIMEBASE=1/1000
START=40000
END=60000
title=Ending
META
printf '1\n00:00:01,000 --> 00:00:05,000\nHello subtitle\n' >en.srt
printf '1\n00:00:01,000 --> 00:00:05,000\nBonjour\n' >fr.srt
ff -f lavfi -i testsrc2=size=640x360:rate=30 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 60 \
	-c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac av.mkv
ff -i av.mkv -i chapters.txt -i en.srt -i fr.srt -map 0 -map 2 -map 3 -map_metadata 1 -c copy -c:s srt \
	-metadata:s:s:0 language=eng -metadata:s:s:1 language=fre chapters.mkv
rm -f av.mkv chapters.txt en.srt fr.srt

ff -f lavfi -i testsrc2=size=960x1080:rate=30 -f lavfi -i testsrc=size=960x1080:rate=30 \
	-filter_complex "[0:v][1:v]hstack" -t 10 -c:v libx264 -preset ultrafast -pix_fmt yuv420p sbs_half_3d.mp4

ff -f lavfi -i testsrc2=size=1920x960:rate=30 -t 10 -c:v libx264 -preset ultrafast -pix_fmt yuv420p vr360_equirect.mp4
ls -l
