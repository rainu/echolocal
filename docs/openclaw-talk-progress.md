# Continuous Talk implementation checkpoint

Issue: https://github.com/rainu/echolocal/issues/9

This is an incomplete implementation, not an end-to-end voice acceptance result.

## Authorization and control transport

Connection-only clients still request no scopes. Explicitly enabling Talk
requests exactly `operator.read` and `operator.talk`, including both scopes in
the signed device handshake. Gateway approval remains separate from the local
preference; setting the preference does not grant privileges.

The backend RPC path shares the paired WebSocket, correlates concurrent replies,
limits outstanding requests to 16 and request payloads to 128 KiB, and has bounded
write and response deadlines. Cancellation removes the request locally. Socket
loss returns an uncertain-delivery error, never replays a request, and never
transfers outstanding calls to a replacement connection. Gateway error messages
and response payloads do not enter the connection diagnostic buffer.

RPC responses can contain short-lived credentials and must remain backend-only.
The current UI does not enable this preference or call this interface yet.

## Actual hardware protocol verification

A separate private probe on the Echo verified the existing device identity and
public key before upgrading the operator scopes. No node or administrator role
was added. The scoped handshake and authenticated Talk catalogue succeeded.

A microphone-free, audio-only WebRTC probe then negotiated
`gateway-control-v1`, `realtime`, and `agent-consult`. Talk session creation,
SDP offer/answer, connected peer state, and explicit logical session closure all
succeeded. No microphone was opened and no spoken request or agent consultation
was submitted. This does not prove audible output, agent identity/persona,
history continuity, echo cancellation, or barge-in.

Every activation must use a fresh voice-session identifier. Reusing a closed
voice-session identifier was rejected, while a fresh identifier succeeded.

The probe is deliberately outside the repository: it accesses the device's
private credential store. No credentials, signaling data, deployment endpoints,
or private probe source are published.

## Probe build limitations

The standard device build is Linux/ARM with CGO disabled. Its Go resolver is initialized by the application's DHCP component. A
standalone probe does not run that component. During this check, the platform's DNS property
slots used by that setup were empty; the standalone Linux probe could not resolve
the Gateway. An Android/CGO probe could resolve it, but the current transitive
Android networking dependency references a private Go runtime symbol rejected
by the default linker. An experimental probe-only linker compatibility override
allowed the transport feasibility test; it is NOT an adopted production build
change. This standalone-probe failure is not evidence of a production DNS failure: the
DHCP library installs its own lease-backed resolver. The final transport must
still be verified inside the actual application lifecycle.

Remaining work includes assistant routing and conditional settings, agent/context
selection, all-catalogue localization, WebRTC media integration with the Echo's
existing audio/AEC lifecycle, visible controls, end-phrase handling, physical
mute, and the complete real-device acceptance suite. Existing Home Assistant
routing and the installed EchoLocal binary have not been changed by this
checkpoint.
