//go:build linux

package sandbox

var platformNetworkReadPaths = []string{
	"/etc/resolv.conf",
	"/etc/hosts",
	"/etc/nsswitch.conf",
	"/etc/services",
	"/etc/ssl/certs",
	"/etc/ssl/openssl.cnf",
	"/etc/ca-certificates",
	"/etc/pki/tls/certs",
	"/etc/pki/ca-trust",
	"/usr/share/ca-certificates",
}
