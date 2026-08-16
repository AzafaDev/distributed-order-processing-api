package httpx

import (
	"net"
	"net/http"
)

// ClientIP ngambil IP pemanggil dari r.RemoteAddr, tanpa nomor port.
//
// CATATAN buat scaling: ini IP koneksi langsung. Begitu ada nginx/LB/CDN di
// depan API, semua request bakal keliatan datang dari IP proxy, jadi rate limit
// per-IP nge-lock semua orang sekaligus. Solusinya baca X-Forwarded-For (chi
// punya middleware.RealIP), tapi header itu gampang dipalsuin client — cuma
// pasang kalau proxy di depan dijamin selalu nimpa header-nya.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
