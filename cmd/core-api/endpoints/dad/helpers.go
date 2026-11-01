package dad

import dadSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/dad"

// newDadService builds the dad service. It is a var so tests can swap in a
// fake.
var newDadService = dadSvc.NewService
