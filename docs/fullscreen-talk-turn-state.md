# Fullscreen Talk turn cue

The existing split-view button keeps its 72 × 72 touch target and action.
The hang-up button is unchanged. No third control is introduced.

- Teal + microphone: listening, only while the Talk lifecycle is listening
  and microphone frames have recently been delivered.
- Blue + speaker: assistant PCM is queued in the device voice lane.
  Incoming packets and the avatar's mouth animation do not establish playback.
- Neutral + dash: connecting, waiting, or disconnected. The accessible name
  names the actual phase and the split-view action.
- Muted input: a crossed microphone when no output is playing; a speaker plus a small crossed
  microphone during playback, with both playback and microphone-muted states in
  the accessible name. Muting does not stop the assistant's output.

All symbols are static and fit inside the existing button aperture. White
foreground contrast exceeds 4.5:1 against both active fills and the neutral
fill. The separate agent-working dots retain their neutral treatment, location,
and optional static alternative. Waiting never fabricates agent-working state.

## State boundaries

The cue is sampled on the existing 100 ms Talk UI tick. A changed cue requests
a redraw independently of caption, lifecycle, and agent-working changes.
The current activation, cancellation, audio-ready gate, mute flags, and voice
lane are checked. Reconnection and terminal states override stale I/O.
Audio prefill and the lifecycle's post-playback grace interval are neutral,
not speaking. Microphone freshness uses the existing 500 ms capture-health
window; it does not claim that the user is currently uttering words.

This is device queue/capture feedback, not a sample-accurate acoustic detector.
Very short playback between UI samples need not appear. A broken downstream
speaker cannot be detected from queued PCM alone.

## Verification scope

Regression tests cover lifecycle and I/O combinations, interruption/drain,
mute with and without output, reconnect, transfer, stop and failure. GUI tests
exercise all four UI languages and supported test sizes, both action targets,
symbol containment, coexistence with working dots, and palette contrast.
These are synthetic checks, not claims of physical display or voice testing.
Live hardware results for an exact candidate are recorded in its pull request.
