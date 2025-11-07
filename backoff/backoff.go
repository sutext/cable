package backoff

import (
	"math"
	"math/rand/v2"
	"time"
)

type Backoff interface {
	Next(count int) time.Duration
}

func Default() Backoff {
	return ExponentialD()
}
func Linear(base, step time.Duration) Backoff {
	return linearBackoff{base: base, step: step}
}
func LinearD() Backoff {
	return Linear(time.Second, time.Second*5)
}
func Random(min, max time.Duration) Backoff {
	return randomBackoff{min: min, max: max}
}
func RandomD() Backoff {
	return Random(time.Second*2, time.Second*10)
}
func Exponential(base time.Duration, exp float64) Backoff {
	return exponentialBackoff{base: base, exponent: exp}
}
func ExponentialD() Backoff {
	return Exponential(time.Second, 2)
}
func Constant(dur time.Duration) Backoff {
	return constantBackoff{duration: dur}
}
func ConstantD() Backoff {
	return Constant(time.Second * 5)
}

type constantBackoff struct {
	duration time.Duration
}

func (b constantBackoff) Next(count int) time.Duration {
	return b.duration
}

type exponentialBackoff struct {
	base     time.Duration
	exponent float64
}

func (b exponentialBackoff) Next(count int) time.Duration {
	return time.Duration(float64(b.base) * math.Pow(b.exponent, float64(count)))
}

type linearBackoff struct {
	base time.Duration
	step time.Duration
}

func (b linearBackoff) Next(count int) time.Duration {
	return b.base + time.Duration(count)*b.step
}

type randomBackoff struct {
	min time.Duration
	max time.Duration
}

func (b randomBackoff) Next(count int) time.Duration {
	return time.Duration(float64(b.min) + float64(b.max-b.min)*rand.Float64())
}
