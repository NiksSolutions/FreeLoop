//go:build linux

package linux

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// The sandbox's guarantees, tried from inside it (ENV-009, OQ-037). They need
// bubblewrap and user namespaces, which a container gives only when privileged:
//
//	make linux-test-sandbox
//
// sets FREELOOP_SANDBOX_TEST=1 and runs these there.
func sandboxed(t *testing.T) (home, project string, run func(c environment.Confinement, script string) (string, error)) {
	t.Helper()
	if os.Getenv("FREELOOP_SANDBOX_TEST") != "1" {
		t.Skip("set FREELOOP_SANDBOX_TEST=1 where bubblewrap can run (make linux-test-sandbox)")
	}
	base, _ := filepath.EvalSymlinks(t.TempDir())
	home = filepath.Join(base, "home")
	project = filepath.Join(home, "work", "project")
	for _, d := range []string{filepath.Join(home, ".ssh"), filepath.Join(project, ".git", "hooks"), filepath.Join(project, "src"), filepath.Join(home, "toolchain"), filepath.Join(home, ".cache", "proj")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(home, ".ssh", "id"), []byte("PRIVATE KEY"), 0o600)
	os.WriteFile(filepath.Join(home, "toolchain", "tool"), []byte("tool"), 0o644)
	os.WriteFile(filepath.Join(project, ".git", "config"), []byte("[core]"), 0o644)
	os.WriteFile(filepath.Join(project, "src", "main.go"), []byte("package main"), 0o644)
	run = func(c environment.Confinement, script string) (string, error) {
		return runIn(Bubblewrap{}, project, c, script)
	}
	return home, project, run
}

// runIn runs script confined by b, from project.
func runIn(b Bubblewrap, project string, c environment.Confinement, script string) (string, error) {
	argv, err := b.Confine(c, []string{"/bin/sh", "-c", script})
	if err != nil {
		return "", err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = project
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// The test binary is the bridge inside the sandbox, as the app's is, and what
// tries a Unix socket or serves a port from inside it, which the image has no
// tool for.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case environment.SandboxBridgeArg:
			os.Exit(environment.RunSandboxBridge(os.Args[2:]))
		case unixProbeArg:
			conn, err := net.DialTimeout("unix", os.Args[2], 2*time.Second)
			if err != nil {
				fmt.Println("refused:", err)
				os.Exit(1)
			}
			conn.Close()
			fmt.Println("connected")
			os.Exit(0)
		case serveArg:
			ln, err := net.Listen("tcp", os.Args[2])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "in the sandbox") }))
			time.Sleep(10 * time.Second)
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

const (
	unixProbeArg = "unix-probe"
	serveArg     = "serve"
)

// withSelf mounts the test binary into the sandbox, for TestMain's helpers.
func withSelf(t *testing.T, c environment.Confinement) (environment.Confinement, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c.Read = append(slices.Clone(c.Read), self)
	return c, self
}

// A security review (2026-11-13) reached sockets of programs outside the sandbox:
// with all of the network, the host's abstract sockets (X11's, which takes
// keystrokes; D-Bus's), since that shared the host's network namespace; and in
// every mode, a path socket outside the folders the sandbox empties, such as LXD's
// or the Nix daemon's, which mounting the root read-only does not close. None may
// answer, in any network mode; the project's own socket still does.
func TestTheSandboxKeepsOtherProgramsSocketsOut(t *testing.T) {
	home, project, _ := sandboxed(t)
	abstract := "@freeloop-probe-" + strconv.Itoa(os.Getpid())
	al, err := net.Listen("unix", abstract)
	if err != nil {
		t.Fatal(err)
	}
	defer al.Close()
	others := []string{abstract}
	// /var/tmp, and a folder the sandbox shows as it is: the real home folder,
	// which is not the sandbox's.
	realHome, _ := os.UserHomeDir()
	var shown string
	for _, dir := range []string{"/var/tmp", realHome} {
		d, err := os.MkdirTemp(dir, "flprobe")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(d)
		sock := filepath.Join(d, "s")
		l, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		others = append(others, sock)
		shown = d
	}
	// What else is in a socket's folder is still there, read-only.
	os.WriteFile(filepath.Join(shown, "notes.txt"), []byte("beside the socket"), 0o644)
	os.Mkdir(filepath.Join(shown, "sub"), 0o755)
	os.WriteFile(filepath.Join(shown, "sub", "deeper.txt"), []byte("deeper"), 0o644)
	os.Symlink("notes.txt", filepath.Join(shown, "link"))
	own := filepath.Join(project, "own.sock")
	ol, err := net.Listen("unix", own)
	if err != nil {
		t.Fatal(err)
	}
	defer ol.Close()
	for _, mode := range []struct {
		name string
		b    Bubblewrap
		n    environment.Network
	}{
		{"none", Bubblewrap{}, environment.NetworkNone},
		{"localhost", Bubblewrap{}, environment.NetworkLocalhost},
		{"localhost without pasta", Bubblewrap{noPasta: true}, environment.NetworkLocalhost},
		{"all", Bubblewrap{}, environment.NetworkAll},
	} {
		c, self := withSelf(t, base(home, project))
		c.Network = mode.n
		for _, sock := range others {
			if out, err := runIn(mode.b, project, c, self+" "+unixProbeArg+" '"+sock+"'"); err == nil {
				t.Errorf("%s: %s answered: %q", mode.name, sock, out)
			}
		}
		if out, err := runIn(mode.b, project, c, self+" "+unixProbeArg+" '"+own+"'"); err != nil {
			t.Errorf("%s: the project's own socket: %q, %v", mode.name, out, err)
		}
	}

	c, self := withSelf(t, base(home, project))
	if out, err := runIn(Bubblewrap{}, project, c, "cd "+shown+" && cat notes.txt sub/deeper.txt link"); out != "beside the socketdeeperbeside the socket" {
		t.Errorf("the rest of a socket's folder: %q, %v", out, err)
	}
	if out, err := runIn(Bubblewrap{}, project, c, "echo x > "+filepath.Join(shown, "notes.txt")); err == nil {
		t.Errorf("a socket's folder became writable: %q", out)
	}
	// A socket gone by the time the command starts does not stop it.
	gone := filepath.Join(shown, "gone.sock")
	gl, err := net.Listen("unix", gone)
	if err != nil {
		t.Fatal(err)
	}
	argv, err := Bubblewrap{}.Confine(c, []string{self, unixProbeArg, gone})
	if err != nil {
		t.Fatal(err)
	}
	gl.Close()
	os.Remove(gone)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = project
	if out, _ := cmd.CombinedOutput(); !strings.Contains(string(out), "refused:") {
		t.Errorf("a socket that went away: %s", out)
	}
}

// ENV-009: with all of the network the command has its own namespace through
// pasta, and still has a route beyond this computer (TestTheSandboxsRealHosts
// tries the internet), this computer's loopback, and is reached there by the
// user's browser: the Run button's dev server. pasta gives the namespace this
// computer's own address, so that address is the namespace's, not this computer's.
func TestTheSandboxsWholeNetworkThroughPasta(t *testing.T) {
	home, project, _ := sandboxed(t)
	needPasta(t)
	dev := hostServer(t, "127.0.0.1:0", "dev-server")
	c, self := withSelf(t, base(home, project))
	c.Network = environment.NetworkAll
	run := func(script string) (string, error) { return runIn(Bubblewrap{}, project, c, script) }
	if out, _ := run("awk 'NR>1 && $2==\"00000000\"' /proc/net/route; awk '$1==\"00000000000000000000000000000000\" && $10!=\"lo\"' /proc/net/ipv6_route"); out == "" {
		t.Error("no default route in the sandbox")
	}
	if out, err := run("curl -sS -m 5 http://" + dev + "/"); out != "dev-server" {
		t.Errorf("the host's loopback: %q, %v", out, err)
	}
	if out, _ := run("grep CapEff /proc/self/status; id -u"); out != "CapEff:\t0000000000000000\n"+strconv.Itoa(os.Getuid()) {
		t.Errorf("capabilities and user: %q", out)
	}
	if out, err := run("exit 3"); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("exit status through pasta: %v, %q", err, out)
	}
	// A server in the sandbox, reached from this computer's loopback.
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := free.Addr().String()
	free.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runIn(Bubblewrap{}, project, c, self+" "+serveArg+" "+addr)
	}()
	var got string
	for i := 0; i < 16 && got != "in the sandbox"; i++ {
		time.Sleep(500 * time.Millisecond)
		if resp, err := http.Get("http://" + addr + "/"); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			got = string(b)
		}
	}
	if got != "in the sandbox" {
		t.Errorf("a server in the sandbox from this computer: %q", got)
	}
	<-done
}

func base(home, project string) environment.Confinement {
	return environment.Confinement{
		Home: home, Write: []string{project}, Scratch: []string{filepath.Join(home, ".cache", "proj")},
		Read: []string{filepath.Join(home, "toolchain")}, Network: environment.NetworkNone,
	}
}

func TestTheSandboxKeepsFilesInTheirPlace(t *testing.T) {
	home, project, run := sandboxed(t)
	c := base(home, project)
	c.Protected = []string{filepath.Join(project, ".git", "config"), filepath.Join(project, ".git", "hooks")}
	c.NoMove = []string{filepath.Join(project, ".git")}
	for _, step := range []struct {
		name, script string
		ok           bool
	}{
		{"the project is written", "echo x > src/new && cat src/new", true},
		{"the project's caches are written", "echo x > " + filepath.Join(home, ".cache", "proj", "c"), true},
		{"a listed toolchain is read", "cat " + filepath.Join(home, "toolchain", "tool"), true},
		{"git can write its objects", "mkdir -p .git/objects && echo x > .git/objects/o", true},
		{"the home folder is not read", "cat " + filepath.Join(home, ".ssh", "id"), false},
		{"the home folder is not listed", "ls " + filepath.Join(home, ".ssh"), false},
		{"nothing is written to the home folder", "echo x > " + filepath.Join(home, "planted"), false},
		{"nothing is written to the system", "echo x > /etc/planted", false},
		{"a protected file is not written", "echo x >> .git/config", false},
		{"a protected folder is not written", "echo x > .git/hooks/pre-commit", false},
		{"a folder that must stay cannot be moved", "mv .git .git-aside", false},
		{"a folder that must stay cannot be removed", "rm -rf .git", false},
	} {
		out, err := run(c, step.script)
		if (err == nil) != step.ok {
			t.Errorf("%s: err = %v, output %q", step.name, err, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(project, ".git", "config")); string(b) != "[core]" {
		t.Errorf(".git/config = %q", b)
	}
	if _, err := os.Stat(filepath.Join(home, "planted")); err == nil {
		t.Error("a file was planted in the home folder")
	}

	ro := c
	ro.ReadOnly = true
	if out, err := run(ro, "echo x > src/plan"); err == nil {
		t.Errorf("plan mode wrote the project: %q", out)
	}
}

func TestTheSandboxHidesWhatADenyRuleCovers(t *testing.T) {
	home, project, run := sandboxed(t)
	os.WriteFile(filepath.Join(project, ".env"), []byte("TOKEN=secret"), 0o600)
	os.MkdirAll(filepath.Join(project, "private", "keep"), 0o755)
	os.WriteFile(filepath.Join(project, "private", "keep", "skill.sh"), []byte("ok"), 0o644)
	os.WriteFile(filepath.Join(project, "private", "data"), []byte("hidden"), 0o644)
	os.WriteFile(filepath.Join(project, "src", "a.key"), []byte("k"), 0o644)
	c := base(home, project)
	c.NoAccess = []string{filepath.Join(project, ".env"), filepath.Join(project, "private")}
	c.ReadAlways = []string{filepath.Join(project, "private", "keep")}
	c.ProtectedPatterns = []string{`^` + strings.ReplaceAll(project, ".", `\.`) + `/src/[^/]*\.key$`}
	for _, step := range []struct {
		name, script string
		ok           bool
	}{
		{"a denied file is not read", "cat .env | grep -q secret", false},
		{"a denied folder is not read", "cat private/data", false},
		{"what is kept readable in it is", "cat private/keep/skill.sh", true},
		{"a wildcard's match is not written", "echo x >> src/a.key", false},
	} {
		out, err := run(c, step.script)
		if (err == nil) != step.ok {
			t.Errorf("%s: err = %v, output %q", step.name, err, out)
		}
	}
}

func TestTheSandboxIsolatesProcessesAndSockets(t *testing.T) {
	home, project, run := sandboxed(t)
	outside := exec.Command("sleep", "30")
	outside.Start()
	defer outside.Process.Kill()
	c := base(home, project)
	if out, err := run(c, "kill -0 "+strconv.Itoa(outside.Process.Pid)+" 2>/dev/null && echo seen || echo hidden"); out != "hidden" {
		t.Errorf("another program is visible: %q, %v", out, err)
	}
	// A socket outside the project, as an SSH agent's or Docker's.
	sock := filepath.Join(home, "agent.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if out, err := run(c, "test -S "+sock+" && echo reachable || echo gone"); out != "gone" {
		t.Errorf("a socket in the home folder is reachable: %q, %v", out, err)
	}
	// A socket in the project stays usable: a dev server's.
	inner := filepath.Join(project, "dev.sock")
	l2, err := net.Listen("unix", inner)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if out, err := run(c, "test -S "+inner+" && echo reachable"); out != "reachable" {
		t.Errorf("a socket in the project is gone: %q, %v", out, err)
	}
}

func TestTheSandboxsNetwork(t *testing.T) {
	home, project, _ := sandboxed(t)
	for _, step := range []struct {
		b Bubblewrap
		n environment.Network
	}{{Bubblewrap{}, environment.NetworkNone}, {Bubblewrap{noPasta: true}, environment.NetworkLocalhost}} {
		c := base(home, project)
		c.Network = step.n
		// /proc/net/dev is the namespace's own; /sys shows the host's.
		if out, _ := runIn(step.b, project, c, "awk 'NR>2{print $1}' /proc/net/dev"); out != "lo:" {
			t.Errorf("%s: interfaces %q, want only lo", step.n, out)
		}
	}
	c := base(home, project)
	c.Network = environment.NetworkAll
	if out, _ := runIn(Bubblewrap{}, project, c, "awk 'NR>2{print $1}' /proc/net/dev"); out == "lo:" {
		t.Errorf("all: only lo")
	}
}

// hostServer listens on addr and answers every request with its name.
func hostServer(t *testing.T, addr, name string) string {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skip(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, name) })}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

// outsideAddr is an address of this computer that is not loopback, which a
// sandboxed command must not reach: the closest thing to the internet a test has.
func outsideAddr(t *testing.T) string {
	t.Helper()
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			return ip.IP.String()
		}
	}
	t.Skip("this computer has no address beyond loopback")
	return ""
}

func needPasta(t *testing.T) {
	t.Helper()
	if pastaArgs(false) == nil {
		t.Skip("pasta is not installed here, or this computer has no default route")
	}
}

// ENV-009, OQ-037: with pasta, localhost is this computer's, as on macOS: a dev
// server on the host's loopback is reached, one started after the command too,
// and nothing beyond it. The command cannot reconfigure pasta's network.
func TestTheSandboxsLocalhostThroughPasta(t *testing.T) {
	home, project, run := sandboxed(t)
	needPasta(t)
	dev := hostServer(t, "127.0.0.1:0", "dev-server")
	outside := hostServer(t, outsideAddr(t)+":0", "outside")
	c := base(home, project)
	c.Network = environment.NetworkLocalhost
	if out, err := run(c, "curl -sS -m 5 http://"+dev+"/"); out != "dev-server" {
		t.Errorf("the host's dev server: %q, %v", out, err)
	}
	if out, err := run(c, "curl -sS -m 5 http://"+outside+"/"); err == nil {
		t.Errorf("an address beyond loopback was reached: %q", out)
	}
	if out, _ := run(c, "awk 'NR>1' /proc/net/route; awk '$10!=\"lo\"' /proc/net/ipv6_route"); out != "" {
		t.Errorf("routes in the sandbox: %q", out)
	}
	if out, _ := run(c, "grep CapEff /proc/self/status; id -u"); out != "CapEff:\t0000000000000000\n"+strconv.Itoa(os.Getuid()) {
		t.Errorf("capabilities and user: %q", out)
	}
	if out, err := run(c, "exit 3"); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("exit status through pasta: %v, %q", err, out)
	}
	// A server that starts while the command runs: pasta looks each second.
	late, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := late.Addr().(*net.TCPAddr).Port
	late.Close()
	lateSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "late") })}
	t.Cleanup(func() { lateSrv.Close() })
	go func() {
		time.Sleep(time.Second)
		if ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
			lateSrv.Serve(ln)
		}
	}()
	if out, err := run(c, "for i in $(seq 20); do curl -sS -m 2 http://127.0.0.1:"+strconv.Itoa(port)+"/ 2>/dev/null && exit 0; sleep 0.5; done; exit 1"); out != "late" {
		t.Errorf("a server started later: %q, %v", out, err)
	}
}

// Without pasta, localhost is the sandbox's own: the host's loopback is not
// reached. (What the sandbox starts there is: the bridge below is such a server.)
func TestTheSandboxsOwnLocalhost(t *testing.T) {
	home, project, _ := sandboxed(t)
	dev := hostServer(t, "127.0.0.1:0", "dev-server")
	c := base(home, project)
	c.Network = environment.NetworkLocalhost
	if out, err := runIn(Bubblewrap{noPasta: true}, project, c, "curl -sS -m 5 http://"+dev+"/"); err == nil {
		t.Errorf("the host's loopback was reached without pasta: %q", out)
	}
}

// egressStandIn is a forward proxy for http URLs on loopback and a Unix socket, as
// the app's egress proxy is on Linux, that lets only allowed.test through.
func egressStandIn(t *testing.T) (port int, socket string) {
	t.Helper()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Hostname() != "allowed.test" {
			http.Error(w, "not on the list", http.StatusForbidden)
			return
		}
		io.WriteString(w, "through the proxy")
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	socket = filepath.Join(t.TempDir(), "egress.sock")
	ul, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	go srv.Serve(ul)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().(*net.TCPAddr).Port, socket
}

// ENV-010, OQ-037: the listed hosts are reached through the proxy, by pasta's
// forwarding or, without it, by the bridge to the proxy's socket; others are
// refused by it, and nothing gets around it. With no network, not even the proxy.
func TestTheSandboxsListedHosts(t *testing.T) {
	home, project, _ := sandboxed(t)
	port, socket := egressStandIn(t)
	outside := hostServer(t, outsideAddr(t)+":0", "outside")
	proxy := "http://127.0.0.1:" + strconv.Itoa(port)
	c := base(home, project)
	c.Network, c.Certificates, c.ProxySocket, c.ProxyPort = environment.NetworkLocalhost, true, socket, port
	ways := map[string]Bubblewrap{"bridge": {noPasta: true}}
	if pastaArgs(false) != nil {
		ways["pasta"] = Bubblewrap{}
	} else {
		t.Log("pasta is not installed here: only the bridge is tried")
	}
	for name, b := range ways {
		if out, err := runIn(b, project, c, "curl -sS -f -m 5 -x "+proxy+" http://allowed.test/"); out != "through the proxy" {
			t.Errorf("%s: a listed host: %q, %v", name, out, err)
		}
		if out, err := runIn(b, project, c, "curl -sS -f -m 5 -x "+proxy+" http://other.test/"); err == nil || !strings.Contains(out, "403") {
			t.Errorf("%s: another host was not refused by the proxy: %q, %v", name, out, err)
		}
		if out, err := runIn(b, project, c, "curl -sS -m 5 --noproxy '*' http://"+outside+"/"); err == nil {
			t.Errorf("%s: around the proxy: %q", name, out)
		}
		if out, err := runIn(b, project, c, "test -S "+socket+" && echo reachable || echo gone"); out != "gone" {
			t.Errorf("%s: the proxy's socket at its own path: %q, %v", name, out, err)
		}
		if out, err := runIn(b, project, c, "echo out; exit 4"); out != "out" || err == nil || !strings.Contains(err.Error(), "exit status 4") {
			t.Errorf("%s: a command's output and status: %q, %v", name, out, err)
		}
	}

	none := c
	none.Network = environment.NetworkNone
	if out, err := runIn(Bubblewrap{}, project, none, "curl -sS -m 5 -x "+proxy+" http://allowed.test/"); err == nil {
		t.Errorf("no network reached the proxy: %q", out)
	}
	if out, _ := runIn(Bubblewrap{}, project, none, "test -e /run/freeloop && echo there || echo absent"); out != "absent" {
		t.Errorf("no network has the bridge: %q", out)
	}
}

// ENV-010 on the real proxy and internet, as TestSeatbeltHosts on macOS.
// FREELOOP_SANDBOX_TEST=1 FREELOOP_SANDBOX_NET=1 go test ./internal/platform/linux -run RealHosts
func TestTheSandboxsRealHosts(t *testing.T) {
	home, project, _ := sandboxed(t)
	if os.Getenv("FREELOOP_SANDBOX_NET") == "" {
		t.Skip("set FREELOOP_SANDBOX_NET=1; this reaches the internet")
	}
	var blocked []string
	egress, err := netguard.StartEgress([]string{"registry.npmjs.org", "github.com"}, func(h string) { blocked = append(blocked, h) })
	if err != nil {
		t.Fatal(err)
	}
	defer egress.Close()
	socket := filepath.Join(t.TempDir(), "egress.sock")
	if err := egress.ServeUnix(socket); err != nil {
		t.Fatal(err)
	}
	c := base(home, project)
	c.Network, c.Certificates, c.ProxySocket, c.ProxyPort = environment.NetworkLocalhost, true, socket, egress.Port()
	env := ""
	for k, v := range environment.ProxyEnv(egress.URL()) {
		env += k + "='" + v + "' "
	}
	for name, b := range map[string]Bubblewrap{"bridge": {noPasta: true}, "pasta": {}} {
		if name == "pasta" && pastaArgs(false) == nil {
			continue
		}
		for _, command := range []string{
			"curl -sS -m 30 -o /dev/null https://registry.npmjs.org/react/latest",
			"git ls-remote https://github.com/git/git.git HEAD",
		} {
			if out, err := runIn(b, project, c, env+command); err != nil {
				t.Errorf("%s: %s: %v: %.300s", name, command, err, out)
			}
		}
		for _, command := range []string{
			"curl -sS -m 15 -o /dev/null https://example.com",
			"curl -sS -m 15 --noproxy '*' -o /dev/null https://registry.npmjs.org/react/latest",
		} {
			if out, err := runIn(b, project, c, env+command); err == nil {
				t.Errorf("%s: %s: was allowed: %s", name, command, out)
			}
		}
	}
	if len(blocked) != 1 || blocked[0] != "example.com" {
		t.Errorf("blocked = %v", blocked)
	}
	// All of the network, through pasta's namespace: names resolve and the
	// internet answers, with no proxy.
	if pastaArgs(true) != nil {
		all := base(home, project)
		all.Network = environment.NetworkAll
		for _, command := range []string{"curl -sS -m 30 -o /dev/null https://example.com", "git ls-remote https://github.com/git/git.git HEAD"} {
			if out, err := runIn(Bubblewrap{}, project, all, command); err != nil {
				t.Errorf("all: %s: %v: %.300s", command, err, out)
			}
		}
	}
}

// What a command's network is made of, without running it.
func TestTheSandboxsNetworkPlan(t *testing.T) {
	c := environment.Confinement{Home: "/home/x", Write: []string{"/home/x/p"}, Network: environment.NetworkLocalhost, ProxySocket: "/nonexistent/egress.sock", ProxyPort: 4000}
	if _, err := (Bubblewrap{noPasta: true}).network(c); err == nil {
		t.Error("a proxy that is not running was taken")
	}
	sock := filepath.Join(t.TempDir(), "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	c.ProxySocket = sock
	n, err := Bubblewrap{noPasta: true}.network(c)
	if err != nil || n.pasta != nil || !slices.Contains(n.mounts, sock) || !slices.Contains(n.bridge, "127.0.0.1:4000") {
		t.Errorf("the bridge: %+v, %v", n, err)
	}
	args, _ := bubblewrapArgs(c, n)
	if !slices.Contains(args, "--unshare-net") || slices.Index(args, bridgeExe) < slices.Index(args, "/run") {
		t.Errorf("bwrap's options with the bridge: %v", args)
	}
	c.Network = environment.NetworkNone
	if n, _ := (Bubblewrap{}).network(c); n.pasta != nil || n.bridge != nil {
		t.Errorf("none: %+v", n)
	}
	// All of the network is never the host's namespace: pasta's, or without pasta
	// the command is refused and told what to install (OQ-058).
	c.Network = environment.NetworkAll
	if n, err := (Bubblewrap{noPasta: true}).network(c); err == nil || !strings.Contains(err.Error(), "install the passt package") {
		t.Errorf("all without pasta: %+v, %v", n, err)
	}
	if n, _ := (Bubblewrap{}).network(c); n.pasta != nil {
		if !slices.Contains(n.pasta, "--config-net") || n.bridge != nil {
			t.Errorf("all through pasta: %+v", n)
		}
		if args, _ := bubblewrapArgs(c, n); !slices.Contains(args, "--unshare-user") {
			t.Errorf("bwrap's options for all in pasta's namespace: %v", args)
		}
	}
	c.Network = environment.NetworkLocalhost
	pasta := netPlan{pasta: []string{"/usr/bin/pasta", "--"}}
	args, _ = bubblewrapArgs(c, pasta)
	if slices.Contains(args, "--unshare-net") || !slices.Contains(args, "--unshare-user") || !slices.Contains(args, "--cap-drop") {
		t.Errorf("bwrap's options in pasta's namespace: %v", args)
	}
}

func TestASandboxNeedsAProject(t *testing.T) {
	if _, err := BubblewrapArgs(environment.Confinement{Home: "/home/x"}); err == nil {
		t.Error("a sandbox without a project was made")
	}
}

// From an AppImage the bridge runs from the AppImage's folder, mounted whole, so it
// finds the libraries beside it; elsewhere the program alone is mounted.
func TestBridgeMounts(t *testing.T) {
	m, exe := bridgeMounts("/tmp/.mount_FreeXY/usr/bin/freeloop", "/tmp/.mount_FreeXY")
	if exe != "/tmp/.mount_FreeXY/usr/bin/freeloop" || !slices.Equal(m, []string{"--ro-bind", "/tmp/.mount_FreeXY", "/tmp/.mount_FreeXY"}) {
		t.Errorf("AppImage: %q, %q", m, exe)
	}
	m, exe = bridgeMounts("/usr/bin/freeloop", "")
	if exe != bridgeExe || !slices.Equal(m, []string{"--ro-bind", "/usr/bin/freeloop", bridgeExe}) {
		t.Errorf("installed: %q, %q", m, exe)
	}
	if _, exe := bridgeMounts("/usr/bin/freeloop", "/tmp/.mount_other"); exe != bridgeExe {
		t.Errorf("an APPDIR the program is not in was used: %q", exe)
	}
}

// Which sockets are hidden, without running anything: /proc/net/unix's paths that
// exist, outside what the command writes and what the sandbox empties anyway,
// unless mounted back.
func TestWhichSocketsAreHidden(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	real, _ := filepath.EvalSymlinks(sock)
	text := "Num       RefCount Protocol Flags    Type St Inode Path\n" +
		"0000000000000000: 00000002 00000000 00010000 0001 01  4242 " + sock + "\n" +
		"0000000000000000: 00000002 00000000 00010000 0001 01  4243 @/tmp/.X11-unix/X0\n" +
		"0000000000000000: 00000002 00000000 00010000 0001 01  4244 /nonexistent/gone.sock\n" +
		"0000000000000000: 00000003 00000000 00000000 0001 03  4245\n"
	if got := listedSockets(text); !slices.Equal(got, []string{real}) {
		t.Errorf("listed = %q", got)
	}
	got := shownSockets([]string{"/var/lib/x/s", "/home/u/p/dev.sock", "/home/u/.cache/c/s", "/home/u/other.sock", "/home/u/tool/s", "/run/docker.sock", "/var/tmp/s", "/nix/var/nix/daemon-socket/socket"},
		"/home/u", []string{"/home/u/p", "/home/u/.cache/c"}, []string{"/home/u/tool"})
	if !slices.Equal(got, []string{"/var/lib/x/s", "/home/u/tool/s"}) {
		t.Errorf("shown = %q", got)
	}
	covers := socketCovers([]string{real})
	if len(covers) != 1 || !covers[0].dir || covers[0].args[0] != "--tmpfs" || slices.Contains(covers[0].args, real) {
		t.Errorf("covers = %+v", covers)
	}
}
