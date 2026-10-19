package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var errSlidesEditFixture = errors.New("invalid Slides mutation fixture request")

type slidesEditState struct {
	text     map[string]string
	skipped  map[string]bool
	revision int
}

func (s *slidesEditState) presentation() *slides.Presentation {
	pages := make([]*slides.Page, 0, 2)

	for _, entry := range []struct{ id, object string }{{"p:0", "shape:one"}, {"p:1", "shape:two"}} {
		text := s.text[entry.object]
		pages = append(pages, &slides.Page{ObjectId: entry.id, SlideProperties: &slides.SlideProperties{IsSkipped: s.skipped[entry.id]}, PageElements: []*slides.PageElement{{ObjectId: entry.object, Shape: &slides.Shape{Text: &slides.TextContent{TextElements: []*slides.TextElement{{EndIndex: int64(len(utf16.Encode([]rune(text)))), TextRun: &slides.TextRun{Content: text}}}}}}}})
	}

	return &slides.Presentation{PresentationId: "selected-deck", RevisionId: fmt.Sprintf("r%d", s.revision), Slides: pages}
}

func (s *slidesEditState) apply(body []byte) (int64, error) {
	var raw struct {
		Requests []map[string]json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, fmt.Errorf("decode fixture: %w", err)
	}

	var batch slides.BatchUpdatePresentationRequest
	if err := json.Unmarshal(body, &batch); err != nil {
		return 0, fmt.Errorf("decode SDK batch: %w", err)
	}

	if len(raw.Requests) != len(batch.Requests) || len(batch.Requests) == 0 {
		return 0, errSlidesEditFixture
	}

	if batch.WriteControl != nil && batch.WriteControl.RequiredRevisionId != fmt.Sprintf("r%d", s.revision) {
		return 0, errSlidesEditFixture
	}

	nextText := make(map[string]string, len(s.text))
	for k, v := range s.text {
		nextText[k] = v
	}

	nextSkip := make(map[string]bool, len(s.skipped))
	for k, v := range s.skipped {
		nextSkip[k] = v
	}
	var changed int64

	for i, r := range batch.Requests {
		if len(raw.Requests[i]) != 1 {
			return 0, errSlidesEditFixture
		}

		switch {
		case r.UpdateSlideProperties != nil:
			v := r.UpdateSlideProperties
			if v.Fields != "isSkipped" || v.SlideProperties == nil {
				return 0, errSlidesEditFixture
			}

			if _, ok := nextSkip[v.ObjectId]; !ok {
				return 0, errSlidesEditFixture
			}

			if !v.SlideProperties.IsSkipped && !strings.Contains(string(raw.Requests[i]["updateSlideProperties"]), `"isSkipped":false`) {
				return 0, errSlidesEditFixture
			}
			nextSkip[v.ObjectId] = v.SlideProperties.IsSkipped
		case r.ReplaceAllText != nil:
			v := r.ReplaceAllText
			if v.ContainsText == nil || v.ContainsText.Text == "" {
				return 0, errSlidesEditFixture
			}

			pattern := regexp.QuoteMeta(v.ContainsText.Text)
			if !v.ContainsText.MatchCase {
				pattern = "(?i)" + pattern
			}

			matcher, err := regexp.Compile(pattern)
			if err != nil {
				return 0, fmt.Errorf("literal fixture matcher: %w", err)
			}
			pages := map[string]bool{}

			for _, p := range v.PageObjectIds {
				if _, ok := nextSkip[p]; !ok {
					return 0, errSlidesEditFixture
				}
				pages[p] = true
			}

			for _, entry := range []struct{ page, object string }{{"p:0", "shape:one"}, {"p:1", "shape:two"}} {
				if len(pages) > 0 && !pages[entry.page] {
					continue
				}
				text := nextText[entry.object]
				changed += int64(len(matcher.FindAllStringIndex(text, -1)))
				nextText[entry.object] = matcher.ReplaceAllStringFunc(text, func(string) string { return v.ReplaceText })
			}
		case r.DeleteText != nil:
			v := r.DeleteText

			text, ok := nextText[v.ObjectId]
			if !ok || v.TextRange == nil || v.TextRange.Type != "FIXED_RANGE" || v.TextRange.StartIndex == nil || v.TextRange.EndIndex == nil {
				return 0, errSlidesEditFixture
			}
			units := utf16.Encode([]rune(text))

			a, b := int(*v.TextRange.StartIndex), int(*v.TextRange.EndIndex)
			if a < 0 || b < a || b > len(units) {
				return 0, errSlidesEditFixture
			}
			nextText[v.ObjectId] = string(utf16.Decode(append(append([]uint16{}, units[:a]...), units[b:]...)))
		case r.InsertText != nil:
			v := r.InsertText

			text, ok := nextText[v.ObjectId]
			if !ok {
				return 0, errSlidesEditFixture
			}
			units := utf16.Encode([]rune(text))

			at := int(v.InsertionIndex)
			if at < 0 || at > len(units) {
				return 0, errSlidesEditFixture
			}
			joined := append(append([]uint16{}, units[:at]...), utf16.Encode([]rune(v.Text))...)
			nextText[v.ObjectId] = string(utf16.Decode(append(joined, units[at:]...)))
		default:
			return 0, errSlidesEditFixture
		}
	}
	s.text, s.skipped = nextText, nextSkip
	s.revision++

	return changed, nil
}

func TestNativeSlidesEditsPersistScopedMutations(t *testing.T) {
	for _, scope := range []string{"presentations", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			state := slidesEditState{text: map[string]string{"shape:one": "Launch 🚀 launch", "shape:two": "Launch outside"}, skipped: map[string]bool{"p:0": false, "p:1": false}, revision: 1}
			calls, posts := 0, 0
			h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Slides = func(ctx context.Context, account string) (*slides.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted edit identity lost")
					}
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++

						if r.URL.Hostname() != nativeSlidesHost {
							return nil, errSlidesEditFixture
						}
						var response any

						switch {
						case r.Method == http.MethodGet && r.URL.Path == "/v1/presentations/selected-deck":
							response = state.presentation()
						case r.Method == http.MethodPost && r.URL.Path == "/v1/presentations/selected-deck:batchUpdate":
							body, err := io.ReadAll(r.Body)
							if err != nil {
								return nil, fmt.Errorf("read fixture body: %w", err)
							}

							n, err := state.apply(body)
							if err != nil {
								return nil, err
							}

							posts++
							response = map[string]any{"replies": []any{map[string]any{"replaceAllText": map[string]any{"occurrencesChanged": n}}}}
						default:
							return nil, errSlidesEditFixture
						}

						body, err := json.Marshal(response)
						if err != nil {
							return nil, fmt.Errorf("marshal fixture: %w", err)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
					})

					return slides.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			call := func(name string, args map[string]any, change func(*ToolAuthorization)) capturedResponse {
				auth := ToolAuthorization{Service: "slides", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true}}
				if change != nil {
					change(&auth)
				}
				body := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth} })

				return postExecute(t, h.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
			}
			good := func(name string, args map[string]any, change func(*ToolAuthorization)) {
				t.Helper()

				if r := call(name, args, change); !r.OK {
					t.Fatalf("%s: %s", name, r.Body)
				}
			}
			good("slides_skip_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, nil)

			if !state.skipped["p:0"] || state.skipped["p:1"] {
				t.Fatal("skip affected wrong page")
			}

			good("slides_unskip_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, nil)

			if state.skipped["p:0"] {
				t.Fatal("explicit false unskip not persisted")
			}

			good("slides_replace_text", map[string]any{"presentation_id": "selected-deck", "find": "Launch", "replacement": "Page", "page_ids": []string{"p:1"}}, nil)

			if state.text["shape:one"] != "Launch 🚀 launch" || state.text["shape:two"] != "Page outside" {
				t.Fatalf("page selection/preservation: %v", state.text)
			}

			// Arrange a matching external edit so object selection is independently discriminating.
			state.text["shape:two"] = "Launch outside"
			state.revision++

			good("slides_replace_text", map[string]any{"presentation_id": "selected-deck", "find": "launch", "replacement": "Done", "object_id": "shape:one"}, nil)

			if state.text["shape:one"] != "Done 🚀 Done" || state.text["shape:two"] != "Launch outside" {
				t.Fatalf("object UTF16 edits/preservation: %v", state.text)
			}

			// Deletion must also preserve matching text outside the selected object.
			state.text["shape:two"] = "Done outside"
			state.revision++

			good("slides_replace_text", map[string]any{"presentation_id": "selected-deck", "find": "Done", "replacement": "", "object_id": "shape:one"}, func(a *ToolAuthorization) { a.AllowDestructive = true })

			if state.text["shape:one"] != " 🚀 " || state.text["shape:two"] != "Done outside" {
				t.Fatalf("empty replacement/preservation: %v", state.text)
			}

			good("slides_replace_text", map[string]any{"presentation_id": "selected-deck", "find": " ", "replacement": "_", "all": true}, nil)

			if state.text["shape:one"] != "_🚀_" || state.text["shape:two"] != "Done_outside" {
				t.Fatalf("whole-deck literal replace: %v", state.text)
			}

			if posts != 6 {
				t.Fatalf("posts=%d want6", posts)
			}
			before := calls

			base := map[string]any{"presentation_id": "selected-deck", "find": "_", "replacement": "Ready", "all": true}
			for _, change := range []func(*ToolAuthorization){
				func(a *ToolAuthorization) { a.AllowWrite = false },
				func(a *ToolAuthorization) {
					a.Scopes = []string{"https://www.googleapis.com/auth/presentations.readonly"}
				},
				func(a *ToolAuthorization) { a.Resource = nil },
				func(a *ToolAuthorization) { a.Resource.Enabled = false },
				func(a *ToolAuthorization) { a.AllowedTools = []string{"slides_list_slides"} },
				func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-deck" },
			} {
				if r := call("slides_replace_text", base, change); r.OK || calls != before {
					t.Fatalf("unauthorized mutation reached Google: %s", r.Body)
				}
			}

			if r := call("slides_replace_text", map[string]any{"presentation_id": "selected-deck", "find": "_", "replacement": "", "all": true}, nil); r.OK || calls != before {
				t.Fatalf("empty replacement without destructive grant: %s", r.Body)
			}
		})
	}
}

func TestNativeSlidesEditsPinSelectedFileAndMethods(t *testing.T) {
	for _, name := range []string{"slides_replace_text", "slides_skip_slide", "slides_unskip_slide"} {
		t.Run(name, func(t *testing.T) {
			inv := &ToolInvocation{Name: name, Arguments: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "find": "old", "replacement": "new", "all": true}, Authorization: ToolAuthorization{Service: "slides", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true}}}
			guard := nativeRequestGuard(inv)

			root := "https://slides.googleapis.com/v1/presentations/selected-deck"
			for _, test := range []struct {
				method, url string
				allow       bool
			}{
				{http.MethodPost, root + ":batchUpdate", true},
				{http.MethodGet, root, name == "slides_replace_text"},
				{http.MethodGet, root + ":batchUpdate", false},
				{http.MethodPost, root, false},
				{http.MethodDelete, root, false},
				{http.MethodPost, "https://slides.googleapis.com/v1/presentations/foreign-deck:batchUpdate", false},
				{http.MethodPost, "https://example.test/v1/presentations/selected-deck:batchUpdate", false},
			} {
				req, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
				if err != nil {
					t.Fatal(err)
				}

				if err := guard(req); (err == nil) != test.allow {
					t.Errorf("%s %s allow=%t error=%v", test.method, test.url, test.allow, err)
				}
			}
		})
	}
}
