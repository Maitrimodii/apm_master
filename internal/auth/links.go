package auth

import "net/url"

// buildAppLink builds a UI link such as the password-reset or invite page.
func buildAppLink(baseURL, path string, params map[string]string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}

	parsed.Path = path
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}
