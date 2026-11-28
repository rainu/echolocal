# Verified agent activity in avatar Talk

Refs #99.

## State source

The Talk activation observes `sessions.describe {key: <current target>}` using
its existing operator read grant and exact authenticated socket. The adapter
retains only session incarnation and `hasActiveRun || hasActiveSubagentRun`.
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
playback. Fresh verified work suspends local idle expiry without changing those\nvoice states; idle/unknown observations release that suspension. A Live provider can answer directly without starting an agent run;
that does not light the agent badge. User transcripts or elapsed time never
create a busy state. Normal agent turns and tool execution are visible for as
long as the Gateway reports them active. Work that starts and ends between reads
can be missed; update latency is normally approximately one second plus read time.

## Background-work limits

The Gateway's active descendant flag includes represented child work even when
the parent has yielded. A detached process, external job, future scheduled turn,
or unrepresented runtime is NOT inferred to be active after the agent run ends.
No claim of complete background-job coverage is made. The direct run flag is
required; absent metadata is not an authoritative idle observation.

## Presentation

A static, text-labelled badge sits above the split layout's fixed action bar,
outside the caption scroller. Fullscreen uses a small lower-left badge beside
the two existing controls. The native surface exposes exactly that rectangle
only while verified work is shown. Its geometry is tested against the Go layout.
The native IPC carries a single boolean, never identifiers or content.

The restrained high-contrast text has an accessibility label and explicit
English, German, Italian and Vietnamese translations. It is static for everyone,
including reduced-motion users: no continuous animation, flashing, spoken
progress, extra session, capture listener, or speech request. Switching layouts
does not restart Talk or the renderer.

## Verification boundaries

Unit/wire tests cover active/idle/child flags, absent metadata, exact target and
incarnation fences, scope denial, reconnect, failure, cancellation and freshness.
GUI tests cover both layouts, every supported language, fixed primary controls,
caption scrolling, native aperture geometry and idle removal. Native protocol
and Go scalar-feed tests cover setting and clearing the aperture without restart.
Real-device results and any remaining hardware/provider limitations are recorded
separately in the pull request; synthetic checks are not spoken/acoustic proof.
