package discovery

import "testing"

func TestDiscovery(t *testing.T) {
	// TODO: Implement test cases
	d, err := New("broker1")
	if err != nil {
		t.Error(err)
	}
	go func() {
		for {
			id, err := d.Query()
			if err != nil {
				t.Error(err)
			}
			t.Log("Received response from", id)
		}
	}()
	d.Start()
}
