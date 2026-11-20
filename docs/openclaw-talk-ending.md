# Configurable Live Talk ending (Refs #23)

Each OpenClaw assistant slot has **End conversation** settings: one editable
phrase (1–256 characters, a single line), **Contains / Start / End** position,
and independently **End of turn / Immediately** timing. Changes apply to the
next activation and persist atomically with the slot, not in provider prompts.
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
