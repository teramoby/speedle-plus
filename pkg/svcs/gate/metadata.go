// Licensed under the Universal Permissive License (UPL) Version 1.0.
package gate

import "strings"

// FilterMetadata returns an independent, lower-cased copy suitable for both
// REST headers and gRPC header metadata. Connection-nominated headers, cookies,
// framing and service-owned headers are excluded. Keys use the gRPC text-key
// alphabet; values must be printable ASCII. Binary gRPC metadata is not forwarded.
func FilterMetadata(values map[string][]string) map[string][]string {
	blocked := map[string]bool{
		"connection": true, "proxy-connection": true, "keep-alive": true,
		"proxy-authenticate": true, "proxy-authorization": true,
		"te": true, "trailer": true, "transfer-encoding": true, "upgrade": true,
		"set-cookie": true, "content-length": true, "content-type": true,
		"cache-control": true, "retry-after": true, "x-speedle-decision-source": true,
		"speedle-principals": true, "www-authenticate": true, "date": true, "server": true,
	}
	for key, entries := range values {
		if strings.EqualFold(key, "connection") {
			for _, entry := range entries {
				for _, name := range strings.Split(entry, ",") {
					blocked[strings.ToLower(strings.TrimSpace(name))] = true
				}
			}
		}
	}
	var result map[string][]string
	for key, entries := range values {
		key = strings.ToLower(key)
		if blocked[key] || strings.HasPrefix(key, "grpc-") || strings.HasSuffix(key, "-bin") || !validMetadataKey(key) {
			continue
		}
		for _, value := range entries {
			if !validMetadataValue(value) {
				continue
			}
			if result == nil {
				result = make(map[string][]string)
			}
			result[key] = append(result[key], value)
		}
	}
	return result
}

func validMetadataKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func validMetadataValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < ' ' || value[i] > '~' {
			return false
		}
	}
	return true
}
