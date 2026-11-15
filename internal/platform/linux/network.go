//go:build linux

package linux

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// The sandbox's network for localhost and the listed hosts (ENV-009, ENV-010,
// OQ-037). On macOS a sandboxed command reaches this computer's loopback, a dev
// server's or the egress proxy's. Under bubblewrap its network is a namespace of
// its own, whose loopback holds only what the sandbox starts, so:
//
//   - with pasta (package passt) installed, the command runs in pasta's namespace
//     and pasta forwards the host's listening loopback TCP ports into it, polling
//     each second as servers come and go (-T auto). Its interface stays down and
//     unconfigured, so nothing beyond the loopback is reached, and the command
//     cannot change that: it has no capabilities in the namespace that owns the
//     network. The egress proxy is one of the forwarded ports.
//   - without it, the loopback is the sandbox's own. The egress proxy is then
//     reached through its Unix socket, mounted in, and a bridge inside listening
//     at the port its variables name: the app's own binary (RunSandboxBridge).
//
// Either way the namespace is private, so the host's abstract sockets, D-Bus's
// among them, stay out of reach. So it is for all of the network, the setup, run
// and archive commands': pasta's namespace with its interface configured
// (pastaArgs), or without pasta, or offline, the sandbox's own loopback and
// nothing else.

// pastaPaths are where pasta is looked for: absolute, like bwrap's.
var pastaPaths = []string{"/usr/bin/pasta", "/bin/pasta", "/usr/local/bin/pasta"}

// Where the bridge's binary and the proxy's socket are inside the sandbox, whose
// /run is its own.
const (
	bridgeExe    = "/run/freeloop/bridge"
	bridgeSocket = "/run/freeloop/egress.sock"
)

// netPlan is how a command reaches this computer's loopback.
type netPlan struct {
	// pasta is pasta and its options, to run bwrap in.
	pasta []string
	// mounts are bwrap's options for what the bridge needs.
	mounts []string
	// bridge goes before the command, to start the bridge.
	bridge []string
}

// network plans c's network. None is a namespace of its own with nothing in it.
func (b Bubblewrap) network(c environment.Confinement) (netPlan, error) {
	switch c.Network {
	case environment.NetworkAll:
		// The host's own namespace would open its abstract sockets, X11's among
		// them, so all of the network is pasta's too. Without pasta, or offline,
		// the command keeps a loopback of its own, as localhost does: it fails
		// closed, and installing passt gives it the network.
		if !b.noPasta {
			if p := pastaArgs(true); p != nil {
				return netPlan{pasta: p, mounts: resolverMounts(p)}, nil
			}
		}
		return netPlan{}, nil
	case environment.NetworkLocalhost:
	default:
		return netPlan{}, nil
	}
	if !b.noPasta {
		if p := pastaArgs(false); p != nil {
			return netPlan{pasta: p}, nil
		}
	}
	if c.ProxySocket == "" || c.ProxyPort <= 0 {
		return netPlan{}, nil
	}
	if fi, err := os.Stat(c.ProxySocket); err != nil || fi.Mode().Type() != os.ModeSocket {
		return netPlan{}, errors.New("the egress proxy for this session's listed hosts is not running")
	}
	self, err := os.Executable()
	if err != nil {
		return netPlan{}, errors.New("the sandbox cannot find the app's own program, which bridges to the egress proxy")
	}
	mounts, exe := bridgeMounts(self, os.Getenv("APPDIR"))
	return netPlan{
		mounts: append(mounts, "--bind", c.ProxySocket, bridgeSocket),
		bridge: []string{exe, environment.SandboxBridgeArg, "-listen", "127.0.0.1:" + strconv.Itoa(c.ProxyPort), "-socket", bridgeSocket, "--"},
	}, nil
}

// bridgeMounts mounts the app's program for the bridge and says where it runs from.
// From an AppImage the program needs the libraries beside it, under a /tmp the
// sandbox replaces, so the AppImage's whole folder (appDir) is mounted read-only at
// its own path and the program runs from there; elsewhere the program alone is
// mounted at bridgeExe.
func bridgeMounts(self, appDir string) ([]string, string) {
	if appDir != "" && filepath.IsAbs(appDir) && within(self, appDir) {
		return []string{"--ro-bind", appDir, appDir}, self
	}
	return []string{"--ro-bind", self, bridgeExe}, bridgeExe
}

// pastaArgs runs a command in a namespace whose loopback has the host's listening
// TCP ports, or is nil where pasta is not installed or cannot start. Pinned to
// pasta 2024-02-20, Ubuntu 24.04's, which later versions keep to:
//
//   - -f, so pasta waits for the command and exits with its status, a terminal or
//     not; -q, so it says nothing on the command's output;
//   - -t none -u none: nothing in the sandbox is offered to the host;
//   - -T auto: the host's listening TCP ports, on the sandbox's loopback;
//   - -U none: no UDP; a host's name server often listens on loopback, and asking
//     it names is a way to send data out, which macOS's sandbox does not offer;
//   - no --config-net, so pasta's interface stays down, with no address or route.
//
// With all of the network (all) the namespace is pasta's still, so the host's
// abstract sockets stay out of reach, but its interface is configured as the
// host's (--config-net) and every port is forwarded both ways: -t auto and -u auto
// offer what the command listens on to the host, the Run button's dev server, and
// -T auto and -U auto give it the host's loopback. Its name server is pasta's,
// which passes lookups to the host's (--dns-forward, resolverMounts).
//
// That version will not start without a default route, so offline the sandbox
// keeps its own loopback; and it warns on the command's output about a family with
// none, so it is told to use only the family that has one.
func pastaArgs(all bool) []string {
	pasta := firstFile(pastaPaths)
	if pasta == "" {
		return nil
	}
	if fi, err := os.Stat("/dev/net/tun"); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	v4, v6 := defaultRoute4(), defaultRoute6()
	args := []string{pasta, "-q", "-f"}
	switch {
	case v4 && !v6:
		args = append(args, "-4")
	case v6 && !v4:
		args = append(args, "-6")
	case !v4 && !v6:
		return nil
	}
	if all {
		args = append(args, "--config-net", "-t", "auto", "-u", "auto", "-T", "auto", "-U", "auto")
		if v4 {
			args = append(args, "--dns-forward", sandboxResolver)
		}
		return append(args, "--")
	}
	return append(args, "-t", "none", "-u", "none", "-T", "auto", "-U", "none", "--")
}

// sandboxResolver is the name server a command with all of the network is given:
// pasta answers there with the host's own, which may listen on a loopback the
// command does not share, or be named by a file under /run, which it does not see.
const sandboxResolver = "169.254.1.1"

// resolverMounts put a resolv.conf naming sandboxResolver where the host's is, for
// pasta's options p.
func resolverMounts(p []string) []string {
	if !slices.Contains(p, "--dns-forward") {
		return nil
	}
	dest, err := filepath.EvalSymlinks("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(base, "freeloop")
	file := filepath.Join(dir, "sandbox-resolv.conf")
	const text = "nameserver " + sandboxResolver + "\n"
	if b, err := os.ReadFile(file); err != nil || string(b) != text {
		if environment.PrivateCache(dir) != nil || environment.WriteCacheFile(file, []byte(text)) != nil {
			return nil
		}
	}
	return []string{"--ro-bind", file, dest}
}

// defaultRoute4 reports an IPv4 default route in /proc/net/route, whose fields
// are the interface, destination, gateway and flags, in hex.
func defaultRoute4() bool {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		if flags, err := strconv.ParseUint(f[3], 16, 32); err == nil && flags&rtfUp != 0 {
			return true
		}
	}
	return false
}

// defaultRoute6 reports an IPv6 default route in /proc/net/ipv6_route: the
// destination, its prefix length, then source, its length, next hop, metric,
// references, use, flags and interface. The kernel's own unreachable default on
// lo does not count.
func defaultRoute6() bool {
	b, err := os.ReadFile("/proc/net/ipv6_route")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || f[0] != strings.Repeat("0", 32) || f[1] != "00" || f[9] == "lo" {
			continue
		}
		if flags, err := strconv.ParseUint(f[8], 16, 32); err == nil && flags&rtfUp != 0 && flags&rtfReject == 0 {
			return true
		}
	}
	return false
}

const (
	rtfUp     = 0x1
	rtfReject = 0x200
)

// firstFile is the first regular file of paths, or "".
func firstFile(paths []string) string {
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}
