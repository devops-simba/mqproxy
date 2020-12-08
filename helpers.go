package main

import (
	"net/url"
	"strings"
)

func GetOptionalBool(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	} else {
		return *value
	}
}

func ParseUrl(s, defaultScheme string) (*url.URL, error) {
	if strings.Index(s, "://") == -1 {
		if strings.IndexAny(s, ":/.") == -1 {
			s += "://"
		} else {
			s = defaultScheme + "://" + s
		}
	}

	return url.Parse(s)
}
func GetUrlHostname(u *url.URL) string {
	hostname := u.Hostname()
	if hostname == "" {
		return "0.0.0.0"
	}
	return hostname
}
func GetUrlPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http", "ws":
			return "80"
		case "https", "wss":
			return "443"
		case "mqtt":
			return "1883"
		case "mqtts":
			return "8883"
		default:
			return ""
		}
	}
	return port
}
func GetUrlDirPath(u *url.URL) string {
	result := u.Path
	if !strings.HasSuffix(result, "/") {
		result += "/"
	}
	return result
}
