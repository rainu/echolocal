# Node registry and safe settings coverage

The historical raw local-entity inventory is retired. A locally registered
entity is not necessarily a published or authorized Node tool. Publishing that
unfiltered inventory as agent guidance also enumerated excluded configuration.

Use the maintained contracts instead:

- [Safe settings coverage across all registered boards](openclaw-settings-coverage.md)
- [Setting discovery, value formats, grants and diagnostics](openclaw-node-settings.md)
- [Node tools and invocation contracts](openclaw-node-tools.md)

The live node-catalog diagnostic reports the actual currently published tool
schemas. Settings discovery reports only reviewed Domain/Name pairs registered
on the current hardware/build and permitted by local grants, without values or
excluded configuration. Do not infer API support from local entity IDs.

The pinned, isolated TestNodeSettingsCoverageAudit checks every registered field
against an explicit review or exclusion. Private audit JSON must not be published
as an unfiltered agent catalog.
