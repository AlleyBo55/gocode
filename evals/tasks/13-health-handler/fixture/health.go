// Package health exposes the service health endpoint.
package health

import "net/http"

// HealthHandler reports whether the service is up.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
}
