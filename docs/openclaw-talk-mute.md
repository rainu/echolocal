# Microphone mute during Talk (Refs #75)

The hardware mute line still cuts the microphone using the existing board driver.
During an already active Talk, muting no longer cancels the conversation,
releases the Talk view or changes the selected session. The input encoder is
muted independently of incoming agent playback. Captured microphone frames
are discarded; synthetic silence keeps the media clock running even if the
hardware capture source stops publishing. The microphone-live indicator is
cleared immediately and no mute-period microphone backlog is replayed on release.

Local inactivity expiry is suspended while muted. Provider responses still
update the underlying activity state; unmuting restores it and starts a fresh
local inactivity interval when listening. No session is created or reconnected
by unmuting. Provider/network failures and provider-imposed session limits still
end Talk normally; mute does not promise unlimited transport lifetime.

Split-screen and caption views display a localized microphone-muted state
without claiming that the conversation ended. Fullscreen keeps its existing
two controls and native cutout geometry: a red, crossed-microphone badge beside
the split-view icon indicates muted input. That button still switches layout,
not the hardware microphone; the separate hang-up button still ends Talk.

Starting a new Talk while already hardware-muted remains rejected. Muting
outside Talk and the board privacy driver, microphone persistence, other audio
consumers and speaker output settings are unchanged. An unreadable hardware
mute state is treated as muted input.

## Verification boundaries

Unit and simulated transport tests exercise lifecycle ownership, mute/unmute
while connecting, idle-timeout suspension, output preservation, PCM suppression,
stale-frame disposal and translated UI/actions. Running those tests on a device
does not prove that its physical mute button toggles the latch or that real
speech/output was heard. The existing diagnostic button command injects a
software button event; on boards whose kernel normally toggles the mute latch
before the event, it is not a physical-button test. Exact candidate build and
actual device observations belong in the linked PR.
