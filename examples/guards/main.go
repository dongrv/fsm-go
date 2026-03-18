package main

import (
	"fmt"
	"time"

	"github.com/dongrv/fsm-go"
)

type VendingMachine struct {
	fsm       *fsm.Machine[string, string]
	balance   int
	itemPrice int
	itemStock int
}

func main() {
	fmt.Println("=== Vending Machine with Guards Example ===")

	vm := &VendingMachine{
		balance:   0,
		itemPrice: 150, // $1.50 in cents
		itemStock: 5,
	}

	// Create state machine with guards
	machine := fsm.NewExtendedBuilder[string, string]().
		AddState("idle").
		AddState("collecting").
		AddState("dispensing").
		AddState("out_of_stock", fsm.AsFinal[string, string]()).
		AddState("maintenance", fsm.AsFinal[string, string]()).
		Build()

	// Add transitions with guards and actions
	// From idle to collecting when coin is inserted
	machine.AddTransition("idle", "collecting", "insert_coin",
		fsm.WithGuard(func(ctx fsm.Context[string, string]) bool {
			return vm.itemStock > 0
		}),
		fsm.WithAction(func(ctx fsm.Context[string, string]) {
			amount := ctx.Payload().(int)
			vm.balance += amount
			fmt.Printf("Coin inserted: %d cents. Balance: %d cents\n", amount, vm.balance)
		}),
	)

	// Stay in collecting when more coins are inserted
	machine.AddInternalTransition("collecting", "insert_coin",
		fsm.WithAction(func(ctx fsm.Context[string, string]) {
			amount := ctx.Payload().(int)
			vm.balance += amount
			fmt.Printf("Coin inserted: %d cents. Balance: %d cents\n", amount, vm.balance)
		}),
	)

	// From collecting to dispensing when enough money
	machine.AddTransition("collecting", "dispensing", "select_item",
		fsm.WithGuard(func(ctx fsm.Context[string, string]) bool {
			return vm.balance >= vm.itemPrice
		}),
		fsm.WithAction(func(ctx fsm.Context[string, string]) {
			fmt.Println("Item selected. Dispensing...")
			vm.itemStock--
			vm.balance -= vm.itemPrice
		}),
	)

	// From dispensing back to idle
	machine.AddTransition("dispensing", "idle", "dispense_complete",
		fsm.WithAction(func(ctx fsm.Context[string, string]) {
			fmt.Println("Item dispensed. Returning change if any.")
			if vm.balance > 0 {
				fmt.Printf("Returning change: %d cents\n", vm.balance)
				vm.balance = 0
			}
		}),
	)

	// From collecting back to idle (cancel)
	machine.AddTransition("collecting", "idle", "cancel",
		fsm.WithAction(func(ctx fsm.Context[string, string]) {
			fmt.Printf("Transaction cancelled. Returning %d cents\n", vm.balance)
			vm.balance = 0
		}),
	)

	// From idle to out_of_stock when stock is empty
	machine.AddTransition("idle", "out_of_stock", "check_stock",
		fsm.WithGuard(func(ctx fsm.Context[string, string]) bool {
			return vm.itemStock == 0
		}),
		fsm.WithAction(func(ctx fsm.Context[string, string]) {
			fmt.Println("Out of stock!")
		}),
	)

	// Maintenance mode
	machine.AddTransition("idle", "maintenance", "maintenance_mode",
		fsm.WithGuard(func(ctx fsm.Context[string, string]) bool {
			// Only allow maintenance with admin key
			key, ok := ctx.Payload().(string)
			return ok && key == "admin123"
		}),
		fsm.WithAction(func(ctx fsm.Context[string, string]) {
			fmt.Println("Entering maintenance mode")
		}),
	)

	// Add listener
	listener := &fsm.SimpleListener[string, string]{
		OnTransitionFunc: func(ctx fsm.Context[string, string]) {
			fmt.Printf("State: %s -> %s\n", ctx.From(), ctx.To())
		},
		OnEventFunc: func(event string, payload any) {
			fmt.Printf("Event: %s", event)
			if payload != nil {
				fmt.Printf(" (payload: %v)", payload)
			}
			fmt.Println()
		},
		OnErrorFunc: func(err error) {
			fmt.Printf("Error: %v\n", err)
		},
	}

	machine.AddListener(listener)

	// Start the state machine
	if err := machine.Start("idle"); err != nil {
		panic(err)
	}

	fmt.Printf("Initial state: %s, Stock: %d, Price: %d cents\n",
		machine.Current(), vm.itemStock, vm.itemPrice)

	// Simulate vending machine operation
	fmt.Println("\n--- Normal operation ---")

	// Insert coins
	coins := []int{25, 25, 50, 25, 25} // Total: 150 cents
	for i, coin := range coins {
		fmt.Printf("\n[Step %d] Inserting %d cent coin\n", i+1, coin)
		if err := machine.Send("insert_coin", coin); err != nil {
			fmt.Printf("Failed: %v\n", err)
			break
		}
		fmt.Printf("Balance: %d cents, State: %s\n", vm.balance, machine.Current())
		time.Sleep(500 * time.Millisecond)
	}

	// Select item
	fmt.Println("\nSelecting item...")
	if err := machine.Send("select_item"); err != nil {
		fmt.Printf("Failed: %v\n", err)
	} else {
		fmt.Printf("Balance after purchase: %d cents\n", vm.balance)
	}

	// Complete dispensing
	time.Sleep(1 * time.Second)
	machine.Send("dispense_complete")
	fmt.Printf("Stock remaining: %d\n", vm.itemStock)

	// Test insufficient funds
	fmt.Println("\n--- Testing insufficient funds ---")
	machine.Send("insert_coin", 50)
	fmt.Printf("Balance: %d cents\n", vm.balance)
	if err := machine.Send("select_item"); err != nil {
		fmt.Printf("Expected error: %v\n", err)
	}

	// Cancel transaction
	fmt.Println("\nCancelling transaction...")
	machine.Send("cancel")
	fmt.Printf("Balance after cancel: %d cents\n", vm.balance)

	// Test maintenance mode
	fmt.Println("\n--- Testing maintenance mode ---")
	fmt.Println("Trying maintenance with wrong key...")
	if err := machine.Send("maintenance_mode", "wrongkey"); err != nil {
		fmt.Printf("Expected error: %v\n", err)
	}

	fmt.Println("Trying maintenance with correct key...")
	if err := machine.Send("maintenance_mode", "admin123"); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Successfully entered maintenance mode")
	}

	// Simulate running out of stock
	fmt.Println("\n--- Simulating out of stock ---")
	// Buy remaining items
	for i := 0; i < 4; i++ {
		machine.Start("idle")
		// Quick purchase
		machine.Send("insert_coin", 150)
		machine.Send("select_item")
		machine.Send("dispense_complete")
		fmt.Printf("Purchase %d complete. Stock: %d\n", i+1, vm.itemStock)
	}

	// Check stock
	fmt.Println("\nChecking stock...")
	if err := machine.Send("check_stock"); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Printf("Final state: %s\n", machine.Current())

	// Stop the state machine
	machine.Stop()

	fmt.Println("\n=== Example completed ===")
}
