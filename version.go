package otelgenai

// Version is the released version of the otelgenai-go core module. It is
// used as the default instrumentation scope version for traces, metrics,
// and logs unless overridden with WithInstrumentationVersion.
//
// Keep this in sync with the tagged release version. The release workflow
// validates that a root release tag matches "v" + Version before tagging.
const Version = "0.6.0"
