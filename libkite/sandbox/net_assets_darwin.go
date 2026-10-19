//go:build darwin

package sandbox

var platformNetworkReadPaths = []string{
	"/etc/resolv.conf",
	"/etc/hosts",
	"/etc/services",
	"/etc/ssl/certs",
	"/etc/ssl/cert.pem",
	"/etc/ssl/openssl.cnf",
	"/private/etc/resolv.conf",
	"/private/etc/hosts",
	"/private/etc/services",
	"/private/etc/ssl/certs",
	"/private/etc/ssl/cert.pem",
	"/private/etc/ssl/openssl.cnf",
}
