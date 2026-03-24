package service

import "github.com/monoes/monoes-agent/internal/workflow"

func RegisterAll(r *workflow.NodeTypeRegistry) {
	RegisterGroupA(r)
	RegisterGroupB(r)
}
