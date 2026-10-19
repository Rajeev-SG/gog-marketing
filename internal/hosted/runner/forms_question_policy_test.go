package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	formsapi "google.golang.org/api/forms/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var errFormsQuestionFixture = errors.New("unexpected Forms question fixture request")

func TestNativeFormsQuestionsPersistSelectedItems(t *testing.T) {
	for _, scope := range []string{"forms.body", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			items := []*formsapi.Item{{ItemId: "seed", Title: "Seed"}}
			calls, posts := 0, 0
			expectedKind := ""
			expectedRequired := true
			expectedIndex := 0
			expectedGrade := false
			h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Forms = func(ctx context.Context, account string) (*formsapi.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted Forms identity lost")
					}
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++

						if r.URL.Hostname() != "forms.googleapis.com" {
							return nil, errFormsQuestionFixture
						}
						var response any
						form := func() map[string]any {
							return map[string]any{"formId": "selected-form", "info": map[string]any{"title": "Fixture"}, "settings": map[string]any{"quizSettings": map[string]any{"isQuiz": true}}, "items": items}
						}

						switch {
						case r.Method == http.MethodGet && r.URL.Path == "/v1/forms/selected-form":
							response = form()
						case r.Method == http.MethodPost && r.URL.Path == "/v1/forms/selected-form:batchUpdate":
							body, err := io.ReadAll(r.Body)
							if err != nil {
								return nil, fmt.Errorf("read Forms fixture: %w", err)
							}

							var wire struct {
								Requests []map[string]json.RawMessage `json:"requests"`
							}
							if err := json.Unmarshal(body, &wire); err != nil {
								return nil, fmt.Errorf("decode Forms wire: %w", err)
							}

							var batch formsapi.BatchUpdateFormRequest
							if err := json.Unmarshal(body, &batch); err != nil {
								return nil, fmt.Errorf("decode Forms SDK: %w", err)
							}

							if len(batch.Requests) != 1 || len(wire.Requests) != 1 || len(wire.Requests[0]) != 1 {
								return nil, errFormsQuestionFixture
							}

							req := batch.Requests[0]
							switch {
							case req.CreateItem != nil:
								create := req.CreateItem
								if create.Item == nil || create.Item.QuestionItem == nil || create.Item.QuestionItem.Question == nil || create.Location == nil {
									return nil, errFormsQuestionFixture
								}

								index := int(create.Location.Index)
								if index != expectedIndex || index < 0 || index > len(items) {
									return nil, errFormsQuestionFixture
								}

								if index == 0 && !strings.Contains(string(wire.Requests[0]["createItem"]), `"index":0`) {
									t.Error("zero insertion index omitted")
									return nil, errFormsQuestionFixture
								}

								q := create.Item.QuestionItem.Question
								if q.Required != expectedRequired {
									t.Error("required answer setting changed")
									return nil, errFormsQuestionFixture
								}
								kind := ""

								switch {
								case q.TextQuestion != nil:
									kind = "text"
									if q.TextQuestion.Paragraph {
										kind = "paragraph"
									}
								case q.ChoiceQuestion != nil:
									switch q.ChoiceQuestion.Type {
									case "RADIO":
										kind = "radio"
									case "CHECKBOX":
										kind = "checkbox"
									case "DROP_DOWN":
										kind = "dropdown"
									}

									if len(q.ChoiceQuestion.Options) != 2 || q.ChoiceQuestion.Options[0].Value != "A" || q.ChoiceQuestion.Options[1].Value != "B" {
										return nil, errFormsQuestionFixture
									}
								case q.ScaleQuestion != nil:
									kind = "scale"

									if q.ScaleQuestion.Low != 0 || q.ScaleQuestion.High != 10 || q.ScaleQuestion.LowLabel != "Low" || q.ScaleQuestion.HighLabel != "High" {
										return nil, errFormsQuestionFixture
									}
								case q.DateQuestion != nil:
									kind = "date"

									if !q.DateQuestion.IncludeTime || !q.DateQuestion.IncludeYear {
										return nil, errFormsQuestionFixture
									}
								case q.TimeQuestion != nil:
									kind = "time"

									if !q.TimeQuestion.Duration {
										return nil, errFormsQuestionFixture
									}
								}

								if kind != expectedKind {
									t.Errorf("question kind=%s want%s", kind, expectedKind)
									return nil, errFormsQuestionFixture
								}

								if expectedGrade && (q.Grading == nil || q.Grading.PointValue != 3 || q.Grading.CorrectAnswers == nil || len(q.Grading.CorrectAnswers.Answers) != 1 || q.Grading.CorrectAnswers.Answers[0].Value != "B") {
									return nil, errFormsQuestionFixture
								}
								create.Item.ItemId = fmt.Sprintf("created-%d", posts)

								items = append(items, nil)
								copy(items[index+1:], items[index:])
								items[index] = create.Item
							case req.MoveItem != nil:
								v := req.MoveItem
								if v.OriginalLocation == nil || v.NewLocation == nil {
									return nil, errFormsQuestionFixture
								}

								from, to := int(v.OriginalLocation.Index), int(v.NewLocation.Index)
								if from < 0 || from >= len(items) || to < 0 || to >= len(items) {
									return nil, errFormsQuestionFixture
								}
								item := items[from]
								next := append([]*formsapi.Item{}, items[:from]...)
								next = append(next, items[from+1:]...)
								next = append(next, nil)
								copy(next[to+1:], next[to:])
								next[to] = item
								items = next
							case req.DeleteItem != nil:
								if req.DeleteItem.Location == nil {
									return nil, errFormsQuestionFixture
								}

								index := int(req.DeleteItem.Location.Index)
								if index < 0 || index >= len(items) {
									return nil, errFormsQuestionFixture
								}
								items = append(append([]*formsapi.Item{}, items[:index]...), items[index+1:]...)
							default:
								return nil, errFormsQuestionFixture
							}

							posts++
							response = map[string]any{"form": form(), "replies": []any{map[string]any{}}}
						default:
							return nil, errFormsQuestionFixture
						}

						body, err := json.Marshal(response)
						if err != nil {
							return nil, fmt.Errorf("marshal Forms fixture: %w", err)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
					})

					return formsapi.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			call := func(name string, args map[string]any, destructive bool, change func(*ToolAuthorization)) capturedResponse {
				auth := ToolAuthorization{Service: "forms", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive, Resource: &ResourceGrantInput{Service: "forms", ResourceType: "file", ResourceID: "selected-form", Enabled: true}}
				if change != nil {
					change(&auth)
				}
				body := nativeToolBody(t, func(r *Request) { r.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth} })

				return postExecute(t, h.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
			}

			for _, kind := range []string{"text", "paragraph", "radio", "checkbox", "dropdown", "scale", "date", "time"} {
				expectedKind = kind
				expectedIndex = len(items)

				args := map[string]any{"form_id": "selected-form", "title": "New " + kind, "type": kind, "required": true}
				switch kind {
				case "radio", "checkbox", "dropdown":
					args["options"] = []string{"A", "B"}
				case "scale":
					args["scale_low"] = 0
					args["scale_high"] = 10
					args["scale_low_label"] = "Low"
					args["scale_high_label"] = "High"
				case "date":
					args["include_time"] = true
					args["include_year"] = true
				case "time":
					args["duration"] = true
				}

				if r := call("forms_add_question", args, false, nil); !r.OK {
					t.Fatalf("%s: %s", kind, r.Body)
				}

				if items[len(items)-1].Title != "New "+kind || items[0].ItemId != "seed" {
					t.Fatal("append changed other items")
				}
			}
			expectedKind, expectedIndex, expectedGrade = "radio", 0, true

			quiz := map[string]any{"form_id": "selected-form", "title": "Quiz", "type": "radio", "required": true, "options": []string{"A", "B"}, "correct": []string{"B"}, "points": 3, "index": 0}
			if r := call("forms_add_question", quiz, false, nil); !r.OK {
				t.Fatal(r.Body)
			}

			if items[0].Title != "Quiz" || items[1].ItemId != "seed" {
				t.Fatal("explicit zero insert changed existing ordering")
			}

			quizID := items[0].ItemId
			if r := call("forms_move_question", map[string]any{"form_id": "selected-form", "old_index": 0, "new_index": len(items) - 1}, false, nil); !r.OK {
				t.Fatal(r.Body)
			}

			if items[0].ItemId != "seed" || items[len(items)-1].ItemId != quizID {
				t.Fatal("move did not preserve other items")
			}

			before := calls
			if r := call("forms_delete_question", map[string]any{"form_id": "selected-form", "index": 0}, false, nil); r.OK || calls != before {
				t.Fatal("delete without destructive grant reached Google")
			}

			if r := call("forms_delete_question", map[string]any{"form_id": "selected-form", "index": 0}, true, nil); !r.OK {
				t.Fatal(r.Body)
			}

			if items[0].Title != "New text" || items[len(items)-1].ItemId != quizID || len(items) != 9 {
				t.Fatal("delete changed unselected items")
			}
			before = calls

			for name, args := range map[string]map[string]any{
				"forms_add_question":    {"form_id": "selected-form", "title": "Denied", "type": "text"},
				"forms_move_question":   {"form_id": "selected-form", "old_index": 0, "new_index": 1},
				"forms_delete_question": {"form_id": "selected-form", "index": 0},
			} {
				for _, change := range []func(*ToolAuthorization){
					func(a *ToolAuthorization) { a.AllowWrite = false },
					func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/forms.body.readonly"} },
					func(a *ToolAuthorization) { a.Resource = nil },
					func(a *ToolAuthorization) { a.Resource.Enabled = false },
					func(a *ToolAuthorization) { a.AllowedTools = []string{"forms_get"} },
					func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-form" },
				} {
					if r := call(name, args, true, change); r.OK || calls != before {
						t.Fatalf("unauthorized question edit reached Google: %s", r.Body)
					}
				}
			}
		})
	}
}

func TestNativeFormsQuestionGuardsPinMethodAndFile(t *testing.T) {
	for _, name := range []string{"forms_add_question", "forms_move_question", "forms_delete_question"} {
		t.Run(name, func(t *testing.T) {
			inv := &ToolInvocation{Name: name, Arguments: map[string]any{"form_id": "selected-form", "title": "Q", "old_index": 0, "new_index": 1, "index": 0}, Authorization: ToolAuthorization{Service: "forms", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: "forms", ResourceType: "file", ResourceID: "selected-form", Enabled: true}}}
			guard := nativeRequestGuard(inv)

			root := "https://forms.googleapis.com/v1/forms/selected-form"
			for _, test := range []struct {
				method, url string
				allow       bool
			}{
				{http.MethodPost, root + ":batchUpdate", true},
				{http.MethodGet, root, name != "forms_move_question"},
				{http.MethodGet, root + ":batchUpdate", false},
				{http.MethodPost, root, false},
				{http.MethodDelete, root, false},
				{http.MethodPost, "https://forms.googleapis.com/v1/forms/foreign-form:batchUpdate", false},
				{http.MethodPost, "https://example.test/v1/forms/selected-form:batchUpdate", false},
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
