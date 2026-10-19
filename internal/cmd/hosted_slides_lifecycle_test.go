package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func hostedSlidesLifecycleTool(t *testing.T, name string) mcpToolSpec {
	t.Helper()
	for _, spec := range hostedSlidesLifecycleTools() {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("missing hosted tool %q", name)
	return mcpToolSpec{}
}

func TestHostedSlidesLifecycleBuildArgs(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
		want []string
	}{
		{"new default", "slides_new_slide", map[string]any{"presentation_id": "selected-deck"}, []string{"slides", "new-slide", "--", "selected-deck"}},
		{"new predefined layout", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout": "TITLE_AND_BODY", "index": int64(0)}, []string{"slides", "new-slide", "--layout=TITLE_AND_BODY", "--index=0", "--", "selected-deck"}},
		{"new layout id", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout_id": "layout:one"}, []string{"slides", "new-slide", "--layout-id=layout:one", "--", "selected-deck"}},
		{"duplicate index", "slides_duplicate_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "to_index": int64(2)}, []string{"slides", "duplicate-slide", "--to-index=2", "--", "selected-deck", "p:0"}},
		{"duplicate default", "slides_duplicate_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, []string{"slides", "duplicate-slide", "--", "selected-deck", "p:0"}},
		{"move index", "slides_move_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:1", "to_index": int64(0)}, []string{"slides", "move-slide", "--to-index=0", "--", "selected-deck", "p:1"}},
		{"delete", "slides_delete_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, []string{"slides", "delete-slide", "--", "selected-deck", "p:0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := hostedSlidesLifecycleTool(t, test.tool)
			got, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: test.args}})
			if err != nil {
				t.Fatalf("BuildArgs: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("argv = %q, want %q", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("argv = %q, want %q", got, test.want)
				}
			}
		})
	}
}

func TestHostedSlidesLifecycleBuildArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"empty layout", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout": ""}},
		{"blank layout", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout": " "}},
		{"bad layout", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout": "FANCY"}},
		{"empty layout id", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout_id": ""}},
		{"bad layout id", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout_id": "bad id"}},
		{"layout conflict", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout": "BLANK", "layout_id": "layout:one"}},
		{"fractional index", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "index": 0.5}},
		{"negative index", "slides_new_slide", map[string]any{"presentation_id": "selected-deck", "index": int64(-1)}},
		{"bad presentation", "slides_new_slide", map[string]any{"presentation_id": "bad deck"}},
		{"missing slide", "slides_duplicate_slide", map[string]any{"presentation_id": "selected-deck"}},
		{"bad slide", "slides_duplicate_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "bad slide"}},
		{"fractional target", "slides_duplicate_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "to_index": 1.5}},
		{"missing target", "slides_move_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}},
		{"negative target", "slides_move_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "to_index": int64(-1)}},
		{"delete bad object", "slides_delete_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := hostedSlidesLifecycleTool(t, test.tool).BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: test.args}}); err == nil {
				t.Fatal("BuildArgs err = nil, want non-nil")
			}
		})
	}
}

func TestSlidesLifecycleHandlersDoNotRetryEffects(t *testing.T) {
	var effects atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/presentations/pres1:batchUpdate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if effects.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":503,"message":"transient backend failure","status":"UNAVAILABLE"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"presentationId":"pres1","replies":[{}]}`))
	}))
	defer server.Close()

	factory := func(ctx context.Context) (*slides.Service, error) {
		retry := googleapi.NewRetryTransport(server.Client().Transport)
		retry.MaxRetries5xx = 1
		return slides.NewService(ctx, option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(&http.Client{Transport: retry}))
	}

	tests := []struct {
		name  string
		force bool
		run   func(context.Context, *RootFlags) error
	}{
		{"new", false, func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesNewSlideCmd{PresentationID: "pres1"}).Run(ctx, flags)
		}},
		{"duplicate", false, func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesDuplicateSlideCmd{PresentationID: "pres1", SlideID: "slide:one"}).Run(ctx, flags)
		}},
		{"move", false, func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesMoveSlideCmd{PresentationID: "pres1", SlideID: "slide:one", ToIndex: int64TestPtr(0)}).Run(ctx, flags)
		}},
		{"delete", true, func(ctx context.Context, flags *RootFlags) error {
			return (&SlidesDeleteSlideCmd{PresentationID: "pres1", SlideID: "slide:one"}).Run(ctx, flags)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			effects.Store(0)
			ctx := withSlidesTestServiceFactory(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), func(requestCtx context.Context, _ string) (*slides.Service, error) { return factory(requestCtx) })
			err := test.run(ctx, &RootFlags{Account: "a@b.com", NoInput: true, Force: test.force})
			if err == nil {
				t.Fatal("Run err = nil, want non-nil after 503")
			}
			if got := effects.Load(); got != 1 {
				t.Fatalf("effect count = %d, want 1", got)
			}
		})
	}
}
