# Verified agent activity in avatar Talk

Refs #99.

## State source

The Talk activation observes `sessions.describe {key: <current target>}` using
its existing operator read grant and exact authenticated socket. The adapter
retains only session incarnation and the boolean result of
`hasActiveRun || hasActiveSubagentRun || backgroundProcessCount > 0`.
It never subscribes to transcripts, retrieves reasoning, enumerates other sessions,
or sends a message to generate activity. No new permission is requested.

A serialized read starts after successful Talk readiness, then one second after
each completed read. Each read has a two-second deadline. A sample older than
four seconds is unknown, not busy. Failed/denied/unsupported reads, missing
activity metadata, disconnect, cancellation and terminal failure clear the badge.
A changed session incarnation ends that observer. Recovery and session transfer
start fresh observers; old observations cannot migrate to a new activation.
The private developer `talk status` reports only Available/Working booleans.

This is independent of listening, connecting, input mute and actual voice
playback. Fresh verified work suspends local idle expiry without changing those
voice states; idle/unknown observations release that suspension. A Live provider
can answer directly without starting an agent run;
that does not light the agent badge. User transcripts or elapsed time never
create a busy state. Normal agent turns and tool execution are visible for as
long as the Gateway reports them active. Work that starts and ends between reads
can be missed; update latency is normally approximately one second plus read time.

## Native voice consultations

A native voice request has its own observable delegation lifecycle. A valid
client-targeted `delegation.created` or `session.delegation.created` immediately
sets the activity signal; it remains set until the corresponding
`delegation.context.appended` or `session.commentary.appended` result. This means
an outstanding host consultation, including dispatch/policy/approval waits, not
proof that a particular tool already executed. It requires no transcript or
reasoning access and does not wait for the next metadata sample.

Native consultations are serialized. A newer delegation replaces the pending
provider request; late results for older IDs cannot clear it. Duplicate IDs,
non-client targets and unrelated results do not create or clear work. A current
result releases only this signal, not independently active agent/child/process
metadata. Transport/session close, failure and activation cancellation clear it;
a new activation starts empty. Speech or a launch acknowledgment alone never
sets it. Metadata failures do not hide a still-live native consultation.

## Background-work limits

The Gateway's active descendant flag includes represented child work even when
the parent has yielded. Session-scoped managed background processes are included
while the Gateway reports
a positive running-process count, even after the agent turn has ended. Completed
processes retained in history do not count. A detached/unregistered process,
external job, future scheduled turn, or unrepresented runtime is NOT inferred
to be active after the agent run ends.
No claim of complete background-job coverage is made. An explicit active child
or positive process count is sufficient evidence of work;
otherwise the direct run flag is required. Missing metadata is not an authoritative
idle observation.

## Presentation

A compact 64×44-pixel capsule contains three dot slots, not a visible text bar.
While verified work is active, dots build and recede as 1 → 2 → 3 → 2 → 1 with
350 ms steps. At least one dot stays visible and geometry never changes.
Split view keeps it above the fixed actions; fullscreen keeps it at the lower
right beside the primary controls, clear of the avatar. The native surface
exposes exactly the same compact rectangle; the application owns drawing/input.

The existing Talk status tick advances the phase. Only changed dot counts request
a render-only refresh through the canvas's live VersionFn; animation does not
rebuild captions/layout or add a timer/goroutine. Idle, covered, finished and
motion-paused pages request no animation frames. Delayed ticks jump to the current
phase without replaying a backlog.

Tapping the capsule pauses motion at three static dots; tapping again resumes.
This reduced-motion alternative is local to the current call/page and survives
layout changes, without changing saved settings or stopping work. Accessible
labels retain localized “Agent working” meaning plus a pause/resume description
in English, German, Italian and Vietnamese. The name does not change per frame.
No flashing, extra session, capture listener, speech request or spoken progress
is added. Switching layouts does not restart Talk or the renderer.

## Verification boundaries

Unit/wire tests cover active/idle/child flags, absent metadata, exact target and
incarnation fences, scope denial, reconnect, failure, cancellation and freshness.
GUI tests cover both layouts, every supported language, fixed primary controls,
caption scrolling, native aperture geometry and idle removal. Native protocol
and Go scalar-feed tests cover setting and clearing the aperture without restart.
Real-device results and any remaining hardware/provider limitations are recorded
separately in the pull request; synthetic checks are not spoken/acoustic proof.
