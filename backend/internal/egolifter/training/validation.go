package training

import (
	"fmt"
	"strings"
)

const (
	maxNameLength = 100
	maxWeightKg   = 1000
	maxReps       = 100
)

func validateRoutineInput(name string, entries []EntryInput) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("validation: routine name is required")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("validation: routine name must be at most %d characters", maxNameLength)
	}
	if len(entries) == 0 {
		return fmt.Errorf("validation: a routine must contain at least one entry")
	}
	for i, entry := range entries {
		if strings.TrimSpace(entry.Name) == "" {
			return fmt.Errorf("validation: entry %d: name is required", i)
		}
		if len(entry.Name) > maxNameLength {
			return fmt.Errorf("validation: entry %d: name must be at most %d characters", i, maxNameLength)
		}
		if entry.WeightKg < 0 || entry.WeightKg > maxWeightKg {
			return fmt.Errorf("validation: entry %d: weight_kg must be between 0 and %d", i, maxWeightKg)
		}
		if entry.Reps <= 0 || entry.Reps > maxReps {
			return fmt.Errorf("validation: entry %d: reps must be between 1 and %d", i, maxReps)
		}
	}
	return nil
}
