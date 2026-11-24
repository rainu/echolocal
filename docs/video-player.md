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

## Home Assistant (no HACS or custom integration)

Display-capable devices expose an additional **Video** media-player entity through the existing ESPHome native integration. Home Assistant discovers it when the device reconnects after this update.

Use the standard service/action with the new Video entity:

```yaml
action: media_player.play_media
target:
  entity_id: media_player.your_device_video
data:
  media_content_type: video
  media_content_id: https://example.com/video.mp4
```

Replace the example entity ID with the Video entity discovered for your device. `media_player.media_pause`, `media_player.media_play` and `media_player.media_stop` control the same playback as the web interface and native fullscreen controls. Its volume and mute controls address the existing media audio channel.

The original **Speaker** entity is unchanged, including Home Assistant's audio conversion and announcement handling. The separate Video entity deliberately advertises no audio-conversion format: Home Assistant otherwise turns MP4 into WAV and drops the picture. No REST login, extra service, HA configuration patch, HACS installation or `bypass_proxy` flag is required.

Pass direct HTTP(S) media URLs. Home Assistant's ESPHome native media protocol does not carry a seek command; seeking remains available on the device and web interface rather than being advertised as an unsupported HA feature. The Video entity does not handle announcements; send those to Speaker.


### Existing Speaker proxy prerequisite

The original Speaker entity still uses Home Assistant's WAV/announcement proxy. That proxy needs a Home Assistant internal URL which is reachable from the device; an automatically detected container-only address may not be reachable. If direct Video playback and a correctly formatted direct WAV work but normal Speaker conversion does not, check this HA URL setting separately. This update does not rewrite HA's global URL configuration.
