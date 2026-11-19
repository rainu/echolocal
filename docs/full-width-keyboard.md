# Full-width touch keyboard (rainu fork)

Adds a fourth screen keyboard size, `full_width`, preserving Compact, Standard and Large. The ten-key row spans the viewport minus outer padding; key height is width / 1.6. On Athena (1280x800), keys measure about 121x76 pixels and leave about 8 pixels of outer padding. The search field stays visible.

## Dependencies

Implementation and geometry tests now live in https://github.com/rainu/libcountertop, commit `fea50415103f7fe0b7b3c1fb449fd7372046e4aa`. EchoLocal retains original import paths and pins the fork's exact Go pseudo-version with a remote `replace` in go.mod. Normal go/make commands require no local patch preparation or sibling checkout. Module cache remains unmodified.

The previous temporary patch/helper/local dependency were moved out of the repository to `/work/migration-archive/fullwidth-local-patch` during migration, not used for builds anymore. Active library checkout: `/work/libcountertop` (origin rainu, upstream ygelfand).

## Selection and verification

Settings > System > Keyboard size > Full width, or `echod ctl settings screen.keyboard full_width`.

Library tests: `go test ./pkg/display/widgets ./pkg/settings/schema`. Geometry covers 1280x800, 800x480 and 720x1152, full width and vertical fit, with taller keys than Large on Athena. The initial deployed UI test used version `0.0.15-dev.fullwidth.1` with `f6b596d-dirty`; screenshot `/work/keyboard-fullwidth.png` confirms full-width keys. Fork migration does not change keyboard behavior.
