package runner

import "errors"

var (
	errEngineReached           = errors.New("engine reached")
	errMissingCapabilityClaims = errors.New("missing capability claims")
	errNonIntegerTimeClaim     = errors.New("non-integer time claim")
	errUnsupportedTimeClaim    = errors.New("unsupported time claim")
	errTrailingJSON            = errors.New("trailing JSON content")
	errOutputLimit             = errors.New("result exceeded the output limit")
)
