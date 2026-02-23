package feature

// Flags provides feature flag evaluation.
type Flags struct {
	flags map[string]bool
}

// New creates a Flags instance from the config map.
func New(flags map[string]bool) *Flags {
	if flags == nil {
		flags = make(map[string]bool)
	}
	return &Flags{flags: flags}
}

// Flag names.
const (
	RoundTrips  = "round_trips"
	Autopacks   = "autopacks"
	Multiseller = "multiseller"
	AfterFilter = "afterfilter"
	Discounts   = "discounts"
)

// Enabled returns whether a feature flag is enabled.
func (f *Flags) Enabled(name string) bool {
	return f.flags[name]
}
