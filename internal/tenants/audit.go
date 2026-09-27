package tenants

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuditEntry records one hosted-access decision or execution.
type AuditEntry struct {
	Time     string `json:"time"`
	Tenant   string `json:"tenant"`
	Action   string `json:"action"`
	Decision string `json:"decision,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// AuditLog appends one JSON object per line to the tenant's audit file.
// Writes are serialized per process; the file lives inside the tenant's
// isolated home so tenants cannot read each other's audit trail.
type AuditLog struct {
	mu   sync.Mutex
	path string
}

func NewAuditLog(tenantHome string) *AuditLog {
	return &AuditLog{path: filepath.Join(tenantHome, "audit", "audit.jsonl")}
}

func (l *AuditLog) Path() string {
	return l.path
}

func (l *AuditLog) Append(entry AuditEntry) error {
	if strings.TrimSpace(entry.Time) == "" {
		entry.Time = time.Now().UTC().Format(time.RFC3339)
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode audit entry: %w", err)
	}

	encoded = append(encoded, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if mkdirErr := os.MkdirAll(filepath.Dir(l.path), 0o700); mkdirErr != nil {
		return fmt.Errorf("ensure audit dir: %w", mkdirErr)
	}

	file, openErr := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if openErr != nil {
		return fmt.Errorf("open audit log: %w", openErr)
	}
	defer file.Close()

	if _, writeErr := file.Write(encoded); writeErr != nil {
		return fmt.Errorf("append audit entry: %w", writeErr)
	}

	return nil
}

// Tail returns the last limit audit entries in file order.
func (l *AuditLog) Tail(limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	raw, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return []AuditEntry{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read audit log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	out := make([]AuditEntry, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry AuditEntry
		if decodeErr := json.Unmarshal([]byte(line), &entry); decodeErr != nil {
			return nil, fmt.Errorf("decode audit entry: %w", decodeErr)
		}

		out = append(out, entry)
	}

	if len(out) > limit {
		out = out[len(out)-limit:]
	}

	return out, nil
}
