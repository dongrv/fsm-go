package fsm

import (
	"encoding/json"
	"fmt"
	"time"
)

// StateSnapshot represents a snapshot of the state machine's state.
type StateSnapshot[S comparable, E comparable] struct {
	CurrentState S              `json:"current_state"`
	History      map[S]S        `json:"history,omitempty"`
	StateData    map[string]any `json:"state_data,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// TakeSnapshot creates a snapshot of the current state.
func (m *Machine[S, E]) TakeSnapshot() (*StateSnapshot[S, E], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Convert state data to serializable format
	stateData := make(map[string]any)
	for state, data := range m.stateData {
		// Try to marshal the data
		if jsonData, err := json.Marshal(data); err == nil {
			stateData[fmt.Sprintf("%v", state)] = json.RawMessage(jsonData)
		} else {
			stateData[fmt.Sprintf("%v", state)] = data
		}
	}

	return &StateSnapshot[S, E]{
		CurrentState: m.current,
		History:      m.history,
		StateData:    stateData,
		Timestamp:    time.Now(),
		Metadata: map[string]any{
			"async":        m.async,
			"initialized":  m.initialized,
			"states_count": len(m.states),
		},
	}, nil
}

// RestoreFromSnapshot restores the state machine from a snapshot.
func (m *Machine[S, E]) RestoreFromSnapshot(snapshot *StateSnapshot[S, E]) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate the state exists
	if _, exists := m.states[snapshot.CurrentState]; !exists {
		return fmt.Errorf("state %v does not exist", snapshot.CurrentState)
	}

	// Restore state
	m.current = snapshot.CurrentState
	m.history = snapshot.History
	m.initialized = true

	// Restore state data
	m.stateData = make(map[S]any)
	if snapshot.StateData != nil {
		for key, data := range snapshot.StateData {
			// Try to unmarshal JSON data
			if jsonData, ok := data.(json.RawMessage); ok {
				var decoded any
				if err := json.Unmarshal(jsonData, &decoded); err == nil {
					// We need to convert string key back to state type
					// Since we can't directly convert string to type parameter S,
					// we need to find the matching state
					if m.states != nil {
						for state := range m.states {
							if fmt.Sprintf("%v", state) == key {
								m.stateData[state] = decoded
								break
							}
						}
					}
				}
			} else {
				// Find matching state for the key
				if m.states != nil {
					for state := range m.states {
						if fmt.Sprintf("%v", state) == key {
							m.stateData[state] = data
							break
						}
					}
				}
			}
		}
	}

	m.logger.Infof("State restored from snapshot: %v", snapshot.CurrentState)
	return nil
}

// Validator provides validation for state machines.
type Validator[S comparable, E comparable] struct {
	machine *Machine[S, E]
}

// NewValidator creates a new validator for a state machine.
func NewValidator[S comparable, E comparable](machine *Machine[S, E]) *Validator[S, E] {
	return &Validator[S, E]{machine: machine}
}

// Validate performs comprehensive validation of the state machine.
func (v *Validator[S, E]) Validate() []error {
	var errors []error

	v.machine.mu.RLock()
	defer v.machine.mu.RUnlock()

	// Check for duplicate states
	stateCount := make(map[S]int)
	for state := range v.machine.states {
		stateCount[state]++
		if stateCount[state] > 1 {
			errors = append(errors, fmt.Errorf("duplicate state: %v", state))
		}
	}

	// Check for unreachable states
	reachable := make(map[S]bool)
	reachable[v.machine.current] = true

	// Perform BFS to find reachable states
	queue := []S{v.machine.current}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, transitions := range v.machine.transitions[current] {
			for _, t := range transitions {
				if !reachable[t.to] {
					reachable[t.to] = true
					queue = append(queue, t.to)
				}
			}
		}
	}

	for state := range v.machine.states {
		if !reachable[state] {
			errors = append(errors, fmt.Errorf("unreachable state: %v", state))
		}
	}

	// Check for dead-end states (no outgoing transitions except to self)
	for state, config := range v.machine.states {
		if config.final {
			continue // Final states are allowed to have no outgoing transitions
		}

		hasOutgoing := false
		for _, transitions := range v.machine.transitions[state] {
			for _, t := range transitions {
				if t.to != state || !t.internal {
					hasOutgoing = true
					break
				}
			}
			if hasOutgoing {
				break
			}
		}

		if !hasOutgoing {
			errors = append(errors, fmt.Errorf("dead-end state with no outgoing transitions: %v", state))
		}
	}

	// Check for conflicting transitions
	for state, eventTransitions := range v.machine.transitions {
		for event, transitions := range eventTransitions {
			// Check for multiple transitions with same from/event but different guards
			// that could be ambiguous
			if len(transitions) > 1 {
				// Check if any two transitions have mutually exclusive guards
				// This is a simplified check
				hasDefault := false
				for _, t := range transitions {
					if t.guard == nil {
						hasDefault = true
					}
				}

				if hasDefault && len(transitions) > 1 {
					errors = append(errors, fmt.Errorf("ambiguous transitions from state %v on event %v: multiple transitions including default", state, event))
				}
			}
		}
	}

	return errors
}

// EventQueueStats provides statistics about the event queue.
type EventQueueStats struct {
	CurrentSize int     `json:"current_size"`
	MaxSize     int     `json:"max_size"`
	Throughput  float64 `json:"throughput"` // events per second
	Dropped     int     `json:"dropped"`    // dropped events due to full queue
}

// GetQueueStats returns statistics about the event queue.
func (m *Machine[S, E]) GetQueueStats() EventQueueStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return EventQueueStats{
		CurrentSize: len(m.eventQueue),
		MaxSize:     m.maxQueueSize,
		// Note: Throughput and Dropped would need to be tracked separately
	}
}

// TimeoutManager manages timeouts for states.
type TimeoutManager[S comparable, E comparable] struct {
	machine *Machine[S, E]
	timers  map[S]*time.Timer
}

// NewTimeoutManager creates a new timeout manager.
func NewTimeoutManager[S comparable, E comparable](machine *Machine[S, E]) *TimeoutManager[S, E] {
	return &TimeoutManager[S, E]{
		machine: machine,
		timers:  make(map[S]*time.Timer),
	}
}

// StartTimeout starts a timeout for a state.
func (tm *TimeoutManager[S, E]) StartTimeout(state S, timeout time.Duration, timeoutEvent E) {
	// Cancel existing timer if any
	tm.CancelTimeout(state)

	timer := time.AfterFunc(timeout, func() {
		tm.machine.Send(timeoutEvent)
	})

	tm.timers[state] = timer
}

// CancelTimeout cancels a timeout for a state.
func (tm *TimeoutManager[S, E]) CancelTimeout(state S) {
	if timer, exists := tm.timers[state]; exists {
		timer.Stop()
		delete(tm.timers, state)
	}
}

// CancelAll cancels all timeouts.
func (tm *TimeoutManager[S, E]) CancelAll() {
	for state, timer := range tm.timers {
		timer.Stop()
		delete(tm.timers, state)
	}
}

// Pattern provides common state machine patterns.
type Pattern[S comparable, E comparable] struct{}

// NewPattern creates a new pattern helper.
func NewPattern[S comparable, E comparable]() *Pattern[S, E] {
	return &Pattern[S, E]{}
}

// DoorPattern creates a door pattern (closed <-> opened).
func (p *Pattern[S, E]) DoorPattern(closed, opened S, open, close E) *Builder[S, E] {
	return NewBuilder[S, E]().
		AddState(closed).
		AddState(opened).
		AddTransition(closed, opened, open).
		AddTransition(opened, closed, close)
}

// TrafficLightPattern creates a traffic light pattern.
func (p *Pattern[S, E]) TrafficLightPattern(red, yellow, green S, next E) *Builder[S, E] {
	return NewBuilder[S, E]().
		AddState(red).
		AddState(green).
		AddState(yellow).
		AddTransition(red, green, next).
		AddTransition(green, yellow, next).
		AddTransition(yellow, red, next)
}

// RetryPattern creates a retry pattern with exponential backoff.
func (p *Pattern[S, E]) RetryPattern(idle, working, retrying, failed S, start, success, fail, retry E) *Machine[S, E] {
	return NewExtendedBuilder[S, E]().
		AddState(idle).
		AddState(working).
		AddState(retrying).
		AddState(failed, AsFinal[S, E]()).
		AddTransition(idle, working, start).
		AddTransition(working, idle, success).
		AddTransition(working, retrying, fail).
		AddTransition(retrying, working, retry).
		AddTransition(retrying, failed, fail).
		Build()
}

// CircuitBreakerPattern creates a circuit breaker pattern.
func (p *Pattern[S, E]) CircuitBreakerPattern(closed, open, halfOpen S, success, failure, timeout, reset E) *Machine[S, E] {
	builder := NewExtendedBuilder[S, E]()

	// Closed state: normal operation
	builder.AddStateWithBuilder(closed).
		AddTransition(open, failure).
		End()

	// Open state: circuit is open
	builder.AddStateWithBuilder(open).
		WithTimeout(30*time.Second, timeout). // Timeout to half-open
		End()

	// Half-open state: testing if service is back
	builder.AddStateWithBuilder(halfOpen).
		AddTransition(closed, success).
		AddTransition(open, failure).
		End()

	// Transitions
	builder.AddTransition(open, halfOpen, timeout).
		AddTransition(halfOpen, closed, success).
		AddTransition(halfOpen, open, failure).
		AddTransition(closed, open, failure).
		AddTransition(open, closed, reset) // Manual reset

	return builder.Build()
}
