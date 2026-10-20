package controlplane

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
)

// These tests exercise the upstream pgx server-side protocol decoders without
// a network or database. gogcli itself uses pgx as a PostgreSQL client, so it
// does not expose pgproto3.Backend.Receive to untrusted clients. The tests
// therefore establish the upstream dependency's patched behavior; they are not
// evidence that gogcli was exploitable through that server-side decoder path.
func TestPgxProtocolSecurityRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("bind", func(t *testing.T) {
		t.Parallel()

		original := pgproto3.Bind{
			DestinationPortal:    "portal",
			PreparedStatement:    "statement",
			ParameterFormatCodes: []int16{0},
			Parameters:           [][]byte{[]byte("value")},
			ResultFormatCodes:    []int16{0},
		}

		encoded, err := original.Encode(nil)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if encoded[0] != 'B' {
			t.Fatalf("unexpected message type = %q", encoded[0])
		}
		// Encode includes the type byte and int32 length, which Decode omits.
		var decoded pgproto3.Bind
		if err := decoded.Decode(encoded[5:]); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if decoded.DestinationPortal != original.DestinationPortal ||
			decoded.PreparedStatement != original.PreparedStatement ||
			len(decoded.ParameterFormatCodes) != 1 || decoded.ParameterFormatCodes[0] != 0 ||
			len(decoded.Parameters) != 1 || !bytes.Equal(decoded.Parameters[0], original.Parameters[0]) ||
			len(decoded.ResultFormatCodes) != 1 || decoded.ResultFormatCodes[0] != 0 {
			t.Fatalf("round trip mismatch: got %+v", decoded)
		}
	})

	t.Run("function call", func(t *testing.T) {
		t.Parallel()

		original := pgproto3.FunctionCall{
			Function:         1234,
			ArgFormatCodes:   []uint16{0},
			Arguments:        [][]byte{[]byte("value")},
			ResultFormatCode: 1,
		}

		encoded, err := original.Encode(nil)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if encoded[0] != 'F' {
			t.Fatalf("unexpected message type = %q", encoded[0])
		}

		var decoded pgproto3.FunctionCall
		if err := decoded.Decode(encoded[5:]); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if decoded.Function != original.Function ||
			len(decoded.ArgFormatCodes) != 1 || decoded.ArgFormatCodes[0] != 0 ||
			len(decoded.Arguments) != 1 || !bytes.Equal(decoded.Arguments[0], original.Arguments[0]) ||
			decoded.ResultFormatCode != original.ResultFormatCode {
			t.Fatalf("round trip mismatch: got %+v", decoded)
		}
	})
}

func TestPgxProtocolSecurityMalformedDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		decode func() error
	}{
		{
			name: "bind rejects a negative parameter length",
			decode: func() error {
				body := bindBody("portal", "statement", 1, 1)
				// Replace the first parameter's length with -2. The value and
				// trailing result-format count are intentionally omitted.
				body = append(body, 0xff, 0xff, 0xff, 0xfe)

				return (&pgproto3.Bind{}).Decode(body)
			},
		},
		{
			name: "function call rejects a negative argument length",
			decode: func() error {
				body := make([]byte, 0, 12)
				body = binary.BigEndian.AppendUint32(body, 1234)
				body = append(body, 0, 0) // zero format codes
				body = append(body, 0, 1) // one argument
				// Replace that argument's length with -2; no value follows.
				body = append(body, 0xff, 0xff, 0xff, 0xfe)

				return (&pgproto3.FunctionCall{}).Decode(body)
			},
		},
		{
			name: "function call rejects a short body",
			decode: func() error {
				return (&pgproto3.FunctionCall{}).Decode([]byte{0, 0})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.decode()
			if err == nil {
				t.Fatal("Decode() = nil, want malformed-message error")
			}
		})
	}
}

func bindBody(destinationPortal, preparedStatement string, formatCodeCount, parameterCount int) []byte {
	body := append([]byte(destinationPortal), 0)
	body = append(body, preparedStatement...)
	body = append(body, 0)
	body = binary.BigEndian.AppendUint16(body, uint16(formatCodeCount))

	for i := 0; i < formatCodeCount; i++ {
		body = binary.BigEndian.AppendUint16(body, 0)
	}

	return binary.BigEndian.AppendUint16(body, uint16(parameterCount))
}
