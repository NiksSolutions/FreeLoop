package mcp

import (
	"errors"
	"net/url"
	"strings"
)

// An HTTP server's errors repeat its address: Go's *url.Error names the
// request's URL. The address's query and userinfo are where a key would be,
// so they are taken out before an error leaves the package for a transcript,
// the model, the MCP page or the log (SEC-004). config.MCPServer.Validate
// refuses such an address; this is for one that got past it.

// scrubbed is an error with its address cleaned. It answers errors.Is as the
// error it replaces but does not unwrap to it, so its address cannot be
// printed by way of errors.As.
type scrubbed struct {
	msg string
	err error
}

func (e *scrubbed) Error() string        { return e.msg }
func (e *scrubbed) Is(target error) bool { return errors.Is(e.err, target) }

// cleanError returns err with address's query and userinfo, and those of any
// *url.Error's URL in it, removed from its text.
func cleanError(err error, address string) error {
	if err == nil {
		return nil
	}
	msg := cleanText(err.Error(), address)
	var ue *url.Error
	if errors.As(err, &ue) {
		msg = cleanText(msg, ue.URL)
	}
	if msg == err.Error() {
		return err
	}
	return &scrubbed{msg: msg, err: err}
}

// cleanText removes address's query and userinfo from s.
func cleanText(s, address string) string {
	u, err := url.Parse(address)
	if err != nil || address == "" {
		return s
	}
	if u.RawQuery != "" {
		s = strings.ReplaceAll(s, "?"+u.RawQuery, "")
	}
	if u.User != nil {
		s = strings.ReplaceAll(s, u.User.String()+"@", "")
	}
	return s
}

// clean cleans an error from the server id with its configured address.
func (m *Manager) clean(id string, err error) error {
	if err == nil {
		return nil
	}
	m.mu.Lock()
	address := ""
	if s, ok := m.servers[id]; ok {
		address = s.address
	}
	m.mu.Unlock()
	return cleanError(err, address)
}
