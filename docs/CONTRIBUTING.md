# Contributing to UDB Temporal

Thank you for contributing to `udb-plugin-temporal`! This guide explains how the plugin is structured, how to set up your local development environment, how to build and test your changes, and how to submit contributions.

## Overview & Architecture

`udb-plugin-temporal` is a Go plugin module for [UDB (Universal Display Board)](https://github.com/benwiebe/udb-core). It provides time, date, countdown, and calendar boards and datasources designed for Raspberry Pi LED matrix displays and HTTP virtual previews.

### Core Concepts

- **Plugin Registration (`plugin_temporal.go`)**: Implements `types.Plugin` from `udb-plugin-library`. In its `init()` function, it registers itself via `udb_plugin_library.Register(&UdbPluginTemporal{})`.
- **Boards (`boards/`)**: Implement `types.Board` or `types.DynamicBoard`. Boards are responsible for layout and rasterization of pixels into `image.RGBA` frames within `Render()`.
  - *No Network on Render Path*: All rendering is synchronous and in-memory. Network fetching and parsing belong in datasources.
  - *Font & Layout System (`boards/font_layout.go`, `boards/text_layout.go`)*: Supports bitmap pixel fonts, integer scaling, multi-line wrapping, and outlined numbers.
- **Datasources (`datasources/`)**: Implement `types.Datasource`.
  - `current-time` (`CurrentTimeDatasource`): Lightweight, synchronous datasource returning `time.Now()`.
  - `ical-calendar-events` and `caldav-calendar-events`: Background polling datasources that fetch, parse, expand recurring events, and cache `CalendarEventsData` in memory. They signal changes via `DataChanged()` channels.
- **Shared Data Contract (`datasources/calendar_events.go`)**: `CalendarEventsData` and `CalendarEvent` provide a unified interface so calendar boards remain agnostic of whether events originate from local `.ics` files, HTTP feeds, or CalDAV servers.

## Development Setup

### Prerequisites

- Go 1.26.1 or later.
- Git.

### Recommended Workspace Layout

For local development and testing alongside UDB core, check out the sibling repositories in a shared directory:

```sh
git clone https://github.com/benwiebe/udb-core.git
git clone https://github.com/benwiebe/udb-plugin-library.git
git clone https://github.com/benwiebe/udb-plugin-temporal.git
```

In `udb-core`, enable Go workspace mode containing all three repositories:

```sh
cd udb-core
go work init . ../udb-plugin-library ../udb-plugin-temporal
```

Ensure `plugin_imports.go` in `udb-core` imports `udb-plugin-temporal`:

```go
import _ "github.com/benwiebe/udb-plugin-temporal"
```

## Building and Testing

From the `udb-plugin-temporal` directory:

```sh
# Run all checks (build, unit tests, and go vet)
make check

# Run tests with the Go race detector
make race

# Format and clean dependencies
make tidy

# Regenerate rendered documentation screenshots
make screenshots
```

### Standalone Verification

To verify that the plugin builds and passes tests using published dependencies rather than local workspace replacements:

```sh
GOWORK=off make check
GOWORK=off make race
```

### Testing with UDB Virtual HTTP Display

To test your changes live without physical LED hardware:

1. Build the UDB binary in `udb-core`:
   ```sh
   cd ../udb-core
   make build
   ```
2. Run UDB using an example configuration from `udb-plugin-temporal`:
   ```sh
   cd ../udb-plugin-temporal
   ../udb-core/udb -config examples/config.json
   ```
3. Open `http://localhost:8080` in your web browser to view the virtual display and board rotation.

## Guidelines for New Features and Changes

### Adding or Modifying Boards

- **Config Validation**: Validate all configuration parameters during `Init()`. Return clear, actionable error messages if dimensions or parameters are invalid.
- **Layout Robustness**: Guard against narrow or short display dimensions. Ensure fallback behavior (such as text truncation with `...` or switching to condensed fonts) prevents visual panics or crashes.
- **Timezones**: Always respect user-configured `timezone` settings and avoid naive timezone conversions.
- **Top-Level & Method Documentation**: Add top-level doc blocks and docstrings on all exported types, constructors, and key internal helpers following standard Go doc conventions.

### Adding or Modifying Datasources

- **Thread Safety**: Protect shared in-memory data using `sync.RWMutex`.
- **Fault Tolerance**: Retain the last known good data during transient network or server outages. Clear `LastError` upon successful recovery.
- **Change Signaling**: Send a non-blocking notification on `DataChanged()` when new data is parsed or when error state changes. Avoid spurious notifications on identical polls.

### Documentation Updates

- Whenever adding new configuration fields or modifying defaults, update [docs/CONFIGURATION.md](CONFIGURATION.md) and [README.md](../README.md).
- If board rendering output changes, regenerate screenshot assets using `make screenshots`.

## Submitting Pull Requests

1. Ensure `make check` and `make race` pass cleanly with zero warnings or test failures.
2. Ensure code follows standard Go formatting (`gofmt` / `goimports`).
3. Include tests covering new functionality, edge cases, and regression paths.
4. Keep commits focused and provide clear commit messages.
