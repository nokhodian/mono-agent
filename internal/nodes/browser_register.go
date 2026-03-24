package nodes

import (
	"fmt"
	"strings"

	"github.com/monoes/monoes-agent/internal/action"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// RegisterBrowserNodes registers all browser/social action types as workflow nodes.
// Node types follow the pattern "<platform>.<action_type>", e.g. "linkedin.find_by_keyword".
func RegisterBrowserNodes(r *workflow.NodeTypeRegistry) {
	loader := action.GetLoader()
	available, err := loader.ListAvailable()
	if err != nil {
		return
	}

	for _, entry := range available {
		// entry is "platform/action_type"
		parts := strings.SplitN(entry, "/", 2)
		if len(parts) != 2 {
			continue
		}
		platform := parts[0]
		actionType := parts[1]
		nodeType := fmt.Sprintf("%s.%s", platform, actionType)

		// Capture for closure
		p, a := platform, actionType
		r.Register(nodeType, func() workflow.NodeExecutor {
			return NewBrowserNode(p, a)
		})
	}
}
