// Package main provides the event router service
//
// The event router polls the event_queue table, collapses events using go-workflows,
// expands events to affected resources, and publishes reconciliation requests to Pub/Sub.
package main

import (
	"fmt"
	"os"

	"github.com/libops/control-plane/internal/eventrouter"
)

func main() {
	if err := eventrouter.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Event router error: %v\n", err)
		os.Exit(1)
	}
}
