# Streaming Talk captions (Refs #22)

The deployed Control UI transport handles session.input_transcript.delta and
session.output_transcript.delta as verbatim, non-final text without requiring
an item ID. turn.done supplies a final snapshot. EchoLocal now follows this
contract on the native channel and accepts transcript.delta/output.text.delta
on the Gateway alternative path.

Display paragraphs are inferred from consecutive same-role fragments. A role
switch starts a new paragraph. These are not asserted to be provider turn
boundaries. A provider-final text replaces the pending same-role paragraphs;
this also handles public Live Gateway publications batched by role. Consequently
final batch paragraph placement is approximate, not a sample-accurate timeline.
No provider-final event is invented at a role switch or on audio completion.
Native partials are authoritative when the native channel is enabled, avoiding
double appends from the Gateway subscription. Legacy turn.done and Gateway
transcript.done/output.text.done remain usable without partials.

Pending captions are bounded, volatile, and excluded from the local history
browser. Cancellation and closure discard unconfirmed text. Reconnect creates
a new activation. Only existing provider-final publication paths write remote
chat history. Delta counters expose availability without transcript payloads.
Device and active-provider verification must be recorded separately; source
inspection and synthetic tests alone do not prove active-provider availability.
