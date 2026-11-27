# Contributing

## Settings and user-facing actions: definition of done

Every new supported user-configurable setting must ship in the same change with:

1. Appropriate native device UI, usable owner web UI and a documented
   owner-authenticated configuration API. A remote screen view or a generic
   partial list of primitive settings is not a replacement for a configuration flow.
2. Shared validation, canonical choice values, availability rules and the same
   persisted source of truth. Verify both directions after refresh and restart.
3. Equivalent supported lifecycle/configuration actions, conditional controls,
   localized labels, actionable errors and practical mobile/desktop layouts.
4. Protected credential update semantics. No plaintext credential read/export,
   request-value logging, extra authorization grants or agent self-administration.
5. Updated coverage inventory and regression checks for omissions, invalid input,
   unauthorized access, existing native flows and restricted Node permissions.

For registry-backed settings, verify the actual bound field appears in the
owner settings API and web renderer on every supported build/hardware variant.
For separately persisted fields/actions, extend the relevant owner API/page and
the maintained manifest, not only the registry or Node tool metadata.
[OpenClaw coverage](docs/openclaw-owner-settings.md) and its JSON manifest include
a reflection regression check for new private-store fields.

The only current temporary exception is the **experimental Avatar Lab**.
It may remain device/CLI-only during prototyping. Reassess web/API completeness
before promoting its configuration to a supported feature. This does not exempt
future avatar settings or any unrelated feature.

Do not claim live device/browser/physical verification based on synthetic tests.
Keep private configuration, credentials, network addresses and screenshots out
of public issues, test fixtures and artifacts. Use synthetic identities in tests.

## Cross-surface action contract

For every new user-facing action, evaluate and normally deliver the authenticated
API operation, OpenClaw Node command and native Home Assistant action together.
Include the HACS wrapper when it is a supported invocation route. Share the
underlying operation, names/argument meaning, validation, permissions, errors,
cancellation and accepted-versus-completed semantics; do not compose public
Stop/Start calls to imitate a transfer. Document any technically or semantically
unsuitable surface explicitly in the issue/PR. Cover equivalent requests and
failure states across surfaces and document invocation, discovery and outcomes.
Transport receipt delivery is not operation completion; test uncertain delivery,
busy/invalid input and recovery without ending an unaffected active operation.
