// Package api contains code generated from the OpenAPI v1 spec at
// docs/openapi/stig-manager.yaml.
//
// The generated types and chi-based router live in types.gen.go and
// server.gen.go. Run `go generate ./internal/api` from the api/ directory
// (or use the helper script at scripts/gen.sh) to regenerate them.
//
// oapi-codegen also emits an `Unimplemented` value type that satisfies
// every ServerInterface method with a 501 response. Real handlers embed
// it and override individual methods as features land.
package api

//go:generate go tool oapi-codegen -config types-config.yaml ../../../docs/openapi/stig-manager.yaml
//go:generate go tool oapi-codegen -config server-config.yaml ../../../docs/openapi/stig-manager.yaml
