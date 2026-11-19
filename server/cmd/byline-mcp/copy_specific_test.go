package main

// extraTestEnv returns any extra environment entries this copy's own
// subprocess tests need set, beyond BYLINE_HOME and HOME, so a leftover
// value on the machine running the test can't leak in. withTelemetryOff
// also turns off this copy's own usage tracking, if it has any.
func extraTestEnv(withTelemetryOff bool) []string {
	return nil
}
