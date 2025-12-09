package metrics

import (
	"time"
)

type Logger interface {
	Printf(format string, v ...interface{})
}

// Log outputs each metric in the given registry periodically using the given logger.
func Log(r Registry, freq time.Duration, l Logger) {
	LogScaled(r, freq, time.Nanosecond, l)
}

// LogOnCue outputs each metric in the given registry on demand through the channel
// using the given logger
func LogOnCue(r Registry, ch chan interface{}, l Logger) {
	LogScaledOnCue(r, ch, time.Nanosecond, l)
}

// LogScaled outputs each metric in the given registry periodically using the given
// logger. Print timings in `scale` units (eg time.Millisecond) rather than nanos.
func LogScaled(r Registry, freq time.Duration, scale time.Duration, l Logger) {
	ch := make(chan interface{})
	go func(channel chan interface{}) {
		for _ = range time.Tick(freq) {
			channel <- struct{}{}
		}
	}(ch)
	LogScaledOnCue(r, ch, scale, l)
}

// LogScaledOnCue outputs each metric in the given registry on demand through the channel
// using the given logger. Print timings in `scale` units (eg time.Millisecond) rather
// than nanos.
func LogScaledOnCue(r Registry, ch chan interface{}, scale time.Duration, l Logger) {
	for _ = range ch {
		r.Each(func(name string, i interface{}) {
			switch metric := i.(type) {
			case Counter:
				l.Printf("counter %s\n", name)
				l.Printf("  count:       %9d\n", metric.Count())
			case Meter:
				m := metric.Snapshot()
				l.Printf("meter %s\n", name)
				l.Printf("  count:       %9d\n", m.Count())
				l.Printf("  1-min rate:  %12.2f\n", m.Rate1())
				l.Printf("  5-min rate:  %12.2f\n", m.Rate5())
				l.Printf("  15-min rate: %12.2f\n", m.Rate15())
				l.Printf("  mean rate:   %12.2f\n", m.RateMean())
			}
		})
	}
}
