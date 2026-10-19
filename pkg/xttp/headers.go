package xttp

import (
	"reflect"
	"sort"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
)

// HelmetPreset is the base set of security headers. Config.Helmet then
// overrides it field by field.
type HelmetPreset int

const (
	// HelmetDefault is Fiber's helmet.ConfigDefault plus Cache-Control: no-store.
	HelmetDefault HelmetPreset = iota
	// HelmetStrict is for APIs that serve no HTML: deny-all CSP, X-Frame-Options
	// DENY, every Permissions-Policy feature off, two-year preloaded HSTS (sent
	// on TLS requests only).
	HelmetStrict
)

// Values HelmetStrict sets, exported so tests and consumers can compare.
const (
	StrictCSP               = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	StrictPermissionsPolicy = "accelerometer=(), ambient-light-sensor=(), autoplay=(), battery=(), camera=(), " +
		"display-capture=(), document-domain=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), " +
		"magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), " +
		"screen-wake-lock=(), sync-xhr=(), usb=(), web-share=(), xr-spatial-tracking=()"
	StrictHSTSMaxAge = 63072000 // two years, the preload list minimum
)

// helmetFor returns the preset with cfg's non-zero fields applied on top; a
// string field set to "-" removes that header.
func helmetFor(preset HelmetPreset, cfg helmet.Config) helmet.Config {
	out := helmet.ConfigDefault
	if preset == HelmetStrict {
		out.ContentSecurityPolicy = StrictCSP
		out.XFrameOptions = "DENY"
		out.CrossOriginResourcePolicy = "same-origin"
		out.PermissionPolicy = StrictPermissionsPolicy
		out.HSTSMaxAge = StrictHSTSMaxAge
		out.HSTSPreloadEnabled = true
		out.ReferrerPolicy = "no-referrer"
	}
	ov, dst := reflect.ValueOf(cfg), reflect.ValueOf(&out).Elem()
	for i := range dst.NumField() {
		f := dst.Field(i)
		switch f.Kind() {
		case reflect.String:
			if v := ov.Field(i).String(); v == "-" {
				f.SetString("")
			} else if v != "" {
				f.SetString(v)
			}
		default: // int, bool, func: set wins
			if !ov.Field(i).IsZero() {
				f.Set(ov.Field(i))
			}
		}
	}
	return out
}

// headerSet is the static part of every response, resolved once at New.
type headerSet struct {
	static [][2]string          // helmet headers plus Config.Headers, sorted by name
	extra  [][2]string          // Config.Headers only (set at the fasthttp layer as well)
	hsts   string               // Strict-Transport-Security value, sent on TLS requests
	next   func(fiber.Ctx) bool // helmet.Config.Next
}

func newHeaderSet(h helmet.Config, extra map[string]string) *headerSet {
	csp := fiber.HeaderContentSecurityPolicy
	if h.CSPReportOnly {
		csp = fiber.HeaderContentSecurityPolicyReportOnly
	}
	m := map[string]string{
		fiber.HeaderXXSSProtection:                h.XSSProtection,
		fiber.HeaderXContentTypeOptions:           h.ContentTypeNosniff,
		fiber.HeaderXFrameOptions:                 h.XFrameOptions,
		csp:                                       h.ContentSecurityPolicy,
		fiber.HeaderReferrerPolicy:                h.ReferrerPolicy,
		fiber.HeaderPermissionsPolicy:             h.PermissionPolicy,
		"Cross-Origin-Embedder-Policy":            h.CrossOriginEmbedderPolicy,
		"Cross-Origin-Opener-Policy":              h.CrossOriginOpenerPolicy,
		fiber.HeaderCrossOriginResourcePolicy:     h.CrossOriginResourcePolicy,
		"Origin-Agent-Cluster":                    h.OriginAgentCluster,
		fiber.HeaderXDNSPrefetchControl:           h.XDNSPrefetchControl,
		fiber.HeaderXDownloadOptions:              h.XDownloadOptions,
		fiber.HeaderXPermittedCrossDomainPolicies: h.XPermittedCrossDomain,
	}
	ex := map[string]string{fiber.HeaderCacheControl: "no-store"}
	for k, v := range extra {
		ex[k] = v
	}
	hs := &headerSet{next: h.Next}
	for k, v := range ex {
		m[k] = v
		if v != "" && v != "-" {
			hs.extra = append(hs.extra, [2]string{k, v})
		}
	}
	for k, v := range m {
		if v != "" && v != "-" {
			hs.static = append(hs.static, [2]string{k, v})
		}
	}
	sort.Slice(hs.static, func(i, j int) bool { return hs.static[i][0] < hs.static[j][0] })
	sort.Slice(hs.extra, func(i, j int) bool { return hs.extra[i][0] < hs.extra[j][0] })
	if h.HSTSMaxAge > 0 {
		hs.hsts = "max-age=" + strconv.Itoa(h.HSTSMaxAge)
		if !h.HSTSExcludeSubdomains {
			hs.hsts += "; includeSubDomains"
		}
		if h.HSTSPreloadEnabled {
			hs.hsts += "; preload"
		}
	}
	return hs
}

// set writes every static header; handlers that run later may override them.
func (hs *headerSet) set(c fiber.Ctx) {
	for _, h := range hs.static {
		c.Set(h[0], h[1])
	}
	if hs.hsts != "" && c.Secure() {
		c.Set(fiber.HeaderStrictTransportSecurity, hs.hsts)
	}
}

// middleware is the helmet replacement: same headers, plus Config.Headers,
// honouring helmet.Config.Next.
func (hs *headerSet) middleware(c fiber.Ctx) error {
	if hs.next == nil || !hs.next(c) {
		hs.set(c)
	}
	return c.Next()
}
