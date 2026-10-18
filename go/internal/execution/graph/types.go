// Package graph is the model of a script (ADR-0003): a directed acyclic graph of typed nodes; `Code` nodes may follow one another,
// everything else is built in and trusted. It knows nothing about the database or about runners: it describes the
// nodes, decodes a stored graph and checks it. Running it is the evaluator's job.
package graph

// PortType is the type of the value a port carries.
type PortType string

const (
	TInt    PortType = "int"
	TFloat  PortType = "float"
	TStr    PortType = "str"
	TBool   PortType = "bool"
	TDoc    PortType = "doc"     // any JSON, null included
	TAny    PortType = "any"     // an input that takes any token and ignores the value (a trigger)
	TSecret PortType = "secrets" // opaque; only the secrets input of Code takes it
	// TValue is for inputs only: a scalar or a document (what Code takes over a link).
	TValue PortType = "value"

	TRefStep      PortType = "ref<step>"
	TRefNote      PortType = "ref<note>"
	TRefTemplate  PortType = "ref<template>"
	TRefQuest     PortType = "ref<quest>"
	TRefWorkspace PortType = "ref<workspace>"
)

// Port is one input or output of a node.
type Port struct {
	Name string   `json:"name"`
	Type PortType `json:"type"`
	// Optional: the input may stay unwired, and a node fires without a value on it.
	Optional bool `json:"optional,omitempty"`
	// Group: of the inputs sharing a non-empty group at least one must be wired (and hold a value to fire).
	Group string `json:"group,omitempty"`
	// Multi: the input takes any number of links (Code's values, the inputs of Any of / All of).
	Multi bool `json:"multi,omitempty"`
}

// scalar reports the types a Compare, a Parse field or a value over a link may have.
func (t PortType) scalar() bool {
	return t == TInt || t == TFloat || t == TStr || t == TBool
}

// value reports a scalar or a document.
func (t PortType) value() bool { return t.scalar() || t == TDoc }

// Assignable says whether an output of type from may feed an input of type to: equal types, `int → float`, a scalar → `doc`,
// anything but secrets → `any`, a scalar or document → `value`. Everything else goes through an explicit node.
func Assignable(from, to PortType) bool {
	if from == to {
		return true
	}
	switch to {
	case TAny:
		return from != TSecret
	case TFloat:
		return from == TInt
	case TDoc:
		return from.scalar()
	case TValue:
		return from.value()
	}
	return false
}
