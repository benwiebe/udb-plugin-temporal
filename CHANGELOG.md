# Changelog

## Unreleased — v1.0.0 MVP

- Add the Upcoming events board, with next-event and list layouts, all-day and ongoing labels, wrapped titles, optional locations, and unavailable/stale states.
- Preserve the parsed ICS calendar across unchanged responses so moving query windows and event limits replenish correctly.
- Handle cancelled events, single-occurrence replacements, RDATE-only recurrence, and comma-separated recurrence/exclusion dates; preserve all-day calendar durations across DST.
- Commit HTTP/file cache validators after decoding succeeds, retain last good data during outages, and retry initial failures.
- Correct CalDAV expansion placement and UTC query timestamps; require named calendars to match.
- Use plugin-library v0.7.0 for standalone bitmap-font builds.
- Apply countdown labelScale, fit wrapped labels, refresh at expiry, size date text for longer dates, and update at local midnight.
- Add config examples, rendered screenshots, a full configuration reference, regression tests, and standalone CI checks.
