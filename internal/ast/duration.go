package ast

// ISO 8601 duration codec — the wire form of DateDuration. The grammar is the
// standard's: an optional leading '-' (whole-duration negation, ISO 8601-2),
// 'P', date components (Y, M, W, D; weeks expand to 7 days), then optional 'T'
// and time components (H, M, S). Integers only. Examples: "P30D", "-P30D",
// "P3M", "PT2H30M", "-P1DT12H".
//
// Hand-written rather than a dependency: the stdlib has no ISO duration
// support, the grammar is ~50 lines, and this package must stay
// dependency-free (deps_test) so the query authority is pure.

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseISODuration parses an ISO 8601 duration string into a DateDuration.
// A bare "P" or "PT" (no components) is an error; so is a zero-valued string
// like "P0D" — an empty interval matches nothing and is never meaningful.
func ParseISODuration(raw string) (DateDuration, error) {
	input := raw
	negative := false
	if strings.HasPrefix(input, "-") {
		negative = true
		input = input[1:]
	} else {
		input = strings.TrimPrefix(input, "+")
	}
	if !strings.HasPrefix(input, "P") {
		return DateDuration{}, fmt.Errorf("iso duration %q: missing 'P'", raw)
	}
	input = input[1:]

	datePart := input
	timePart := ""
	if t := strings.IndexByte(input, 'T'); t >= 0 {
		datePart, timePart = input[:t], input[t+1:]
		if timePart == "" {
			return DateDuration{}, fmt.Errorf("iso duration %q: 'T' with no time components", raw)
		}
	}

	var duration DateDuration
	var weeks int
	seen := false
	if err := parseComponents(datePart, map[byte]*int{
		'Y': &duration.Years, 'M': &duration.Months, 'D': &duration.Days, 'W': &weeks,
	}, &seen); err != nil {
		return DateDuration{}, fmt.Errorf("iso duration %q: %w", raw, err)
	}
	if err := parseComponents(timePart, map[byte]*int{
		'H': &duration.Hours, 'M': &duration.Minutes, 'S': &duration.Seconds,
	}, &seen); err != nil {
		return DateDuration{}, fmt.Errorf("iso duration %q: %w", raw, err)
	}
	if weeks != 0 {
		// ISO 8601: the week form is exclusive — "P2W" never combines with
		// other components. Weeks normalize to days internally.
		if duration.Years != 0 || duration.Months != 0 || duration.Days != 0 || timePart != "" {
			return DateDuration{}, fmt.Errorf("iso duration %q: weeks combine with no other component", raw)
		}
		duration.Days = weeks * 7
	}
	if !seen {
		return DateDuration{}, fmt.Errorf("iso duration %q: no components", raw)
	}
	// A zero duration cannot reach here: parseComponents rejects zero-valued
	// components, so any seen component is non-zero.
	if negative {
		duration = DateDuration{
			Years: -duration.Years, Months: -duration.Months, Days: -duration.Days,
			Hours: -duration.Hours, Minutes: -duration.Minutes, Seconds: -duration.Seconds,
		}
	}
	return duration, nil
}

// parseComponents scans "<int><designator>" pairs into the target map.
func parseComponents(part string, targets map[byte]*int, seen *bool) error {
	for len(part) > 0 {
		digits := 0
		for digits < len(part) && part[digits] >= '0' && part[digits] <= '9' {
			digits++
		}
		if digits == 0 {
			return fmt.Errorf("expected digits, found %q", part)
		}
		if digits == len(part) {
			return fmt.Errorf("number %q with no designator", part)
		}
		value, err := strconv.Atoi(part[:digits])
		if err != nil {
			return err
		}
		designator := part[digits]
		part = part[digits+1:]

		target, ok := targets[designator]
		if !ok {
			return fmt.Errorf("unknown designator %q", string(designator))
		}
		if *target != 0 {
			return fmt.Errorf("designator %q repeated", string(designator))
		}
		if value == 0 {
			return fmt.Errorf("designator %q has zero value", string(designator))
		}
		*target = value
		*seen = true
	}
	return nil
}

// FormatISODuration renders a DateDuration as a canonical ISO 8601 duration
// string (no weeks; leading '-' when the duration looks backward). The zero
// duration has no canonical form — callers must not format one.
func FormatISODuration(duration DateDuration) string {
	negative := duration.Years < 0 || duration.Months < 0 || duration.Days < 0 ||
		duration.Hours < 0 || duration.Minutes < 0 || duration.Seconds < 0

	abs := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}

	var out strings.Builder
	if negative {
		out.WriteByte('-')
	}
	out.WriteByte('P')
	writeComponent(&out, abs(duration.Years), 'Y')
	writeComponent(&out, abs(duration.Months), 'M')
	writeComponent(&out, abs(duration.Days), 'D')
	if duration.Hours != 0 || duration.Minutes != 0 || duration.Seconds != 0 {
		out.WriteByte('T')
		writeComponent(&out, abs(duration.Hours), 'H')
		writeComponent(&out, abs(duration.Minutes), 'M')
		writeComponent(&out, abs(duration.Seconds), 'S')
	}
	return out.String()
}

func writeComponent(out *strings.Builder, value int, designator byte) {
	if value == 0 {
		return
	}
	out.WriteString(strconv.Itoa(value))
	out.WriteByte(designator)
}
