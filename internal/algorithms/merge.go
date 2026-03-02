package algorithms

// Action represents the minimal action fields needed for queue merging.
type Action struct {
	ID            string
	CreatedAt     int64
	ScheduledDate string
	NextPeriod    string
}

// IsUserAdded is a function that checks if an action was manually added.
type IsUserAdded func(action *Action) bool

// DefaultIsUserAdded checks that the action has no scheduled date and no recurring period.
func DefaultIsUserAdded(action *Action) bool {
	return action.ScheduledDate == "" && action.NextPeriod == ""
}

// MergePrevCurrentQueue merges user-manually-added actions from a previous queue
// into a new scheduled action queue, preserving user ordering intent.
func MergePrevCurrentQueue(prevQueue, scheduledActions []*Action, isUserAdded IsUserAdded) []*Action {
	if len(prevQueue) == 0 {
		return scheduledActions
	}
	if len(scheduledActions) == 0 {
		return prevQueue
	}

	var headNodes []*Action
	var tailNodes []*Action

	// Step 1: Extract user-added from HEAD
	for len(prevQueue) > 0 && isUserAdded(prevQueue[0]) {
		headNodes = append(headNodes, prevQueue[0])
		prevQueue = prevQueue[1:]
	}
	if len(prevQueue) == 0 {
		return append(headNodes, scheduledActions...)
	}

	// Step 2: Extract user-added from TAIL
	for len(prevQueue) > 0 && isUserAdded(prevQueue[len(prevQueue)-1]) {
		tailNodes = append([]*Action{prevQueue[len(prevQueue)-1]}, tailNodes...)
		prevQueue = prevQueue[:len(prevQueue)-1]
	}
	if len(prevQueue) < 3 {
		result := append(headNodes, scheduledActions...)
		return append(result, tailNodes...)
	}

	// Step 3: Build lookup of scheduled action positions
	scheduledIDs := make([]int64, len(scheduledActions))
	for i, a := range scheduledActions {
		scheduledIDs[i] = a.CreatedAt
	}

	// Merge remaining user-added based on relative position
	for i := 1; i < len(prevQueue)-1; i++ {
		if !isUserAdded(prevQueue[i]) {
			continue
		}
		inserted := false
		for j := i + 1; j < len(prevQueue); j++ {
			idx := indexOf(scheduledIDs, prevQueue[j].CreatedAt)
			if idx >= 0 {
				scheduledActions = insertAt(scheduledActions, idx, prevQueue[i])
				scheduledIDs = insertAtInt64(scheduledIDs, idx, prevQueue[i].CreatedAt)
				inserted = true
				break
			}
		}
		if !inserted {
			tailNodes = append([]*Action{prevQueue[i]}, tailNodes...)
		}
	}

	result := append(headNodes, scheduledActions...)
	return append(result, tailNodes...)
}

func indexOf(slice []int64, val int64) int {
	for i, v := range slice {
		if v == val {
			return i
		}
	}
	return -1
}

func insertAt(slice []*Action, index int, val *Action) []*Action {
	if index >= len(slice) {
		return append(slice, val)
	}
	slice = append(slice, nil)
	copy(slice[index+1:], slice[index:])
	slice[index] = val
	return slice
}

func insertAtInt64(slice []int64, index int, val int64) []int64 {
	if index >= len(slice) {
		return append(slice, val)
	}
	slice = append(slice, 0)
	copy(slice[index+1:], slice[index:])
	slice[index] = val
	return slice
}
