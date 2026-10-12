package runner

import (
	"net/http"
	"regexp"
)

var adSenseAccountName = regexp.MustCompile(`^accounts/[A-Za-z0-9_-]{1,256}$`)

func nativeAdSenseAccountBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "adsense_accounts_list", Service: "adsense", hosts: []string{"adsense.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"adsense.readonly"}, paths: exactPaths("/v2/accounts", "")},
		{Name: "adsense_accounts_get", Service: "adsense", hosts: []string{"adsense.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"adsense.readonly"}, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			name, _ := args["account_name"].(string)
			if !adSenseAccountName.MatchString(name) {
				return nil, invalidf("invalid AdSense account name")
			}

			return []string{"/v2/" + name}, nil
		}},
	}
}
