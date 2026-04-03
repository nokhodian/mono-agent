package service

import "github.com/nokhodian/mono-agent/internal/workflow"

func RegisterAll(r *workflow.NodeTypeRegistry) {
	RegisterGroupA(r)
	RegisterGroupB(r)
}
