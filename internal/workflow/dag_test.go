package workflow

import (
	"errors"
	"testing"
)

// nodeIDs extracts the IDs from a slice of WorkflowNode in order.
func nodeIDs(nodes []WorkflowNode) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

// makeNode builds a WorkflowNode with the given id and Type.
func makeNode(id, nodeType string) WorkflowNode {
	return WorkflowNode{ID: id, Type: nodeType, Name: id}
}

// makeConn builds a WorkflowConnection between src and dst on the default handles.
func makeConn(id, src, dst string) WorkflowConnection {
	return WorkflowConnection{
		ID:           id,
		SourceNodeID: src,
		TargetNodeID: dst,
		SourceHandle: "output",
		TargetHandle: "input",
	}
}

// containsAll asserts that every expected string appears in got (order-insensitive).
func containsAll(t *testing.T, got []string, expected []string) {
	t.Helper()
	set := make(map[string]bool, len(got))
	for _, s := range got {
		set[s] = true
	}
	for _, e := range expected {
		if !set[e] {
			t.Errorf("expected %q to be present in %v", e, got)
		}
	}
}

// TestDAGLinearChain verifies that A→B→C produces the sort order [A, B, C].
func TestDAGLinearChain(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("A", "action.a"),
		makeNode("B", "action.b"),
		makeNode("C", "action.c"),
	}
	conns := []WorkflowConnection{
		makeConn("1", "A", "B"),
		makeConn("2", "B", "C"),
	}

	dag, err := BuildDAG(nodes, conns)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}

	ids := nodeIDs(sorted)
	if len(ids) != 3 {
		t.Fatalf("expected 3 nodes, got %d: %v", len(ids), ids)
	}
	if ids[0] != "A" || ids[1] != "B" || ids[2] != "C" {
		t.Errorf("expected [A B C], got %v", ids)
	}
}

// TestDAGDiamond verifies A→B→D, A→C→D: A first, D last, B and C in between.
func TestDAGDiamond(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("A", "action.a"),
		makeNode("B", "action.b"),
		makeNode("C", "action.c"),
		makeNode("D", "action.d"),
	}
	conns := []WorkflowConnection{
		makeConn("1", "A", "B"),
		makeConn("2", "A", "C"),
		makeConn("3", "B", "D"),
		makeConn("4", "C", "D"),
	}

	dag, err := BuildDAG(nodes, conns)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}

	ids := nodeIDs(sorted)
	if len(ids) != 4 {
		t.Fatalf("expected 4 nodes, got %d: %v", len(ids), ids)
	}
	if ids[0] != "A" {
		t.Errorf("expected A first, got %v", ids)
	}
	if ids[len(ids)-1] != "D" {
		t.Errorf("expected D last, got %v", ids)
	}
	containsAll(t, ids, []string{"A", "B", "C", "D"})
}

// TestDAGCycle verifies that A→B→A returns ErrCycleDetected.
func TestDAGCycle(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("A", "action.a"),
		makeNode("B", "action.b"),
	}
	conns := []WorkflowConnection{
		makeConn("1", "A", "B"),
		makeConn("2", "B", "A"),
	}

	_, err := BuildDAG(nodes, conns)
	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

// TestDAGDisconnected verifies that nodes with no connections all appear in the result.
func TestDAGDisconnected(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("X", "action.x"),
		makeNode("Y", "action.y"),
		makeNode("Z", "action.z"),
	}

	dag, err := BuildDAG(nodes, nil)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}

	ids := nodeIDs(sorted)
	if len(ids) != 3 {
		t.Fatalf("expected 3 nodes, got %d: %v", len(ids), ids)
	}
	containsAll(t, ids, []string{"X", "Y", "Z"})
}

// TestDAGEmpty verifies that an empty workflow returns an empty slice without error.
func TestDAGEmpty(t *testing.T) {
	dag, err := BuildDAG(nil, nil)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}

	if len(sorted) != 0 {
		t.Errorf("expected empty slice, got %v", nodeIDs(sorted))
	}
}

// TestDAGTriggerFirst verifies that a trigger-typed node appears first even
// when it is not the first element in the input slice.
func TestDAGTriggerFirst(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("B", "action.b"),       // listed before A intentionally
		makeNode("C", "action.c"),
		makeNode("A", "trigger.cron"),   // trigger
	}
	conns := []WorkflowConnection{
		makeConn("1", "A", "B"),
		makeConn("2", "B", "C"),
	}

	dag, err := BuildDAG(nodes, conns)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	sorted, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort returned error: %v", err)
	}

	ids := nodeIDs(sorted)
	if len(ids) != 3 {
		t.Fatalf("expected 3 nodes, got %d: %v", len(ids), ids)
	}
	if ids[0] != "A" {
		t.Errorf("expected trigger node A first, got %v", ids)
	}
	if ids[1] != "B" || ids[2] != "C" {
		t.Errorf("expected [A B C], got %v", ids)
	}
}

// TestDAGSuccessors verifies Successors returns all direct successors.
func TestDAGSuccessors(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("A", "action.a"),
		makeNode("B", "action.b"),
		makeNode("C", "action.c"),
	}
	conns := []WorkflowConnection{
		makeConn("1", "A", "B"),
		makeConn("2", "A", "C"),
	}

	dag, err := BuildDAG(nodes, conns)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	succs := dag.Successors("A")
	ids := nodeIDs(succs)
	if len(ids) != 2 {
		t.Fatalf("expected 2 successors of A, got %d: %v", len(ids), ids)
	}
	containsAll(t, ids, []string{"B", "C"})
}

// TestDAGSuccessorsOnHandle verifies handle-specific successor filtering.
func TestDAGSuccessorsOnHandle(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("A", "core.if"),
		makeNode("B", "action.b"),
		makeNode("C", "action.c"),
	}
	conns := []WorkflowConnection{
		{ID: "1", SourceNodeID: "A", TargetNodeID: "B", SourceHandle: "true", TargetHandle: "input"},
		{ID: "2", SourceNodeID: "A", TargetNodeID: "C", SourceHandle: "false", TargetHandle: "input"},
	}

	dag, err := BuildDAG(nodes, conns)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	trueSuccs := dag.SuccessorsOnHandle("A", "true")
	if len(trueSuccs) != 1 || trueSuccs[0].ID != "B" {
		t.Errorf("expected [B] on handle 'true', got %v", nodeIDs(trueSuccs))
	}

	falseSuccs := dag.SuccessorsOnHandle("A", "false")
	if len(falseSuccs) != 1 || falseSuccs[0].ID != "C" {
		t.Errorf("expected [C] on handle 'false', got %v", nodeIDs(falseSuccs))
	}
}

// TestDAGPredecessors verifies Predecessors returns the correct incoming nodes.
func TestDAGPredecessors(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("A", "action.a"),
		makeNode("B", "action.b"),
		makeNode("C", "action.c"),
	}
	conns := []WorkflowConnection{
		makeConn("1", "A", "C"),
		makeConn("2", "B", "C"),
	}

	dag, err := BuildDAG(nodes, conns)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	preds := dag.Predecessors("C")
	ids := nodeIDs(preds)
	if len(ids) != 2 {
		t.Fatalf("expected 2 predecessors of C, got %d: %v", len(ids), ids)
	}
	containsAll(t, ids, []string{"A", "B"})
}

// TestDAGInDegree verifies in-degree counts.
func TestDAGInDegree(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("A", "action.a"),
		makeNode("B", "action.b"),
		makeNode("C", "action.c"),
	}
	conns := []WorkflowConnection{
		makeConn("1", "A", "C"),
		makeConn("2", "B", "C"),
	}

	dag, err := BuildDAG(nodes, conns)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	if d := dag.InDegree("A"); d != 0 {
		t.Errorf("expected in-degree 0 for A, got %d", d)
	}
	if d := dag.InDegree("B"); d != 0 {
		t.Errorf("expected in-degree 0 for B, got %d", d)
	}
	if d := dag.InDegree("C"); d != 2 {
		t.Errorf("expected in-degree 2 for C, got %d", d)
	}
}

// TestDAGNode verifies the Node lookup by ID.
func TestDAGNode(t *testing.T) {
	nodes := []WorkflowNode{makeNode("A", "action.a")}

	dag, err := BuildDAG(nodes, nil)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	n, ok := dag.Node("A")
	if !ok || n.ID != "A" {
		t.Errorf("expected to find node A, got ok=%v node=%v", ok, n)
	}

	_, ok = dag.Node("MISSING")
	if ok {
		t.Error("expected Node('MISSING') to return false")
	}
}

// TestDAGTriggerNodes verifies TriggerNodes returns only trigger-typed nodes.
func TestDAGTriggerNodes(t *testing.T) {
	nodes := []WorkflowNode{
		makeNode("T1", "trigger.cron"),
		makeNode("T2", "trigger.webhook"),
		makeNode("A1", "action.send_email"),
	}

	dag, err := BuildDAG(nodes, nil)
	if err != nil {
		t.Fatalf("BuildDAG returned error: %v", err)
	}

	triggers := dag.TriggerNodes()
	ids := nodeIDs(triggers)
	if len(ids) != 2 {
		t.Fatalf("expected 2 trigger nodes, got %d: %v", len(ids), ids)
	}
	containsAll(t, ids, []string{"T1", "T2"})
}
