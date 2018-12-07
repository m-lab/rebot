package node

import (
	"time"
)

const (
	// Unchecked is the default status, used when a reboot command has just
	// been issued.
	Unchecked = 0

	// Rebooted means the machine was successfully rebooted on the last run.
	Rebooted = 1

	// Failed means the machine is still offline after a reboot command.
	Failed = 2
)

// Node represents a machine on M-Lab's infrastructure
type Node struct {
	Name string
	Site string
}

// History holds the last reboot of a Node and the status.
//
// Status is always unchecked initially, and should be updated to rebooted
// or failed as soon as the information is available.
type History struct {
	Node
	LastReboot time.Time
	Status     uint8
}

// New returns a new Node
func New(name string, site string) Node {
	return Node{
		Name: name,
		Site: site,
	}
}

// NewHistory returns a new NodeHistory, defaulting Status to "unchecked".
func NewHistory(name string, site string, lastReboot time.Time) History {
	return History{
		New(name, site),
		lastReboot,
		Unchecked,
	}
}
