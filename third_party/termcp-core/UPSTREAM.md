# termcp Core v0.2.9 desktop patch

This module is source from upstream tag `v0.2.9`:

- release: <https://github.com/open-mcp-ai/Termcp/releases/tag/v0.2.9>
- commit: `714f8d491d0913fd7012afadc7f22f062e213839`
- release date: 2026-11-08

Desktop version display: `0.2.9+desktop.1`

Patch:

- `../patches/termcp-v0.2.9-desktop.1.patch`
  - SHA-256: `2705165dc26906ca33dea87402e418571845c468336c997bf0f618988382b832`
  - verified with `git apply --check` against commit `714f8d491d0913fd7012afadc7f22f062e213839`
- `no_webui` retains the Core management REST API, WebSocket, extension
  reconciler, approval execution callback, client events, and `/api.md`.
- URL templates always get the internal pipe relay required by their native MCP
  peer and `/ext/{name}/stream`.
- A direct call through an extension peer is refused after that template's URL,
  headers or other fingerprint fields change; the caller reopens the extension.
- Browser page assets remain absent. Only `internal/webui/assets/api.md` is
  retained from the upstream asset tree.
- Regressions are in `core/no_webui_test.go`,
  `internal/extensions/url_relay_shell_test.go`, and
  `internal/mcp/ext_peer_fingerprint_test.go`.

Remove this local replacement once upstream publishes an immutable module
version containing equivalent API-only wiring and Desktop has moved to it.
