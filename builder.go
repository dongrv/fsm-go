package fsm

import (
	"context"
	"time"
)

// StateBuilder provides a fluent API for building state configurations.
type StateBuilder[S comparable, E comparable] struct {
	machine *Machine[S, E]
	state   S
}

// NewStateBuilder creates a new state builder.
func NewStateBuilder[S comparable, E comparable](machine *Machine[S, E], state S) *StateBuilder[S, E] {
	return &StateBuilder[S, E]{
		machine: machine,
		state:   state,
	}
}

// WithEntry adds an entry action to the state.
func (b *StateBuilder[S, E]) WithEntry(action Action[S, E]) *StateBuilder[S, E] {
	b.machine.AddState(b.state, WithEntryAction(action))
	return b
}

// WithExit adds an exit action to the state.
func (b *StateBuilder[S, E]) WithExit(action Action[S, E]) *StateBuilder[S, E] {
	b.machine.AddState(b.state, WithExitAction(action))
	return b
}

// WithTimeout adds a timeout to the state.
func (b *StateBuilder[S, E]) WithTimeout(timeout time.Duration, timeoutEvent E) *StateBuilder[S, E] {
	b.machine.AddState(b.state, WithTimeout[S, E](timeout, timeoutEvent))
	return b
}

// AsFinal marks the state as final.
func (b *StateBuilder[S, E]) AsFinal() *StateBuilder[S, E] {
	b.machine.AddState(b.state, AsFinal[S, E]())
	return b
}

// AsParallel marks the state as parallel.
func (b *StateBuilder[S, E]) AsParallel() *StateBuilder[S, E] {
	b.machine.AddState(b.state, AsParallel[S, E]())
	return b
}

// WithHistory sets the history type for the state.
func (b *StateBuilder[S, E]) WithHistory(history HistoryType) *StateBuilder[S, E] {
	b.machine.AddState(b.state, WithHistory[S, E](history))
	return b
}

// AddTransition adds a transition from this state.
func (b *StateBuilder[S, E]) AddTransition(to S, event E, opts ...TransitionOption[S, E]) *StateBuilder[S, E] {
	b.machine.AddTransition(b.state, to, event, opts...)
	return b
}

// AddInternalTransition adds an internal transition for this state.
func (b *StateBuilder[S, E]) AddInternalTransition(event E, opts ...TransitionOption[S, E]) *StateBuilder[S, E] {
	b.machine.AddInternalTransition(b.state, event, opts...)
	return b
}

// End returns the parent builder.
func (b *StateBuilder[S, E]) End() *Builder[S, E] {
	return &Builder[S, E]{machine: b.machine}
}

// TransitionBuilder provides a fluent API for building transitions.
type TransitionBuilder[S comparable, E comparable] struct {
	machine *Machine[S, E]
	from    S
	to      S
	event   E
}

// NewTransitionBuilder creates a new transition builder.
func NewTransitionBuilder[S comparable, E comparable](machine *Machine[S, E], from, to S, event E) *TransitionBuilder[S, E] {
	return &TransitionBuilder[S, E]{
		machine: machine,
		from:    from,
		to:      to,
		event:   event,
	}
}

// WithGuard adds a guard to the transition.
func (b *TransitionBuilder[S, E]) WithGuard(guard Guard[S, E]) *TransitionBuilder[S, E] {
	b.machine.AddTransition(b.from, b.to, b.event, WithGuard(guard))
	return b
}

// WithAction adds an action to the transition.
func (b *TransitionBuilder[S, E]) WithAction(action Action[S, E]) *TransitionBuilder[S, E] {
	b.machine.AddTransition(b.from, b.to, b.event, WithAction(action))
	return b
}

// End returns the parent builder.
func (b *TransitionBuilder[S, E]) End() *Builder[S, E] {
	return &Builder[S, E]{machine: b.machine}
}

// CompositeBuilder provides a fluent API for building composite states.
type CompositeBuilder[S comparable, E comparable] struct {
	machine *Machine[S, E]
	parent  S
	builder *Builder[S, E]
}

// NewCompositeBuilder creates a new composite builder.
func NewCompositeBuilder[S comparable, E comparable](machine *Machine[S, E], parent S, builder *Builder[S, E]) *CompositeBuilder[S, E] {
	return &CompositeBuilder[S, E]{
		machine: machine,
		parent:  parent,
		builder: builder,
	}
}

// AddState adds a substate to the composite state.
func (b *CompositeBuilder[S, E]) AddState(state S, opts ...StateOption[S, E]) *StateBuilder[S, E] {
	// Mark as substate by setting parent
	opts = append(opts, func(c *StateConfig[S, E]) {
		c.parent = &b.parent
	})

	b.machine.AddState(state, opts...)
	return NewStateBuilder(b.machine, state)
}

// WithInitial sets the initial substate.
func (b *CompositeBuilder[S, E]) WithInitial(initial S) *CompositeBuilder[S, E] {
	b.machine.AddState(b.parent, WithInitialSubstate[S, E](initial))
	return b
}

// End returns to the parent builder.
func (b *CompositeBuilder[S, E]) End() *Builder[S, E] {
	return b.builder
}

// AddCompositeState adds a nested composite state.
func (b *CompositeBuilder[S, E]) AddCompositeState(state S, buildFunc func(*CompositeBuilder[S, E])) *CompositeBuilder[S, E] {
	// Add the substate with parent reference
	opts := []StateOption[S, E]{
		func(c *StateConfig[S, E]) {
			c.parent = &b.parent
		},
	}

	b.machine.AddState(state, opts...)

	// Create nested composite builder
	nestedBuilder := NewCompositeBuilder(b.machine, state, b.builder)

	// Build nested substates
	buildFunc(nestedBuilder)

	return b
}

// ExtendedBuilder provides extended builder methods.
type ExtendedBuilder[S comparable, E comparable] struct {
	*Builder[S, E]
}

// AddState adds a state with options and returns ExtendedBuilder for chaining.
func (b *ExtendedBuilder[S, E]) AddState(state S, opts ...StateOption[S, E]) *ExtendedBuilder[S, E] {
	b.Builder.AddState(state, opts...)
	return b
}

// AddTransition adds a transition with options and returns ExtendedBuilder for chaining.
func (b *ExtendedBuilder[S, E]) AddTransition(from, to S, event E, opts ...TransitionOption[S, E]) *ExtendedBuilder[S, E] {
	b.Builder.AddTransition(from, to, event, opts...)
	return b
}

// AddInternalTransition adds an internal transition and returns ExtendedBuilder for chaining.
func (b *ExtendedBuilder[S, E]) AddInternalTransition(state S, event E, opts ...TransitionOption[S, E]) *ExtendedBuilder[S, E] {
	b.Builder.AddInternalTransition(state, event, opts...)
	return b
}

// WithAsync enables async event processing.
func (b *ExtendedBuilder[S, E]) WithAsync() *ExtendedBuilder[S, E] {
	b.Builder.WithAsync(true)
	return b
}

// WithQueueSize sets the maximum event queue size.
func (b *ExtendedBuilder[S, E]) WithQueueSize(size int) *ExtendedBuilder[S, E] {
	b.Builder.WithQueueSize(size)
	return b
}

// WithLogger sets a custom logger.
func (b *ExtendedBuilder[S, E]) WithLogger(logger Logger) *ExtendedBuilder[S, E] {
	b.Builder.WithLogger(logger)
	return b
}

// WithDefaultTimeout sets the default timeout for states.
func (b *ExtendedBuilder[S, E]) WithDefaultTimeout(timeout time.Duration) *ExtendedBuilder[S, E] {
	b.machine.defaultTimeout = timeout
	return b
}

// WithContext sets a context for the state machine.
func (b *ExtendedBuilder[S, E]) WithContext(ctx context.Context) *ExtendedBuilder[S, E] {
	b.machine.context = ctx
	return b
}

// NewExtendedBuilder creates a new extended builder.
func NewExtendedBuilder[S comparable, E comparable]() *ExtendedBuilder[S, E] {
	return &ExtendedBuilder[S, E]{
		Builder: NewBuilder[S, E](),
	}
}

// AddCompositeState adds a composite state with substates.
func (b *ExtendedBuilder[S, E]) AddCompositeState(state S, buildFunc func(*CompositeBuilder[S, E])) *ExtendedBuilder[S, E] {
	// Add the parent state
	b.AddState(state)

	// Create composite builder
	compositeBuilder := NewCompositeBuilder(b.machine, state, b.Builder)

	// Build substates
	buildFunc(compositeBuilder)

	return b
}

// AddParallelState adds a parallel state with regions.
func (b *ExtendedBuilder[S, E]) AddParallelState(state S, buildFunc func(*CompositeBuilder[S, E])) *ExtendedBuilder[S, E] {
	// Add as parallel state
	b.AddState(state, AsParallel[S, E]())

	// Create composite builder
	compositeBuilder := NewCompositeBuilder(b.machine, state, b.Builder)

	// Build parallel regions
	buildFunc(compositeBuilder)

	return b
}

// AddStateWithBuilder adds a state and returns a state builder.
func (b *ExtendedBuilder[S, E]) AddStateWithBuilder(state S) *StateBuilder[S, E] {
	b.AddState(state)
	return NewStateBuilder(b.machine, state)
}

// AddTransitionWithBuilder adds a transition and returns a transition builder.
func (b *ExtendedBuilder[S, E]) AddTransitionWithBuilder(from, to S, event E) *TransitionBuilder[S, E] {
	b.AddTransition(from, to, event)
	return NewTransitionBuilder(b.machine, from, to, event)
}
