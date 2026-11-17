# Input-aware mirroring (macOS)

When two computers share a monitor, changing its input may leave macOS using an invisible extended desktop. Input-aware mirroring makes that external display mirror the MacBook before switching to the other computer, then restores the saved layout and display modes when returning to the Mac.

## Setup and use

In **Picture → Input-aware mirroring**, enable the feature and the selected display. Both options are off by default. Disable the per-display switch to edit the Mac and Windows input mappings. HDMI 1 (`0x11`) and HDMI 2 (`0x12`) are the defaults; choose the physical monitor inputs in use. A dock reporting DisplayPort does not identify its HDMI connector.

Use **Switch to Windows** and **Switch to Mac**, the Overview input buttons, or existing input shortcuts. The same switching path is used by all three. **Restore extended layout** restores an owned layout without changing the physical input.

Configured inputs remain available when a dock temporarily fails to return the MCCS capability string. Recorded input shortcuts also provide fallback sources on a connected DDC display. A valid capability list continues to take precedence for other sources.

## Behavior and limits

- The built-in display must be primary before mirroring. Preexisting mirror sets are left alone.
- The app retains a snapshot of display modes and origins, tracks online mirror secondaries, and restores only layouts it still owns. Manual display changes revoke ownership.
- External gamma adjustments and brightness following pause while mirrored, protecting the built-in display. Display callbacks during an app-driven switch do not tear down the DDC channel used by that switch.
- Polling is asynchronous, requires consecutive valid readings, and uses a cooldown. Failed or unknown input readings never imply a switch to Windows.
- Inactive-cable DDC support varies. Some docks stop replying or return stale Mac input values while Windows is visible. After an app-driven Windows switch, stale Mac readings cannot immediately undo mirroring: Windows must first be confirmed, or the user must request **Switch to Mac**. If the display was switched manually and remains mirrored, use **Switch to Mac** or **Restore extended layout**.
- A successful DDC write acknowledges transport only; it does not prove the monitor has displayed the requested source. Do not rely on automatic detection with an unreliable dock.
- Disabling the feature or quitting restores an owned layout. Windows builds retain their existing display behavior; this feature is macOS-only.

## Brightness follow and DDC queue

Brightness following samples the built-in display about every 33 ms and uses a short exponential glide, settling in roughly half a second in the animation model. Actual hardware timing depends on DDC and the monitor. Media-key permission checks remain at one second. An unchanged source preserves manual external brightness adjustments.

DDC queue state and bus transactions have separate locks. UI producers can replace unsent values without waiting for slow monitor I/O; reads and writes remain serialized on the bus. Close is idempotent and late operations on closed transports are ignored.

## Validation

```sh
go tool mygo generate
go test -p 1 ./...
go test -race -p 1 ./internal/display
go vet -p 1 ./...
GOOS=windows GOARCH=amd64 go vet -p 1 ./...
GOOS=windows GOARCH=amd64 go build -p 1 -o build/MonitorBar.exe .
go tool mygo build -platform darwin/arm64 -skip-dmg
```

Tests cover debounce, failed and stale reads, explicit return requests, ownership, exact restore arguments, preference compatibility, missing capability lists, retained shortcuts, follow timing, and nonblocking/coalesced DDC writes. Hardware validation is separate from these tests.
