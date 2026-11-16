package api

import (
	"fmt"
	"net/url"
	"strings"
)

// A download URL can carry the response headers its caller wants the download
// to send: file_urls folds them into the query, and the download route puts
// them on the response. The instruction therefore travels inside the URL, so a
// client that has never heard of termcp — curl, a browser, a pasted link —
// still gets the headers, and no second channel has to agree on how to pass
// them.
//
// Each header becomes ?h.<Name>=<value>. The name rides in the parameter's KEY
// rather than in a "Name: value" list, so it arrives exactly as written and the
// value is percent-escaped by url.Values like any other parameter.
const responseHeaderParamPrefix = "h."

// AppendResponseHeaders folds headers into q as the parameters above. A name
// that is not an HTTP token or a value carrying a line break is refused rather
// than encoded: both would either be dropped silently by net/http or open the
// response to header injection, and a URL that quietly means something other
// than what was asked for is worse than no URL.
func AppendResponseHeaders(q url.Values, headers map[string]string) error {
	for name, value := range headers {
		if !validHeaderName(name) {
			return fmt.Errorf("invalid response header name %q", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("response header %q contains a line break", name)
		}
		q.Add(responseHeaderParamPrefix+name, value)
	}
	return nil
}

// ResponseHeadersFromQuery is the inverse: it picks those parameters out of a
// query, under the names as written. A repeated name keeps all its values.
func ResponseHeadersFromQuery(q url.Values) (map[string][]string, error) {
	var out map[string][]string
	for key, values := range q {
		name, ok := strings.CutPrefix(key, responseHeaderParamPrefix)
		if !ok {
			continue
		}
		if !validHeaderName(name) {
			return nil, fmt.Errorf("invalid response header name %q", name)
		}
		for _, v := range values {
			if strings.ContainsAny(v, "\r\n") {
				return nil, fmt.Errorf("response header %q contains a line break", name)
			}
			if out == nil {
				out = map[string][]string{}
			}
			out[name] = append(out[name], v)
		}
	}
	return out, nil
}

// validHeaderName reports whether s is an RFC 7230 token, the only shape a
// header name may take. net/http drops a malformed name silently, which would
// turn a typo into "no header, no complaint".
func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	const punctuation = "!#$%&'*+-.^_`|~"
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte(punctuation, c) >= 0:
		default:
			return false
		}
	}
	return true
}
