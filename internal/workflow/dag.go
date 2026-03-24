package workflow

import "strings"

// DAG represents the directed acyclic graph of a workflow.
type DAG struct {
	nodes       map[string]*WorkflowNode       // nodeID → node
	connections []WorkflowConnection
	outEdges    map[string][]WorkflowConnection // sourceNodeID → connections
	inEdges     map[string][]WorkflowConnection // targetNodeID → connections
}

// BuildDAG constructs a DAG from nodes and connections.
// Returns ErrCycleDetected if the graph has a cycle.
func BuildDAG(nodes []WorkflowNode, connections []WorkflowConnection) (*DAG, error) {
	d := &DAG{
		nodes:       make(map[string]*WorkflowNode, len(nodes)),
		connections: make([]WorkflowConnection, len(connections)),
		outEdges:    make(map[string][]WorkflowConnection),
		inEdges:     make(map[string][]WorkflowConnection),
	}

	for i := range nodes {
		n := nodes[i]
		d.nodes[n.ID] = &n
	}

	copy(d.connections, connections)

	for _, c := range connections {
		d.outEdges[c.SourceNodeID] = append(d.outEdges[c.SourceNodeID], c)
		d.inEdges[c.TargetNodeID] = append(d.inEdges[c.TargetNodeID], c)
	}

	// Eagerly detect cycles at construction time.
	if _, err := d.TopologicalSort(); err != nil {
		return nil, err
	}

	return d, nil
}

// TopologicalSort returns nodes in execution order using Kahn's BFS algorithm.
// Trigger nodes (NodeType starts with "trigger.") appear first in the initial queue.
// Returns ErrCycleDetected if the graph contains a cycle.
func (d *DAG) TopologicalSort() ([]WorkflowNode, error) {
	// Step 1: compute in-degree for all nodes.
	inDegree := make(map[string]int, len(d.nodes))
	for id := range d.nodes {
		inDegree[id] = len(d.inEdges[id])
	}

	// Step 2: seed the queue — trigger nodes first, then other zero-in-degree nodes.
	var triggers []WorkflowNode
	var others []WorkflowNode
	for id, deg := range inDegree {
		if deg == 0 {
			node := *d.nodes[id]
			if strings.HasPrefix(node.Type, "trigger.") {
				triggers = append(triggers, node)
			} else {
				others = append(others, node)
			}
		}
	}

	queue := make([]WorkflowNode, 0, len(triggers)+len(others))
	queue = append(queue, triggers...)
	queue = append(queue, others...)

	// Step 3: BFS.
	result := make([]WorkflowNode, 0, len(d.nodes))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		for _, conn := range d.outEdges[current.ID] {
			inDegree[conn.TargetNodeID]--
			if inDegree[conn.TargetNodeID] == 0 {
				next := *d.nodes[conn.TargetNodeID]
				queue = append(queue, next)
			}
		}
	}

	// Step 4: cycle check.
	if len(result) < len(d.nodes) {
		return nil, ErrCycleDetected
	}

	return result, nil
}

// TriggerNodes returns all nodes whose Type starts with "trigger.".
func (d *DAG) TriggerNodes() []WorkflowNode {
	var out []WorkflowNode
	for _, n := range d.nodes {
		if strings.HasPrefix(n.Type, "trigger.") {
			out = append(out, *n)
		}
	}
	return out
}

// Successors returns all nodes directly connected from the given node on any handle.
func (d *DAG) Successors(nodeID string) []WorkflowNode {
	conns := d.outEdges[nodeID]
	out := make([]WorkflowNode, 0, len(conns))
	for _, c := range conns {
		if n, ok := d.nodes[c.TargetNodeID]; ok {
			out = append(out, *n)
		}
	}
	return out
}

// SuccessorsOnHandle returns nodes connected from nodeID on a specific output handle.
func (d *DAG) SuccessorsOnHandle(nodeID string, handle string) []WorkflowNode {
	var out []WorkflowNode
	for _, c := range d.outEdges[nodeID] {
		if c.SourceHandle == handle {
			if n, ok := d.nodes[c.TargetNodeID]; ok {
				out = append(out, *n)
			}
		}
	}
	return out
}

// Predecessors returns all nodes that have an edge pointing to nodeID.
func (d *DAG) Predecessors(nodeID string) []WorkflowNode {
	conns := d.inEdges[nodeID]
	out := make([]WorkflowNode, 0, len(conns))
	for _, c := range conns {
		if n, ok := d.nodes[c.SourceNodeID]; ok {
			out = append(out, *n)
		}
	}
	return out
}

// InDegree returns the number of incoming edges for a node.
func (d *DAG) InDegree(nodeID string) int {
	return len(d.inEdges[nodeID])
}

// ConnectionsFrom returns all connections where source is nodeID.
func (d *DAG) ConnectionsFrom(nodeID string) []WorkflowConnection {
	return append([]WorkflowConnection(nil), d.outEdges[nodeID]...)
}

// ConnectionsTo returns all connections where target is nodeID.
func (d *DAG) ConnectionsTo(nodeID string) []WorkflowConnection {
	return append([]WorkflowConnection(nil), d.inEdges[nodeID]...)
}

// Node returns a node by ID. The returned pointer is a copy; mutations do not
// affect the DAG's internal state.
func (d *DAG) Node(nodeID string) (*WorkflowNode, bool) {
	n, ok := d.nodes[nodeID]
	if !ok {
		return nil, false
	}
	cp := *n
	return &cp, true
}

// AllNodes returns all nodes in the DAG (order is non-deterministic).
func (d *DAG) AllNodes() []WorkflowNode {
	out := make([]WorkflowNode, 0, len(d.nodes))
	for _, n := range d.nodes {
		out = append(out, *n)
	}
	return out
}
