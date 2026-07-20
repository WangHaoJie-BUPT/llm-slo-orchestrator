package picker

import (
	"errors"

	"inference.bupt.dev/llm-slo-orchestrator/internal/api"
)

var ErrNoEligibleEndpoint = errors.New("no eligible endpoint")

// Picker has pure-function semantics: implementations must not perform network I/O.
type Picker interface {
	Pick(request api.Request, snapshot api.EndpointSnapshot, policy api.Policy) (api.Decision, error)
}
