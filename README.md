# AudioSwap

A small, standalone Windows command-line utility that rotates through an ordered
list of audio output device IDs. No background service, PowerShell module,
NirSoft utility, or administrator access is required.

## Build

Requires Go 1.25 or newer on Windows:

```powershell
go build -o .\bin\audioswap.exe .
```

Keep the executable in a user-writable folder. Configuration lives beside it,
so you can invoke it from any working directory.

## Commands

```powershell
.\bin\audioswap.exe devices
.\bin\audioswap.exe add "{output-device-id-from-devices}"
.\bin\audioswap.exe add "{another-output-device-id}"
.\bin\audioswap.exe targets
.\bin\audioswap.exe
.\bin\audioswap.exe --include-comms
.\bin\audioswap.exe remove "{output-device-id}"
.\bin\audioswap.exe help
```

`devices` lists installed playback endpoints, including unplugged and disabled
ones, with their names, states, IDs, and current default roles. Historical
endpoints belonging to removed adapters are excluded. Copy the full ID,
including its braces. `add` validates the ID against that list and appends it to
the rotation. Duplicate and unknown IDs are rejected. `remove` works even if the
device is no longer installed.

Running without arguments selects the next available configured output after
the current **multimedia** default, wrapping around the list. Devices that are
disabled, disconnected, or no longer installed are skipped. If the current
default is not configured (or there is no default), the first available target
is selected. If there is no different available target, the command reports an
error and does not change any defaults.

A normal swap changes **console and multimedia** output defaults.
`--include-comms` also changes the **communications** output default; microphone
defaults are never changed. The utility checks that Windows accepted each change
and attempts to restore previous defaults if a later change fails. Any restoration
failures are reported explicitly.

Applications pinned to a specific output may ignore system default changes.
The utility does not force existing audio streams to move. Speakers and headphones
must appear as separate Windows playback endpoints.

## Targets file

The first successful `add` creates `audioswap.json` beside the executable:

```json
{
  "targets": [
    "{first-output-device-id}",
    "{second-output-device-id}"
  ]
}
```

Array order is rotation order. `targets` prints the file location and current
rotation. You can edit the file to reorder targets while the utility is not
running. Save valid UTF-8 JSON with a `targets` array and unique, nonempty IDs.
An empty array is allowed; an empty file is not. Malformed files are reported,
not silently reset. Updates use a temporary file followed by replacement, and
local configuration and built binaries are ignored by Git.

Do not run simultaneous swaps or configuration updates: commands do not lock
against other instances.

## Validation

```powershell
go test .\...
go vet .\...
```

Tests use fake audio devices and do not reroute your audio. To additionally
exercise the Windows API against this machine's endpoints:

```powershell
$env:AUDIOSWAP_INTEGRATION = "1"
go test -run TestWindowsAudio -v .
Remove-Item Env:\AUDIOSWAP_INTEGRATION
```

That opt-in test lists actual endpoints and reapplies existing defaults without
intentionally changing the selected outputs.

## Windows API compatibility

Endpoint enumeration uses Windows Core Audio through `go-wca` and `go-ole`.
Default switching uses Windows' undocumented `IPolicyConfig` COM interface.
This is the same interface family used by established audio switchers, but
Microsoft does not guarantee its compatibility across future Windows releases.
No C compiler or DLL bundled with audioswap is needed.

## License

AudioSwap is licensed under the [MIT License](LICENSE).
