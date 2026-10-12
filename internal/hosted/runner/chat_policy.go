package runner

import "net/http"

func nativeChatBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "chat_spaces_list", Service: "chat", hosts: []string{"chat.googleapis.com"}, scopes: []string{"chat.spaces.readonly", "chat.spaces"}, methods: []string{http.MethodGet}, paths: exactPaths("/v1/spaces", "")},
		{Name: "chat_spaces_create", Service: "chat", Write: true, hosts: []string{"chat.googleapis.com"}, scopes: []string{"chat.spaces"}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/spaces:setup", "")},
		{Name: "chat_messages_send", Service: "chat", Write: true, hosts: []string{"chat.googleapis.com"}, scopes: []string{"chat.messages.create"}, methods: []string{http.MethodPost}, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			name, _ := args["space_name"].(string)
			if !meetingSpaceName.MatchString(name) {
				return nil, invalidf("invalid Chat space name")
			}

			return []string{"/v1/" + name + "/messages"}, nil
		}},
	}
}
