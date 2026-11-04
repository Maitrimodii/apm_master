package metrics

// StandardizeUnit standardises various unit names to a canonical set of units
func StandardizeUnit(u string) string {
	switch u {
	case "s", "sec", "second", "seconds":
		return "seconds"
	case "ms", "millis", "millisecond", "milliseconds":
		return "milliseconds"
	case "us", "micro", "microsecond", "microseconds":
		return "microseconds"
	case "ns", "nano", "nanosecond", "nanoseconds":
		return "nanoseconds"
	case "b", "byte", "bytes":
		return "bytes"
	case "kb", "kilobyte", "kilobytes":
		return "kilobytes"
	case "mb", "megabyte", "megabytes":
		return "megabytes"
	case "pct", "percent", "%":
		return "percent"
	case "count", "counter", "1", "":
		return "count"
	default:
		return u
	}
}
