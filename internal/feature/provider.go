package feature

// Provider defines the interface for feature flag evaluation.
// Implementations can use GrowthBook, LaunchDarkly, or static config.
type Provider interface {
	// Enabled returns whether a feature flag is enabled.
	Enabled(name string) bool
}

// Ensure Flags implements Provider.
var _ Provider = (*Flags)(nil)

// StaticProvider returns a provider backed by a static map.
func StaticProvider(flags map[string]bool) Provider {
	return New(flags)
}
