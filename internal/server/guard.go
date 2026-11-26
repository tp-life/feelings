package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// LocalOnly 保护只监听在本机回环地址上的工作台：
//   - 校验 Host 必须是 localhost / 127.0.0.1 / ::1，防御 DNS rebinding
//     （否则恶意网页可把自己的域名解析到 127.0.0.1 后读取 API 响应）；
//   - 对会产生副作用的请求（非 GET/HEAD/OPTIONS），若浏览器带了 Origin 或
//     Sec-Fetch-Site，则必须来自同源页面，防御跨站请求伪造（删项目、触发付费生成等）。
//
// 没有 Origin / Sec-Fetch-Site 的请求（curl、脚本）不受第二条限制。
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden: host not allowed", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				http.Error(w, "forbidden: cross-site request", http.StatusForbidden)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) || !isLoopbackHost(u.Host) {
					http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
