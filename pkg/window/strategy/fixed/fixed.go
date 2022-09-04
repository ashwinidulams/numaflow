// Package fixed implements Fixed windows. Fixed windows (sometimes called tumbling windows) are
// defined by a static window size, e.g. minutely windows or hourly windows. They are generally aligned, i.e. every
// window applies across all the data for the corresponding period of time.
package fixed

import (
	"time"

	"github.com/numaproj/numaflow/pkg/window"
)

// Fixed implements Fixed window.
type Fixed struct {
	// Length is the temporal length of the window.
	Length time.Duration
}

var _ window.Windower = (*Fixed)(nil)

// NewFixed returns a Fixed window.
func NewFixed(length time.Duration) *Fixed {
	return &Fixed{
		Length: length,
	}
}

// AssignWindow assigns a window for the given eventTime.
func (f *Fixed) AssignWindow(eventTime time.Time) []*window.IntervalWindow {
	start := eventTime.Truncate(f.Length)

	return []*window.IntervalWindow{
		{
			Start: start,
			End:   start.Add(f.Length),
		},
	}
}
