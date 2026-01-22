package main

import "testing"

func TestLoadConfig(t *testing.T) {
	// Load the config file
	_, err := readConfig("config.yaml")
	if err != nil {
		t.Error(err)
	}
}
