package main

import (
	"fmt"
	"time"

	"github.com/dongrv/fsm-go"
)

func main() {
	fmt.Println("=== Hierarchical State Machine Example ===")

	// Create a washing machine state machine with hierarchical states
	machine := fsm.NewExtendedBuilder[string, string]().
		AddState("off").
		AddCompositeState("on", func(b *fsm.CompositeBuilder[string, string]) {
			// Substates of "on"
			b.AddState("idle").
				WithEntry(func(ctx fsm.Context[string, string]) {
					fmt.Println("Washing machine is now idle")
				}).
				End()

			b.AddState("washing").
				WithEntry(func(ctx fsm.Context[string, string]) {
					fmt.Println("Starting washing cycle")
				}).
				WithExit(func(ctx fsm.Context[string, string]) {
					fmt.Println("Washing cycle completed")
				}).
				End()

			b.AddCompositeState("washing", func(b2 *fsm.CompositeBuilder[string, string]) {
				// Substates of "washing"
				b2.AddState("filling").
					WithEntry(func(ctx fsm.Context[string, string]) {
						fmt.Println("Filling water...")
					}).
					End()

				b2.AddState("washing").
					WithEntry(func(ctx fsm.Context[string, string]) {
						fmt.Println("Washing clothes...")
					}).
					End()

				b2.AddState("rinsing").
					WithEntry(func(ctx fsm.Context[string, string]) {
						fmt.Println("Rinsing clothes...")
					}).
					End()

				b2.AddState("spinning").
					WithEntry(func(ctx fsm.Context[string, string]) {
						fmt.Println("Spinning clothes...")
					}).
					End()

				// Set initial substate
				b2.WithInitial("filling")
			})

			b.AddState("drying").
				WithEntry(func(ctx fsm.Context[string, string]) {
					fmt.Println("Drying clothes...")
				}).
				End()

			// Set initial substate
			b.WithInitial("idle")
		}).
		AddState("error", fsm.AsFinal[string, string]()).
		Build()

	// Add transitions
	machine.AddTransition("off", "on", "power_on")
	machine.AddTransition("on", "off", "power_off")
	machine.AddTransition("on/idle", "on/washing", "start_wash")
	machine.AddTransition("on/washing", "on/drying", "start_dry")
	machine.AddTransition("on/drying", "on/idle", "complete")
	machine.AddTransition("on/washing/filling", "on/washing/washing", "water_full")
	machine.AddTransition("on/washing/washing", "on/washing/rinsing", "wash_complete")
	machine.AddTransition("on/washing/rinsing", "on/washing/spinning", "rinse_complete")
	machine.AddTransition("on/washing/spinning", "on/washing", "spin_complete")
	machine.AddTransition("on", "error", "error")

	// Add a listener
	listener := &fsm.SimpleListener[string, string]{
		OnTransitionFunc: func(ctx fsm.Context[string, string]) {
			fmt.Printf("TRANSITION: %s -> %s (event: %s)\n",
				ctx.From(), ctx.To(), ctx.Event())
		},
		OnEntryFunc: func(state string, event string) {
			fmt.Printf("ENTER: %s\n", state)
		},
		OnExitFunc: func(state string, event string) {
			fmt.Printf("EXIT: %s\n", state)
		},
	}

	machine.AddListener(listener)

	// Start the state machine
	if err := machine.Start("off"); err != nil {
		panic(err)
	}

	fmt.Printf("\nInitial state: %s\n", machine.Current())

	// Simulate washing machine operation
	fmt.Println("\n--- Operating washing machine ---")

	events := []struct {
		event string
		delay time.Duration
	}{
		{"power_on", 1 * time.Second},
		{"start_wash", 1 * time.Second},
		{"water_full", 2 * time.Second},
		{"wash_complete", 2 * time.Second},
		{"rinse_complete", 2 * time.Second},
		{"spin_complete", 2 * time.Second},
		{"start_dry", 1 * time.Second},
		{"complete", 1 * time.Second},
		{"power_off", 1 * time.Second},
	}

	for _, e := range events {
		fmt.Printf("\n>>> Sending event: %s\n", e.event)
		if err := machine.Send(e.event); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
		fmt.Printf("Current state: %s\n", machine.Current())
		time.Sleep(e.delay)
	}

	// Test error handling
	fmt.Println("\n--- Testing error handling ---")
	if err := machine.Start("off"); err != nil {
		fmt.Printf("Restarting: %v\n", err)
	}
	machine.Send("power_on")
	machine.Send("error")
	fmt.Printf("Final state: %s\n", machine.Current())

	// Stop the state machine
	machine.Stop()

	fmt.Println("\n=== Example completed ===")
}
