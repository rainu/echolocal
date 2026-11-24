# Dashboard date formats

The Clock settings expose **Date format** and **Custom date pattern**.
**Show date** still independently controls visibility, including the idle clock.
Existing settings keep their language-specific default date layout.

Presets: language default, dd.MM.yyyy, MM/dd/yyyy, yyyy-MM-dd,
d MMM yyyy, EEEE, d MMMM yyyy, and Custom.

Choose Custom to use the saved pattern. Tokens are case-sensitive:

| Token | Meaning |
| --- | --- |
| yyyy / yy | Four/two-digit year |
| MMMM / MMM | Localized full/short month |
| MM / M | Padded/unpadded month |
| dd / d | Padded/unpadded day |
| EEEE / EEE | Localized full/short weekday |

Single quotes delimit literals; two consecutive quotes insert an apostrophe.
Patterns are limited to 128 bytes and need at least one calendar field.
Unsupported tokens are rejected without overwriting the previous pattern.
Empty custom patterns and invalid legacy values fall back to the language default.

ESPHome exposes clock_date (switch), clock_date_format (select), and
clock_date_pattern (text). HACS controls these same settings. There is no
separate format preference to drift out of sync.
