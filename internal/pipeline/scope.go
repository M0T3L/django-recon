package pipeline

import (
	"net"
	"net/url"
	"strings"
)

// ScopeEngine enforces target boundaries across pipeline stages.
type ScopeEngine struct {
	RootDomain     string
	AllowedCIDRs   []*net.IPNet
	ExcludedHosts  map[string]bool
	ExcludedTokens []string
}

// NewScopeEngine initializes a ScopeEngine for a given root domain target.
func NewScopeEngine(rootDomain string) *ScopeEngine {
	clean := strings.TrimSpace(strings.ToLower(rootDomain))
	clean = strings.TrimPrefix(clean, "http://")
	clean = strings.TrimPrefix(clean, "https://")
	clean = strings.Split(clean, "/")[0]
	clean = strings.Split(clean, ":")[0]

	return &ScopeEngine{
		RootDomain:     clean,
		ExcludedHosts:  make(map[string]bool),
		ExcludedTokens: []string{"logout", "signout", "delete", "destroy"},
	}
}

// IsInScope checks whether a hostname or URL belongs to the target scope.
func (s *ScopeEngine) IsInScope(target string) bool {
	if s == nil || s.RootDomain == "" {
		return true
	}

	target = strings.TrimSpace(strings.ToLower(target))
	if target == "" {
		return false
	}

	// Parse host if full URL passed
	host := target
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if parsed, err := url.Parse(target); err == nil {
			host = parsed.Hostname()
			// Exclude destructive URLs (e.g. /logout, /delete)
			pathLower := strings.ToLower(parsed.Path)
			for _, token := range s.ExcludedTokens {
				if strings.Contains(pathLower, token) {
					return false
				}
			}
		}
	} else {
		// Strip port if host:port
		if h, _, err := net.SplitHostPort(target); err == nil {
			host = h
		}
	}

	if s.ExcludedHosts[host] {
		return false
	}

	// Exact match or subdomain suffix match
	if host == s.RootDomain || strings.HasSuffix(host, "."+s.RootDomain) {
		return true
	}

	// IP scope check if target is IP
	if ip := net.ParseIP(host); ip != nil {
		for _, cidr := range s.AllowedCIDRs {
			if cidr.Contains(ip) {
				return true
			}
		}
	}

	return false
}
