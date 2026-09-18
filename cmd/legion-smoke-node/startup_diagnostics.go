package main

import (
	"log"
	"time"
)

// Stage names are fixed call-site constants. Never log arguments or raw errors:
// initialization and registration errors may contain credentials or URLs.
// A started stage without a completion identifies a blocked or aborted step.
func startStartupStage(enabled bool, stage string) func(error) {
	if !enabled {
		return func(error) {}
	}
	started := time.Now()
	log.Printf("component=legion-node event=startup stage=%s status=started", stage)
	return func(err error) {
		status := "completed"
		if err != nil {
			status = "failed"
		}
		log.Printf("component=legion-node event=startup stage=%s status=%s elapsed_ms=%.3f", stage, status, float64(time.Since(started))/float64(time.Millisecond))
	}
}
