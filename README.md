# udb-plugin-temporal

Temporal boards and supporting datasources for UDB.

This plugin is the home for time-centric boards that can be composed into the main UDB app through the shared plugin library.

## Current status

- Plugin scaffold is in place.
- `boards/digital_clock.go` is implemented and registered as `digital-clock`.
- `boards/date.go` is implemented and registered as `date`.
- `boards/clock_date.go` is implemented and registered as `clock-date`.
- `boards/countdown.go` is implemented and registered as `countdown`.
- The digital clock already supports a small config surface:
  - `format` for Go time layouts
  - `colour` for digit colour
  - `blinkColon` to hide colons on alternating seconds

## Board ideas

### Digital clock

A polished, configurable clock board with a small but useful config surface.

Possible config options to keep exploring:

- font style or theme presets
- padding / alignment modes
- background colour or transparency
- seconds display toggle
- 12h / 24h presets via format helpers
- locale / timezone handling if that belongs in the datasource or board config

### Date board

A date-focused board that can stay separate or be folded into the clock board as a variant.

Things to decide:

- standalone date board vs combined clock/date board
- format presets for common date styles
- weekday inclusion
- locale awareness

### Combined clock + date board

A stacked layout that shows time and date together in one board.

Useful config directions:

- separate `timeFormat` and `dateFormat`
- per-line colour overrides
- compact versus spacious stacked layout
- date size ratio relative to time

### Countdown board

A board that counts down to a target date/time.

Possible behaviours:

- target as absolute date/time
- display in days/hours/minutes/seconds
- end-of-countdown state
- optional label for the target event

### Upcoming calendar events

A board that shows the next event or a short list of upcoming events.

This likely wants a datasource abstraction that can support multiple calendar backends, for example:

- iCal / ICS feeds
- CalDAV
- other calendar sources if we add them later

Questions to settle:

- how calendar datasources are configured
- polling vs push update model
- how far ahead to look for events
- how all-day events should render

### Other temporal boards worth considering

- world clock / timezone board
- timer / stopwatch board
- elapsed-time board
- sunrise / sunset board if we decide that belongs here

## Progress tracker

- [x] Plugin scaffolding
- [x] Digital clock board
- [x] Date board
- [x] Combined clock + date board
- [x] Countdown board
- [ ] Upcoming events board
- [ ] Calendar datasource abstraction
- [ ] Calendar backend support (`iCal`, `CalDAV`, etc.)
- [ ] Config examples and screenshots

## Notes

The goal is to keep the board implementations small and composable so the main app can mix and match temporal boards without special-case logic.
