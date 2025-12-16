package metrics

import "sutext.github.io/cable/internal/safe"

var store safe.Map[string, Meter]

func GetMeter(key string) Meter {
	if m, ok := store.Get(key); ok {
		return m
	}
	m := NewMeter()
	store.Set(key, m)
	return m
}
