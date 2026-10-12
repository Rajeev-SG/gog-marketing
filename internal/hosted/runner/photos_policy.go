package runner

import "net/http"

func nativePhotosLibraryBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "photos_list", Service: "photos", hosts: []string{"photoslibrary.googleapis.com"}, scopes: []string{"photoslibrary.readonly.appcreateddata"}, methods: []string{http.MethodGet}, paths: exactPaths("/v1/mediaItems", "")},
		{Name: "photos_search", Service: "photos", hosts: []string{"photoslibrary.googleapis.com"}, scopes: []string{"photoslibrary.readonly.appcreateddata"}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/mediaItems:search", "")},
		{Name: "photos_get", Service: "photos", hosts: []string{"photoslibrary.googleapis.com"}, scopes: []string{"photoslibrary.readonly.appcreateddata"}, methods: []string{http.MethodGet}, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			id, _ := args["media_id"].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid app-created media ID")
			}

			return []string{"/v1/mediaItems/" + id}, nil
		}},
	}
}
