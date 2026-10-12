package runner

import (
	"net/http"

	"github.com/openclaw/gogcli/internal/hosted/native"
)

func nativeYouTubePlaylistBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "youtube_playlists_list", Service: "youtube", hosts: []string{"youtube.googleapis.com"}, scopes: []string{"youtube.readonly", "youtube"}, methods: []string{http.MethodGet}, paths: exactPaths("/youtube/v3/playlists", "")},
		{Name: "youtube_playlists_create", Service: "youtube", Write: true, hosts: []string{"youtube.googleapis.com"}, scopes: []string{"youtube"}, methods: []string{http.MethodPost}, paths: exactPaths("/youtube/v3/playlists", "")},
		{Name: "youtube_playlists_delete", Service: "youtube", Write: true, destructive: true, hosts: []string{"youtube.googleapis.com"}, scopes: []string{"youtube"}, methods: []string{http.MethodDelete}, paths: exactPaths("/youtube/v3/playlists", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			id, _ := args["playlist_id"].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid playlist ID")
			}

			return []string{"/youtube/v3/playlists"}, nil
		}},
	}
}

func admitYouTubePlaylistQuery(invocation *ToolInvocation, request *http.Request) error {
	if invocation.Name == "youtube_playlists_list" && request.URL.Query().Get("mine") != "true" {
		return native.ErrDenied
	}

	if invocation.Name == "youtube_playlists_delete" {
		id, _ := invocation.Arguments["playlist_id"].(string)
		if request.URL.Query().Get("id") != id {
			return native.ErrDenied
		}
	}

	return nil
}
