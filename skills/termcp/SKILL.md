---
name: termcp-http
description: Use a running Termcp Core through its documented HTTP and TERM/1 interfaces.
---

# Termcp HTTP

Use the Core instance address shown in Termcp Desktop. Do not assume a remote or
alternative address.

1. Read `GET /api.md` for the REST and WebSocket contract.
2. Send command calls to `POST /api/term` as a JSON object with a `cmd` string.
3. Call `POST /api/term` with an empty object or `{"cmd":""}` for the command
   guide.
4. Use `{"cmd":"help <directive>"}` to read the current directive reference.
5. Follow the TERM/1 status code, headers, body, cursor, and error-code fields
   returned by Core. Do not substitute shell parsing for the command language.

The HTTP MCP endpoints are `/sse` and `/stream`. If the Core instance requires
authentication, send its configured bearer token through the normal
`Authorization` header. Never place credentials in a URL.

This skill is a Desktop documentation export. The running Core remains the
source of truth for its available commands and current protocol.
