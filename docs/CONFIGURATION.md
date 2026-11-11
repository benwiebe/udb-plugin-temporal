# Temporal configuration

UDB loads this plugin through a compiled blank import. The plugin has no plugin-level settings, so no `plugins` entry is required. Board and datasource configuration uses the plugin ID `udb-plugin-temporal`.

All five boards are dynamic. Use a positive `durationSeconds` to rotate between them; zero or omitted means hold that board indefinitely. Datasources must be declared in the `datasources` array and selected through the board's `datasource` field, or matched by type when exactly one compatible datasource exists.

## Common time-board settings

These settings apply to `digital-clock`, `date`, `clock-date` and `countdown`:

| Setting | Default | Meaning |
|---------|---------|---------|
| `timezone` | System local timezone | IANA timezone, e.g. `America/Toronto` or `UTC` |
| `colour` | `#FFFFFF` | Text colour; fallback for both lines on stacked boards |
| `backgroundColour` | `#000000` | Background colour |
| `padding` | 10% of width and height | Nonnegative pixel padding on each side |
| `fillScreen` | `false` | Use zero padding; when `true`, overrides `padding` |
| `fontStyle` | `auto` | `auto`, `dotMatrix` (5×7 text), `scoreboard` (3×7 digits), `condensed` (3×5 text), `outlined` (outlined 5×7 digits), `silkscreenBold`, or `pixelifySansBold` |
| `fontScale` | `0` (automatic) | Positive integer pixel scale; bold text faces are rasterized and enlarged in hard-edged pixel blocks; vertical overflow is an initialization error, while long lines are still shortened |
| `horizontalAlign` | `center` | `left`, `center`, or `right` |
| `verticalAlign` | `center` | `top`, `center`, or `bottom` |

Colours accept six-digit RGB hex values with or without `#`. Font scales are integer pixel multiples; requested ratios are approximate. The bold faces use their own base raster sizes, so scale `1` does not have identical dimensions across styles. Small displays automatically use condensed bitmap glyphs where needed. Invalid dimensions or padding that leaves no usable text area fail initialization.

`silkscreenBold` and `pixelifySansBold` are full-text faces, unlike the numeric-only `outlined` style. They can render mixed-case text, digits, and punctuation; use the explicit style on any temporal board and set `fontScale` to `0` or omit it to fit the chosen face automatically. These shared `udb-plugin-library` styles are available to any plugin on v0.8.0 or later. Their SIL Open Font License notices are included with the library assets: [Silkscreen](https://github.com/benwiebe/udb-plugin-library/blob/main/fonts/assets/Silkscreen/OFL.txt) and [Pixelify Sans](https://github.com/benwiebe/udb-plugin-library/blob/main/fonts/assets/Pixelify_Sans/OFL.txt). The fonts are bundled from [Google Fonts](https://github.com/google/fonts/tree/main/ofl/silkscreen) and [Pixelify Sans](https://github.com/eifetx/Pixelify-Sans).

For example, a `date` board can try Pixelify Sans Bold with automatic sizing and no padding:

```json
{
  "format": "Monday, Jan 2",
  "fontStyle": "pixelifySansBold",
  "fillScreen": true
}
```

The font settings also apply to `upcoming-events`; that board keeps its fixed scale-1 default when no font setting is given. Stacked boards use `fontStyle` and `fontScale` for their primary line. `clock-date` accepts `dateFontStyle` and `dateFontScale`, while `countdown` accepts `labelFontStyle` and `labelFontScale`; without a secondary `fontScale`, the existing `dateScale` or `labelScale` ratio remains in effect. A style override without a scale still fits automatically. `fontScale: 0` is equivalent to automatic sizing.

For example, this uses large outlined digits with no padding, inspired by a clock-and-status display:

```json
{
  "format": "15:04",
  "fontStyle": "outlined",
  "fontScale": 2,
  "fillScreen": true,
  "colour": "#45E6DD"
}
```

Time layouts use Go's reference time, `Mon Jan 2 15:04:05 MST 2006`. For example, `15:04` is a 24-hour clock, `3:04 PM` is a 12-hour clock, and `Monday, January 2` is a long date. Formats are Go layouts, not `strftime` patterns. Month and weekday names are English. The pixel fonts primarily support ASCII and render unsupported characters with a fallback glyph.

### Digital clock

| Setting | Default | Meaning |
|---------|---------|---------|
| `format` | `15:04` | Go time layout; use `15:04:05` for seconds |
| `blinkColon` | `true` | Hide colon separators on alternating seconds |

### Date

| Setting | Default | Meaning |
|---------|---------|---------|
| `format` | `Mon Jan 2` | Go date layout |
| `uppercase` | `false` | Uppercase formatted text |

The date refreshes at least once per minute and at the next local midnight.

### Clock and date

| Setting | Default | Meaning |
|---------|---------|---------|
| `timeFormat` | `15:04` | Top-line layout |
| `dateFormat` | `Mon Jan 2` | Bottom-line layout |
| `timeColour`, `dateColour` | `colour` | Per-line overrides |
| `blinkColon` | `true` | Blink separators in the time line |
| `dateUppercase` | `false` | Uppercase the date |
| `dateScale` | `0.45` | Positive date scale relative to time |
| `dateFontStyle` | `fontStyle` / automatic | Optional date-line style override |
| `dateFontScale` | `dateScale` ratio | Optional absolute date-line pixel scale |
| `lineGap` | `2` | Nonnegative gap in pixels |

### Countdown

| Setting | Default | Meaning |
|---------|---------|---------|
| `target` | Required | RFC3339 timestamp, `YYYY-MM-DD HH:MM:SS`, `YYYY-MM-DD HH:MM`, or `YYYY-MM-DD` |
| `label` | Empty | Optional label below the countdown |
| `labelColour` | `colour` | Label colour |
| `showSeconds` | `true` | Display seconds |
| `expiredText` | `NOW` | Text after the target is reached |
| `labelUppercase` | `false` | Uppercase the label |
| `labelScale` | `0.35` | Positive label scale relative to countdown |
| `labelFontStyle` | `fontStyle` / automatic | Optional label style override |
| `labelFontScale` | `labelScale` ratio | Optional absolute label pixel scale |
| `lineGap` | `2` | Nonnegative gap in pixels |

Zone-less targets use `timezone`, or the system local timezone. RFC3339 offsets are authoritative. Countdown days represent 24-hour intervals. Labels wrap and are shortened when they exceed the available height. Disabling seconds still schedules a refresh at expiry rather than waiting for the next full minute.

## Upcoming events board

`upcoming-events` accepts either calendar backend through the shared event contract. It shows the next event by default; larger panels can show a list. Ended events disappear on the next render, even between datasource polls.

| Setting | Default | Meaning |
|---------|---------|---------|
| `maxEvents` | `1` | Positive maximum number of visible events, further limited by panel height |
| `timezone` | System local timezone | Display timezone for timed events |
| `timeFormat` | `15:04` | Go time layout |
| `dateFormat` | `Jan 2` | Go date layout; today's and tomorrow's events use relative labels |
| `showLocation` | `false` | Include location when space permits |
| `emptyText` | `No events` | Text for a successfully refreshed empty calendar |
| `colour` | `#FFFFFF` | Title colour |
| `timeColour` | `#64B4FF` | Date/time and status colour |
| `backgroundColour` | `#000000` | Background colour |
| `padding` | 10% of width and height | Nonnegative pixel padding |
| `fillScreen` | `false` | Use zero padding; when `true`, overrides `padding` |
| `fontStyle` | `auto` | `auto`, `dotMatrix`, `scoreboard`, `condensed`, `outlined`, `silkscreenBold`, or `pixelifySansBold` |
| `fontScale` | `0` (automatic) | Positive integer pixel scale; must leave capacity for an event and title |

The board requires at least 11 pixels of usable height for a date/time and title. Long titles wrap, then end in `...` when necessary. The board re-renders every second and immediately after a datasource change. Ongoing timed events display `Now` and their end time; all-day events display `All day`. All-day dates retain the source calendar's timezone rather than shifting to another date in the display timezone.

Before the first successful refresh, the board displays `Calendar unavailable`. If a later refresh fails, it retains unexpired events and marks them `STALE`. If no usable cached events remain during an outage, it shows the unavailable state rather than claiming the calendar is empty. Detailed fetch errors are logged, not displayed on the panel.

## Calendar datasources

Both backends supply `benwiebe/udb-plugin-temporal/calendar-events` and share these settings:

| Setting | Default | Meaning |
|---------|---------|---------|
| `refreshSeconds` | `300` | Positive polling interval |
| `lookBackDays` | `1` | Nonnegative query/expansion lookback |
| `lookAheadDays` | `30` | Positive query/expansion horizon |
| `maxEvents` | `10` | Positive cap after filtering and sorting |
| `includeAllDay` | `true` | Include all-day events |
| `timezone` | System local timezone | Interpretation of floating times and all-day dates |

Window days are 24-hour durations. Expired events are filtered out; lookback allows retrieval/expansion of ongoing events, not display of historical events. Increase lookback for long-running recurring events, especially with server-side CalDAV expansion. All-day ends are exclusive calendar dates, and recurring all-day durations follow calendar days across DST changes.

Set datasource `maxEvents` above the board's visible count to keep later cached events available between polls. Successful refreshes sort by start time and retain ongoing events. Failed refreshes keep the last good result and continue retrying; an initial outage does not disable the datasource. Context cancellation stops polling.

### ICS / iCalendar

Datasource ID: `ical-calendar-events`.

| Setting | Default | Meaning |
|---------|---------|---------|
| `url` | Either `url` or `path` required | HTTP(S), `webcal://`, or `webcals://` feed URL |
| `path` | Either `url` or `path` required | Local ICS file path |
| `maxBytes` | `8388608` (8 MiB) | Positive maximum feed size |

Specify exactly one source. Both webcal schemes normalize to HTTPS. Network requests have a 15-second timeout and use `ETag` / `Last-Modified` validators. Unchanged local files reuse the parsed calendar. Even when the source is unchanged, recurrence expansion and event filtering advance with the query window. Validators/file versions are acknowledged only after successful decoding.

Supported recurrence includes `RRULE`, comma-separated `RDATE` and `EXDATE` date lists, RDATE-only series, single-occurrence `RECURRENCE-ID` replacements, and cancellations. Invalid events are skipped with a warning so valid events remain available.

Current limitations: `RECURRENCE-ID;RANGE=THISANDFUTURE`, RDATE period values, embedded custom `VTIMEZONE` definitions, and provider-specific/non-IANA timezone IDs are not fully supported. Prefer CalDAV server expansion for complex recurrence and large calendars. The entire ICS feed is downloaded when modified, and cached recurrence is re-expanded on each poll.

### CalDAV

Datasource ID: `caldav-calendar-events`.

| Setting | Default | Meaning |
|---------|---------|---------|
| `endpoint` | Required | HTTP(S) DAV endpoint |
| `calendarPath` | Selector required | Explicit calendar collection path or URL |
| `calendarName` | Selector required | Exact display name resolved through principal/home-set discovery |
| `username`, `password` | Empty | Optional HTTP Basic authentication credentials |

Specify `calendarPath` or `calendarName`; an explicit path takes precedence. A missing or non-event calendar name causes an unavailable state and retries; it never silently selects another calendar. For services with two-factor authentication, use the service's app password if required.

CalDAV issues bounded time-range REPORT queries with server-side recurrence expansion. Query timestamps are converted to UTC before encoding. Discovery and requests have a 15-second timeout. OAuth, provider-specific discovery shortcuts, and automatic fallback for servers that reject expansion are outside this MVP.

## Shared calendar data contract

`datasources.CalendarEventsData` contains `Events`, `LastUpdated` (last successful refresh), `LastError` (latest refresh failure, cleared on success), and `Source`. Each `CalendarEvent` contains `ID`, `Title`, `Start`, `End`, `AllDay`, `Location`, and `Description`.

An expanded or overridden occurrence uses `UID/<original-occurrence-start-in-UTC>` as its ID. Boards should depend on the datasource interface and this data shape, not the concrete ICS or CalDAV implementation. `GetData()` returns cached data immediately and copies the event slice. `DataChanged()` signals event changes, initial successful loading, and error/recovery transitions; it does not signal every unchanged successful poll.
