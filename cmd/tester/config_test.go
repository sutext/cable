package main

import "testing"

func TestReadConfig(t *testing.T) {
	_, err := readConfig("config.json")
	if err != nil {
		t.Fatal(err)
	}
}
