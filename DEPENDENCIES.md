# Project Dependencies

Exact Go package versions are declared in `go.mod` and checked by `go.sum`.
Verify tooling on each development machine rather than assuming it is installed.

| Requirement | Purpose | Needed for | Verification |
| --- | --- | --- | --- |
| Windows 10/11 | Core Audio endpoints and audio policy COM interfaces | Running audio commands | Run `audioswap devices` |
| Go 1.25+ | Build, formatting, static checks, and tests | Development only | `go version` |
| Git | Local version control | Development only | `git --version` |
| `github.com/moutend/go-wca` | Core Audio enumeration and device properties | Build dependency | `go mod verify` |
| `github.com/go-ole/go-ole` | COM initialization, GUIDs, and interface lifetime | Build dependency | `go mod verify` |

`IPolicyConfig` is an undocumented Windows interface used for switching defaults;
future Windows changes may require updating its small binding in
`audio_windows.go`. Audio endpoint IDs are machine-specific and belong only in the
ignored local `audioswap.json` file.

There are no services, databases, external executables, C toolchain requirements,
or additional runtime installations. The text database is a local JSON file.
