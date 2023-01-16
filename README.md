# mpdGrabber

Experimental, unsupported library + tool to backup mpeg-dash streams.

Think of a dash/mpd only, light version of [youtube-dl](https://youtube-dl.org/) or [ffmpeg stream downloader](https://ffmpeg.org/) but 10x faster.

* Audio, video and subtitles (webvtt and ttml) streams are supported (fragmented or not).
* Subtitle streams are also converted to files in case your player doesn't play the embedded version.
* Live streams not supported

Why is it so fast you might ask? Because the streams are downloaded concurrently and reasseembled at the end. 
When other tools usually download one 1 segment at a time.

## What about m3u8/hsls streams?

Take a look at [m3u8Grabber](https://github.com/mattetti/m3u8Grabber)

## How can I backup <insert website>

That's not the purpose of this library, write your own wrapper that gets the `.mpd` and feed it to this library.

## Why do I need to have ffmpeg installed?

Because the final stream is currently being assembled using ffmpeg but I might end up doing the muxing in Go myself to drop the dependency later on.
