// Package backoff provides various backoff strategies for retry mechanisms.
// It supports constant, linear, exponential, and random backoff algorithms.
package backoff

import (
	"math"
	"math/rand/v2"
	"time"
)

// Backoff defines the interface for backoff strategies.
// The Next method returns the duration to wait before the next retry attempt.
type Backoff interface {
	// Next returns the duration to wait before the next retry attempt.
	//
	// Parameters:
	// - count: The number of retry attempts made so far
	//
	// Returns:
	// - time.Duration: The duration to wait before the next retry
	Next(count int64) time.Duration
}

// Default returns the default backoff strategy, which is exponential backoff.
//
// Returns:
// - Backoff: Default exponential backoff strategy
func Default() Backoff {
	return ExponentialD()
}

// Linear creates a linear backoff strategy that increases linearly with each retry.
//
// Parameters:
// - base: Initial backoff duration
// - step: Duration added to the backoff for each retry
//
// Returns:
// - Backoff: Linear backoff strategy
func Linear(base, step time.Duration) Backoff {
	return linearBackoff{base: base, step: step}
}

// LinearD returns the default linear backoff strategy.
// Default values: base = 1 second, step = 5 seconds
//
// Returns:
// - Backoff: Default linear backoff strategy
func LinearD() Backoff {
	return Linear(time.Second, time.Second*5)
}

// Random creates a random backoff strategy that returns a random duration between min and max.
//
// Parameters:
// - min: Minimum backoff duration
// - max: Maximum backoff duration
//
// Returns:
// - Backoff: Random backoff strategy
func Random(min, max time.Duration) Backoff {
	return randomBackoff{min: min, max: max}
}

// RandomD returns the default random backoff strategy.
// Default values: min = 2 seconds, max = 5 seconds
//
// Returns:
// - Backoff: Default random backoff strategy
func RandomD() Backoff {
	return Random(time.Second*2, time.Second*5)
}

// Exponential creates an exponential backoff strategy that increases exponentially with each retry.
//
// Parameters:
// - base: Initial backoff duration
// - exp: Exponential factor
//
// Returns:
// - Backoff: Exponential backoff strategy
func Exponential(base time.Duration, exp float64) Backoff {
	return exponentialBackoff{base: base, exponent: exp}
}

// ExponentialD returns the default exponential backoff strategy.
// Default values: base = 1 second, exponent = 2
//
// Returns:
// - Backoff: Default exponential backoff strategy
func ExponentialD() Backoff {
	return Exponential(time.Second, 2)
}

// Constant creates a constant backoff strategy that returns the same duration for each retry.
//
// Parameters:
// - dur: Constant backoff duration
//
// Returns:
// - Backoff: Constant backoff strategy
func Constant(dur time.Duration) Backoff {
	return constantBackoff{duration: dur}
}

// ConstantD returns the default constant backoff strategy.
// Default value: 3 seconds
//
// Returns:
// - Backoff: Default constant backoff strategy
func ConstantD() Backoff {
	return Constant(time.Second * 3)
}

// constantBackoff implements the Backoff interface with a constant duration.
type constantBackoff struct {
	duration time.Duration // Constant backoff duration
}

// Next returns the constant backoff duration.
//
// Parameters:
// - count: The number of retry attempts made so far (ignored for constant backoff)
//
// Returns:
// - time.Duration: The constant backoff duration
func (b constantBackoff) Next(count int64) time.Duration {
	return b.duration
}

// exponentialBackoff implements the Backoff interface with exponential growth.
type exponentialBackoff struct {
	base     time.Duration // Initial backoff duration
	exponent float64       // Exponential growth factor
}

// Next returns the exponential backoff duration.
// Formula: base * (exponent ^ count)
//
// Parameters:
// - count: The number of retry attempts made so far
//
// Returns:
// - time.Duration: Exponentially increasing backoff duration
func (b exponentialBackoff) Next(count int64) time.Duration {
	return time.Duration(float64(b.base) * math.Pow(b.exponent, float64(count)))
}

// linearBackoff implements the Backoff interface with linear growth.
type linearBackoff struct {
	base time.Duration // Initial backoff duration
	step time.Duration // Step added for each retry
}

// Next returns the linear backoff duration.
// Formula: base + (step * count)
//
// Parameters:
// - count: The number of retry attempts made so far
//
// Returns:
// - time.Duration: Linearly increasing backoff duration
func (b linearBackoff) Next(count int64) time.Duration {
	return b.base + time.Duration(count)*b.step
}

// randomBackoff implements the Backoff interface with random duration.
type randomBackoff struct {
	min time.Duration // Minimum backoff duration
	max time.Duration // Maximum backoff duration
}

// Next returns a random backoff duration between min and max.
//
// Parameters:
// - count: The number of retry attempts made so far (ignored for random backoff)
//
// Returns:
// - time.Duration: Random backoff duration between min and max
func (b randomBackoff) Next(count int64) time.Duration {
	return time.Duration(float64(b.min) + float64(b.max-b.min)*rand.Float64())
}
