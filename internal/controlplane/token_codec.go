package controlplane

func MarshalOAuthToken(token OAuthToken) ([]byte, error) {
	return marshalToken(token)
}
