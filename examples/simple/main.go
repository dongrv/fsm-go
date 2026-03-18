package main

import (
	"fmt"
	"time"

	"github.com/dongrv/fsm-go"
)

type Door struct {
	fsm *fsm.Machine[string, string]
}

func main() {
	fmt.Println("=== Simple Door Example ===")

	// Create a door state machine
	door := &Door{}

	machine := fsm.NewBuilder[string, string]().
		AddState("closed").
		AddState("opened").
		AddTransition("closed", "opened", "open").
		AddTransition("opened", "closed", "close").
		Build()

	door.fsm = machine

	// Add a listener to monitor state changes
	listener := &fsm.SimpleListener[string, string]{
		OnTransitionFunc: func(ctx fsm.Context[string, string]) {
			fmt.Printf("Transition: %s -> %s (event: %s)\n",
				ctx.From(), ctx.To(), ctx.Event())
		},
		OnEntryFunc: func(state string, event string) {
			fmt.Printf("Entered state: %s\n", state)
		},
		OnExitFunc: func(state string, event string) {
			fmt.Printf("Exited state: %s\n", state)
		},
	}

	door.fsm.AddListener(listener)

	// Start the state machine
	if err := door.fsm.Start("closed"); err != nil {
		panic(err)
	}

	fmt.Printf("Initial state: %s\n", door.fsm.Current())

	// Send events
	fmt.Println("\n--- Sending events ---")

	events := []string{"open", "close", "open", "close"}
	for _, event := range events {
		fmt.Printf("\nSending event: %s\n", event)
		if err := door.fsm.Send(event); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
		fmt.Printf("Current state: %s\n", door.fsm.Current())
		time.Sleep(500 * time.Millisecond)
	}

	// Test invalid event
	fmt.Println("\n--- Testing invalid event ---")
	if err := door.fsm.Send("invalid"); err != nil {
		fmt.Printf("Expected error: %v\n", err)
	}

	// Check if event can be processed
	fmt.Println("\n--- Checking event validity ---")
	fmt.Printf("Can 'open' from '%s'? %v\n", door.fsm.Current(), door.fsm.Can("open"))
	fmt.Printf("Can 'close' from '%s'? %v\n", door.fsm.Current(), door.fsm.Can("close"))

	// Stop the state machine
	if err := door.fsm.Stop(); err != nil {
		fmt.Printf("Error stopping: %v\n", err)
	}

	fmt.Println("\n=== Example completed ===")
}
