package main

import (
	"time"

	"sutext.github.io/cable/internal/discovery"
)

func main() {
	d, err := discovery.New("broker")
	go func() {
		for {
			id, err := d.Query()
			if err != nil {
				time.Sleep(2 * time.Second)
				println("Error querying:", err.Error())
				continue
			}
			println("Found broker:", id)
			time.Sleep(2 * time.Second)
		}
	}()
	if err != nil {
		println("Error starting discovery:", err.Error())
	}
	d.Start()
}
