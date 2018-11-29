package node

import (
	"time"
)

// Node represents a machine on M-Lab's infrastructure
type Node struct {
	Name string
	Site string
}

// History holds the last reboot of a Node.
type History struct {
	Node
	LastReboot time.Time
}

// New returns a new Node
func New(name string, site string) Node {
	return Node{
		Name: name,
		Site: site,
	}
}

// NewHistory returns a new NodeHistory.
func NewHistory(name string, site string, lastReboot time.Time) History {
	return History{
		New(name, site),
		lastReboot,
	}
}
