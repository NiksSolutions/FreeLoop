package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// credentialParam matches a query parameter that holds a credential by its
// whole name or after a dash or underscore; credentialCamel by the last word
// of a camelCase name, such as exaApiKey.
var (
	credentialParam = regexp.MustCompile(`(?i)^(.*[_-])?(key|apikey|token|secret|password|passwd|pwd|auth|sig|signature)$`)
	credentialCamel = regexp.MustCompile(`[a-z0-9](Key|KEY|Token|TOKEN|Secret|SECRET|Password|PASSWORD|Passwd|Pwd|Auth|AUTH|Sig|Signature)$`)
)

// CredentialParam reports a query parameter whose name says it holds a
// credential.
func CredentialParam(name string) bool {
	return credentialParam.MatchString(name) || credentialCamel.MatchString(name)
}

// CredentialsIn returns the credential-like values in an address's query and
// userinfo. A {key} or {query} placeholder is not one.
func CredentialsIn(u *url.URL) []string {
	var out []string
	if u.User != nil {
		if p, ok := u.User.Password(); ok && p != "" {
			out = append(out, p)
		}
		if n := u.User.Username(); n != "" {
			out = append(out, n)
		}
	}
	for name, values := range u.Query() {
		if CredentialParam(name) {
			for _, v := range values {
				if v != "" && v != "{key}" && v != "{query}" {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

// CheckAddress refuses an address holding a credential (SEC-004): config
// files are not a place for one, a backup carries them, and it would show in
// errors. The error does not repeat the address.
func CheckAddress(what, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s is not an address", what)
	}
	if u.User != nil {
		return fmt.Errorf("%s holds a user name or password; those go in the Keychain, not in the address", what)
	}
	if len(CredentialsIn(u)) > 0 {
		return fmt.Errorf("%s holds a key in its query; enter the key separately so it stays in the Keychain", what)
	}
	return nil
}
