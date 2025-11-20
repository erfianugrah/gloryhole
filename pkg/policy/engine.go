package policy

import (
	"github.com/expr-lang/expr/vm"
)

// Engine is the policy engine
type Engine struct {
	rules []*Rule
}

// Rule is a policy rule
type Rule struct {
	Name       string
	Logic      string
	Action     string
	ActionData string
	program    *vm.Program
}

// NewEngine creates a new policy engine
func NewEngine() *Engine {
	return &Engine{}
}

// AddRule adds a rule to the engine
func (e *Engine) AddRule(rule *Rule) error {
	// Logic to compile and add a rule will go here.
	e.rules = append(e.rules, rule)
	return nil
}

// Evaluate evaluates the request context against the rules
func (e *Engine) Evaluate(ctx map[string]interface{}) (bool, *Rule) {
	// Logic to evaluate the context against the rules will go here.
	return false, nil
}
