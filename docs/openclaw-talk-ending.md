# Configurable Live Talk ending (Refs #23)

Each OpenClaw assistant slot has **End conversation** settings: one editable
phrase (1–256 characters, a single line), **Contains / Start / End** position,
and independently **End of turn / Immediately** timing. Changes apply to the
next activation and persist atomically with the slot.
Only user transcripts can trigger ending; closure is guarded once per activation.

## Defaults and migration

Older slot files without `ending` use the selected UI language: `End conversation`
(English/fallback), `Gespräch beenden` (German), `Termina conversazione` (Italian),
or `Kết thúc cuộc trò chuyện` (Vietnamese). Position defaults to `exact`, timing
to `turn_end`. The next successful slot save materializes these defaults; after
that a language change does not replace the saved phrase. Until that first save,
an old slot derives its phrase from the current UI language.

The stored position key `exact` is retained for compatibility, but now means
**Contains**: the complete normalized phrase can occur anywhere in the utterance.
Existing saved slots immediately gain this behavior without losing their phrase
or timing. Only complete word sequences match, not substrings inside words.

Matching uses Unicode NFC, case-insensitive words, normalized whitespace and
punctuation. Letter/number/combining-mark/underscore boundaries prevent matches
inside words. With `Gespräch beenden`, Contains matches both
`Gespräch beenden, danke` and `Okey, du kannst das Gespräch beenden, danke`.
Start matches only at the beginning; End only at the end. A middle mention
matches Contains but neither Start nor End. These are textual rules, not semantic
intent classifiers. Synonyms and translations are not additional hidden phrases.

## Interim recognition and transport limits

Immediately evaluates the accumulated user utterance on every available interim
update, with no turn-end gate for any position. Recognition may revise itself;
an early match cannot be undone after closing. In End mode the current interim
end is provisional. This behavior is documented here; all helper text below the timing selector was removed from
settings at the owner's request.

Native `session.input_transcript.delta` events and alternative Gateway
`transcript.delta` events append verbatim. Native `input_transcript.added` and
Gateway `transcript.updated` snapshots replace the pending text (including an
empty correction); they are not concatenated. New identified turns reset the
cache. Final native `turn.done` and Gateway `transcript.done` snapshots replace
and retire interim text. Assistant output never contributes to this cache.
On native-media sessions, Gateway interim copies are ignored; public Gateway
user finals remain authoritative as before. The cache is bounded and volatile.

Final-only transports cannot close before recognition publishes its final text.
This limitation is documented here; choosing Immediately does not
manufacture interim updates or silently change the selected policy. Existing
input-delta diagnostics indicate whether a transport is supplying partials.
End-of-turn never closes because of an interim-only match, including a match
later revised away. Final-only publications batched by the transport are evaluated
as the complete final text that transport actually supplies.

## Verification

Tests cover all position/timing combinations, language defaults, Unicode and
word boundaries, delta accumulation, replacing/empty revisions, turn resets,
assistant exclusion, final-only fallback, duplicate/concurrent closing events,
bounded input, invalid settings, restart persistence and localized UI choices.
Hardware/build results and exact reviewed commit are recorded in the linked
issue and PR; synthetic tests are not a physical speech/acoustics claim.

## Experimental voice-only prompt guard (Refs #60)

Each native EchoLocal Talk activation snapshots its effective end phrase,
including the selected language default or saved custom phrase. At the first
provider-confirmed user speech event, a silent transport-control guard is sent
once, before the optional device-context hint, using the existing public Live
or legacy native append contract. It is deliberately not sent on connection
startup: silent context can otherwise resume historical turns. It is not
attached to each user message, stored as a global agent instruction, or added
to ordinary typed chat.

The guard quotes the phrase as JSON data and tells the voice layer to leave
underlying text-session work running: no cancellation, pause, summary, task
change or substantive reply when ending voice transport. Discussion/quotation
of the phrase and unrelated explicit task-cancellation requests retain their
normal meaning. The guard uses English transport instructions for all supported
UI languages; the phrase itself is preserved verbatim. Saved changes apply to
the next activation only. A separate bounded append preserves the complete
user-written device context, even at its existing 1536-byte limit.

This is a prompt experiment, **not deterministic interception**. Recognition
has already happened on the server when the client receives a transcript.
The guard first reaches the live voice model; it is not an authenticated
filter on the backing text agent. Delegation, timing or model behavior can
still forward the end utterance before client closure. Existing position/timing
matching and text-session cancellation semantics are unchanged. In particular,
Contains matching can still close the voice connection during discussion about
the phrase; the prompt does not turn the local textual matcher into an intent
classifier.

Synthetic tests verify phrase preservation (including quotes, backslashes and
worst-case Unicode), per-payload bounds, once-per-activation ordering, no
startup send, unchanged device metadata, and failure handling on both native
wire contracts. They do not establish model compliance.

Before claiming effective protection, run real voice tests with active and idle
backing text sessions, another configured phrase and ordinary discussion about
the phrase. Inspect bounded session events/transcripts: record whether the
utterance reached the text agent, whether work continued, and whether any
spurious stop reply occurred. A missing forwarded utterance alone does not prove
that the text agent followed the guard. If the experiment fails, retain the
failure evidence and investigate a server-side transcription-to-agent filter
or explicit protocol support rather than describe this as guaranteed protection.
