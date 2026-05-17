# Security Policy

## Supported versions

`affectus` is pre-1.0. Security fixes are applied to the latest release and
the `main` branch only.

| Version | Supported |
|---------|-----------|
| latest release / `main` | yes |
| older | no |

## Reporting a vulnerability

Please report security issues privately — do not open a public issue.

Use GitHub's private vulnerability reporting: open the **Security** tab of
this repository and click **Report a vulnerability**. This creates a private
channel visible only to the maintainers.

Expect an initial response within about a week. If a fix is needed, it will
be released and the report disclosed once the fix is available.

## Scope

`affectus` is a local command-line tool with no network listener and no
authentication surface. It reads and writes local files (a YAML config and a
JSON state file) and offers an MCP server over stdio. Reports about local
file handling, the MCP tool surface, dependency vulnerabilities, or input
parsing are all in scope.
