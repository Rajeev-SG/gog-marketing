package runner

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/hosted/native"
	"github.com/openclaw/gogcli/internal/hosted/policy"
)

const OperationVerifyFiles = "verify_files"

type FileSelection struct {
	IDs    []string `json:"ids"`
	Scopes []string `json:"scopes"`
}
type SelectedFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
}
type FileVerificationResult struct {
	Files    []SelectedFile `json:"files"`
	Rejected []string       `json:"rejected"`
}

func validateFileSelection(selection *FileSelection) error {
	if selection == nil || len(selection.IDs) == 0 || len(selection.IDs) > 20 || len(selection.Scopes) > 128 {
		return invalidf("invalid selected files")
	}

	if policy.ValidateScopes(selection.Scopes) != nil || !slices.Contains(selection.Scopes, "https://www.googleapis.com/auth/drive.file") {
		return invalidf("selected-file scope is required")
	}

	seen := map[string]bool{}
	for _, id := range selection.IDs {
		if !fileResourceID.MatchString(id) || seen[id] {
			return invalidf("invalid selected file identifier")
		}
		seen[id] = true
	}

	return nil
}

// Browser Picker names/MIME/account claims are not authority. Use the existing
// typed Drive client with the account's drive.file token before saving grants.
func (s *Server) executeFileVerification(ctx context.Context, req *Request) outcome {
	ids := req.FileSelection.IDs
	ctx = googleapi.WithRequestGuard(ctx, func(request *http.Request) error {
		if request.URL == nil || request.URL.Scheme != "https" || request.URL.Host != "www.googleapis.com" || request.URL.User != nil || request.Method != http.MethodGet || request.Header.Get("X-HTTP-Method-Override") != "" {
			return native.ErrDenied
		}

		for _, id := range ids {
			if request.URL.Path == "/drive/v3/files/"+id {
				return nil
			}
		}

		return native.ErrDenied
	})

	factory := s.Engine.NativeServices.Drive
	if factory == nil {
		factory = googleapi.NewDrive
	}

	service, err := factory(ctx, req.GoogleEmail)
	if err != nil {
		return engineOutcome("selected-file verification unavailable")
	}

	result := FileVerificationResult{Files: []SelectedFile{}, Rejected: []string{}}
	for _, id := range ids {
		file, err := service.Files.Get(id).Fields("id,name,mimeType,trashed").SupportsAllDrives(true).Context(ctx).Do()
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			return timeoutOutcome()
		}

		if err != nil || file == nil || file.Id != id || file.Trashed || file.MimeType == "application/vnd.google-apps.shortcut" {
			result.Rejected = append(result.Rejected, id)
			continue
		}
		result.Files = append(result.Files, SelectedFile{ID: file.Id, Name: file.Name, MimeType: file.MimeType})
	}

	return outcome{result: result}
}
