# Processed microphone test

Open **Settings → Microphone / test → Record and listen (processed)** on the
device, or use **Microphone test** next to microphone processing settings in the
authenticated web UI.

1. Pause any other speaker playback or voice conversation.
2. Record for up to eight seconds, or press Stop earlier.
3. Play the take on the Echo. Playback is never concurrent with test capture.
4. Change gain, mixing, leveling or echo-cancellation settings between takes and
   record again. Processing edits are rejected while a test is active.
5. Delete the take, or leave the test page/section.

The signal is the same 16 kHz mono processed mic.Source.Listen stream consumed
by recognition, including the selected microphone mixing, hardware gain,
echo-cancellation/preprocessing, low-cut and leveling. The displayed processing
summary is captured when a take starts. Raw hardware channels and the unleveled
diagnostic stream are deliberately not used. Actual playback also depends on
the configured speaker output/volume, so keep that constant for comparisons.

## Privacy and lifecycle

One take at a time is held in bounded RAM (at most 128,000 signed 16-bit samples).
There is no audio file, upload, download endpoint, assistant conversation,
history entry or persistent setting for test audio. Web commands control local
Echo capture and Echo playback; no audio is sent to the browser or a provider.

Each device page/browser tab owns its take. Another page cannot accidentally
stop, play or replace it. Leaving the page discards it. A ten-second heartbeat
lease bounds cleanup after a crashed/disconnected browser. Even with a live
page, the take expires after two minutes from capture start. Replacing or
discarding a take clears the retained sample buffer; service shutdown clears it.

Wake-word detection and new assistant starts are gated during capture/playback.
A test refuses busy voice/output operation and stops if another voice operation
takes priority, capture stalls, or mute becomes active during capture. A recording
is never automatically played. Stop cancels only test capture/playback; Delete
also erases the take. Playback keeps the gate closed through output drainage and
the hardware tail. Errors remain visible and a new take can be attempted.

Live monitoring was not selected: sequential capture/playback avoids an
unbounded acoustic feedback loop and needs no browser audio transport.
Physical microphone/speaker quality is an audible owner acceptance check;
synthetic tests do not prove room acoustics or physical touch behavior.
