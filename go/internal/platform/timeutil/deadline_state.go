package timeutil

import "time"

// DeadlineState is independent of how the deadline instant was obtained.
type DeadlineState struct {
	Remaining *int
	Tone      *string
	Urgent    bool
	Overdue   bool
	Reached   bool
}

func DeadlineAt(deadline *time.Time, window *int, expire bool, now time.Time) DeadlineState {
	rem := RemainingSeconds(deadline, now)
	state := DeadlineState{Remaining: rem}
	if rem == nil {
		return state
	}
	state.Reached = *rem <= 0
	if state.Reached {
		state.Remaining = nil
		if expire {
			tone := "overdue"
			state.Tone = &tone
			state.Overdue = true
			state.Urgent = true
		}
		return state
	}
	state.Tone = TimerTone(deadline, window, now)
	state.Urgent = IsInUrgentWindow(deadline, window, now)
	return state
}
