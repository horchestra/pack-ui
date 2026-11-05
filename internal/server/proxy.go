package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// The SPA never talks to the operator itself: the browser knows pack-ui only, pack-ui proxies /api/op/* to
// the operator's /api/* with the user in X-Pack-User (К4) — the operator's address stays out of the browser,
// and the user is asserted by the backend, not by the page. SSE endpoints stream through the reverse proxy
// (a flush after every write), Last-Event-ID is a header like any other.
func (s *Server) proxy() http.Handler {
	target, err := url.Parse(s.cfg.Operator)
	if err != nil {
		panic("operator: " + err.Error())
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			// the escaped path is kept: a release id is one segment with the path's slashes encoded
			// (%2F), and the operator's router matches segments escaped
			esc := "/api/" + strings.TrimPrefix(r.In.URL.EscapedPath(), "/api/op/")
			dec, err := url.PathUnescape(esc)
			if err != nil {
				dec = esc
			}
			r.Out.URL.Path = dec
			r.Out.URL.RawPath = esc
			// the session's user, with pack-ui's service token — the operator acts as them (C9)
			if user := s.user(r.In); len(user) > 0 {
				r.Out.Header.Set("X-Pack-User", user)
			}
			if len(s.cfg.OperatorToken) > 0 {
				r.Out.Header.Set("Authorization", "Bearer "+s.cfg.OperatorToken)
			}
			r.Out.Header.Del("Cookie") // the session is pack-ui's, the operator never sees it
			r.Out.Host = target.Host
		},
		// SSE: no buffering, no timeout of the whole response
		FlushInterval: -1,
	}
	return rp
}
