package node

import (
	"time"
)

// NodeStatus is an alias for uint8, used for readability.
type NodeStatus uint8

const (
	// NotObserved is the default status, used when a reboot command has just
	// been issued.
	NotObserved = NodeStatus(0)

	// ObservedOnline means the machine was seen online during the last run.
	ObservedOnline = NodeStatus(1)

	// ObservedOffline means the machine is still seen as offline after a
	// reboot command.
	ObservedOffline = NodeStatus(2)
)

// Node represents a machine on M-Lab's infrastructure
type Node struct {
	Name string
	Site string
}

// History holds the last reboot of a Node and the status.
//
// Status is always NotObserved initially, and should be updated to
// ObservedOnline or ObservedOffline as soon as the information is available.
type History struct {
	Node
	LastReboot time.Time
	Status     NodeStatus
}

// New returns a new Node
func New(name string, site string) Node {
	return Node{
		Name: name,
		Site: site,
	}
}

// NewHistory returns a new NodeHistory, defaulting Status to "NotObserved".
func NewHistory(name string, site string, lastReboot time.Time) History {
	return History{
		New(name, site),
		lastReboot,
		NotObserved,
	}
}
