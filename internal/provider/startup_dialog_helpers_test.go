package provider_test

import (
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// dogName and dogBlock are the session startupProvider's tests launch and the
// block the fake gives it.
const (
	dogName               = "Indigo--dog-1"
	dogBlock  rex.BlockID = "block:dog-1"
	trustName             = "Trust this folder?"
)

// idleProgramStatuses is a program that reported idle once and never changes,
// so a submit is never confirmed by program_status.
func idleProgramStatuses() map[rex.BlockID]rex.ProgramStatus {
	return map[rex.BlockID]rex.ProgramStatus{
		dogBlock: {Records: []rex.ProgramStatusRecord{{State: "idle", UpdatedAt: time.Now()}}},
	}
}
