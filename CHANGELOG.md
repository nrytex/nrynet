# Changelog

## v1.3.9 - 2026-11-17

### Fixed

- Reset the tunnel editor when switching between tunnels or opening the create form, so fields do not retain another tunnel's values.
- Preserve an HTTP tunnel's configured remote port when opening its editor.
- Refresh client editor fields and server setting values when their selected records change.
- Clear an unfinished token name when the create dialog is cancelled.

### Verification

- Added dashboard interaction tests for tunnel, client, token, and setting forms.
