package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// requireEOF rejects any trailing JSON values or garbage after the first
// top-level value in a bounded body.
func requireEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errTrailingJSON
		}

		return fmt.Errorf("read trailing token: %w", err)
	}

	return nil
}
