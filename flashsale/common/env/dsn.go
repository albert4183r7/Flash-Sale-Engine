package env

import (
	"fmt"
	"net/url"
	"strings"
)

// PostgresDSN returns the PostgreSQL connection string.
//
// POSTGRES_URL is honoured when set, which is what CI and managed databases
// use. Otherwise the DSN is assembled from its parts. Assembling it here rather
// than writing a full URL into configuration files keeps credentials out of
// committed files, and lets Docker Compose point a service at another host by
// overriding POSTGRES_HOST alone: Compose passes env_file values through
// literally, so a "${POSTGRES_USER}" inside a URL would never be expanded.
func PostgresDSN() (string, error) {
	if dsn := Get("POSTGRES_URL", ""); dsn != "" {
		return dsn, nil
	}

	host := Get("POSTGRES_HOST", "localhost")
	port := Get("POSTGRES_PORT", "5432")
	database := strings.TrimPrefix(Get("POSTGRES_DB", "flashsale"), "/")
	if host == "" || database == "" {
		return "", fmt.Errorf("set POSTGRES_URL, or POSTGRES_HOST and POSTGRES_DB")
	}

	dsn := url.URL{
		Scheme:   "postgres",
		Host:     joinHostPort(host, port),
		Path:     "/" + database,
		User:     userInfo(Get("POSTGRES_USER", "postgres"), Get("POSTGRES_PASSWORD", "")),
		RawQuery: "sslmode=" + url.QueryEscape(Get("POSTGRES_SSLMODE", "disable")),
	}
	return dsn.String(), nil
}

// AMQPURL returns the RabbitMQ connection string, honouring RABBITMQ_URL when
// set and otherwise assembling it from its parts, for the same reasons as
// PostgresDSN.
func AMQPURL() (string, error) {
	if raw := Get("RABBITMQ_URL", ""); raw != "" {
		return raw, nil
	}

	host := Get("RABBITMQ_HOST", "localhost")
	if host == "" {
		return "", fmt.Errorf("set RABBITMQ_URL, or RABBITMQ_HOST")
	}

	amqpURL := url.URL{
		Scheme: "amqp",
		Host:   joinHostPort(host, Get("RABBITMQ_PORT", "5672")),
		// A leading slash denotes the default vhost.
		Path: "/" + strings.TrimPrefix(Get("RABBITMQ_VHOST", ""), "/"),
		User: userInfo(Get("RABBITMQ_USER", ""), Get("RABBITMQ_PASSWORD", "")),
	}
	return amqpURL.String(), nil
}

// userInfo builds the credentials portion of a URL. It returns nil when no
// username is configured, which leaves the credentials out of the URL entirely
// so the client applies its own defaults.
func userInfo(username, password string) *url.Userinfo {
	if username == "" {
		return nil
	}
	if password == "" {
		return url.User(username)
	}
	return url.UserPassword(username, password)
}

// joinHostPort joins a host and port, leaving the host alone when no port
// is configured.
func joinHostPort(host, port string) string {
	if port == "" {
		return host
	}
	return host + ":" + port
}
