// Package recovery provides panic recovery mechanisms for the agent
// [TD-002] Phase 0 - Panic Recovery implementation
package recovery

import (
	"fmt"
	"log"
	"runtime/debug"
)

// RecoveryHandler is a function that handles recovered panics
type RecoveryHandler func(component string, err interface{}, stack []byte)

// defaultHandler logs the panic with stack trace
var defaultHandler RecoveryHandler = func(component string, err interface{}, stack []byte) {
	log.Printf("[CRITICAL] [%s] panic_recovered error=%q", component, err)
	log.Printf("[CRITICAL] [%s] stack_trace=%s", component, string(stack))
}

// SetHandler sets a custom recovery handler
func SetHandler(handler RecoveryHandler) {
	defaultHandler = handler
}

// Recover wraps a function with panic recovery
// Usage: defer recovery.Recover("component_name")
func Recover(component string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		defaultHandler(component, r, stack)
	}
}

// RecoverWithCallback wraps a function with panic recovery and custom callback
func RecoverWithCallback(component string, callback func(interface{}, []byte)) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		defaultHandler(component, r, stack)
		if callback != nil {
			callback(r, stack)
		}
	}
}

// SafeCall executes a function with panic recovery, returns error on panic
func SafeCall(component string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			defaultHandler(component, r, stack)
			err = fmt.Errorf("panic in %s: %v", component, r)
		}
	}()
	return fn()
}

// SafeCallVoid executes a void function with panic recovery
func SafeCallVoid(component string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			defaultHandler(component, r, stack)
		}
	}()
	fn()
}
