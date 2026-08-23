// Package e2e drives the flash sale end to end.
//
// The tests build the three service binaries and run them as real processes
// against real PostgreSQL, Redis and RabbitMQ, then exercise the system through
// the API gateway's public HTTP API. Running the actual binaries means the
// tests also cover configuration loading, startup ordering and the wiring
// between services, none of which an in-process harness would exercise.
//
// They are skipped unless POSTGRES_URL, REDIS_ADDR and RABBITMQ_URL are set and
// reachable, so "go test ./..." stays usable without infrastructure.
package e2e
