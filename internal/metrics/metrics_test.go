package metrics

import "testing"

func TestMetrics(t *testing.T) {
	// TODO: Implement test

	// m := GetMeter("test")
	// for i := 0; i < 10; i++ {
	// 	m.Mark(1)
	// }
	for range 100 {
		go func() {
			t.Log(GetMeter("test").Count())
		}()
	}
	select {}
}
