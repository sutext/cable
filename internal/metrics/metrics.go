package metrics

import "sutext.github.io/cable/internal/safe"

var store safe.Map[string, Meter]

func GetMeter(key string) Meter {
	m, _ := store.GetOrSet(key, NewMeter())
	return m
}
