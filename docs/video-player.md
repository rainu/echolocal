# Standalone video playback

The **Video** page in the device's authenticated web interface plays a direct HTTP(S) media URL on the Echo display and speakers. It does not use Google Cast discovery, device authentication, or a sender app.

## Use

1. Open the device web interface and sign in with its existing admin password.
2. Select **Video**.
3. Paste a direct media URL and choose **Play on device**.
4. Use **Pause / Resume**, **Stop**, or **Jump to (seconds)**. Seeking is available only when the source supports it. The Echo's existing fullscreen video controls remain available.

The browser is a remote control: the Echo fetches the media itself. A local media server must therefore be reachable from the Echo. URLs are not saved as settings. Stop playback before leaving the device unattended; closing the browser does not stop the video.

## Scope and limitations

- Start with MP4 containing H.264 video and AAC audio.
- The underlying player also has HLS support; codec, server and stream compatibility must be tested for the particular source.
- This is not a browser-tab or screen-mirroring receiver.
- YouTube watch links and arbitrary web pages are not direct video URLs and are not supported by this page.
- DRM-protected streaming services and URL-embedded login credentials are not supported by this entry point.
- The existing Cast functionality is unchanged. This feature does not fix or bypass Google's Cast device trust checks.
- The Video page is offered only by display-capable builds.

The playback API is mounted under the existing authenticated admin API at `/api/v1/video/`: GET returns status; POST accepts `load` (with `url`), `play`, `pause`, `stop`, or `seek` (with a nonnegative `position` in seconds). Status contains no source URL or raw upstream error. Do not expose the admin API publicly.

Refs #68.
