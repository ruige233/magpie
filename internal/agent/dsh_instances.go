package agent

// dsh instances of the user's own: dsh started on a home of its own —
// DSH_HOME set to a second checkout (~/.dsh-trial beside ~/.dsh), or a
// second profile answering on a port of its own — instead of the default
// home the dsh row wires. magpie's dsh row reads and writes ~/.dsh only,
// so a trial harness beside the everyday one, or Kiro Crew's
// `dsh --profile acp` (see kirocrew.go), was wired by hand or not at all.
// Each instance running now gets a row of its own here, named by its
// profile and port, with the same model field: a pick is written to that
// instance's own profile patch list, which dsh hot-reloads
// (@deepseek-ai/dsh-hmr watches cordis.patch.yml on every long-lived
// surface), so it takes effect for the sessions started after it — no
// restart, the sessions already open keeping the model they started on.
//
// Instances are found the way the user names them: the command lines of
// the dsh processes running now (ps), each one's DSH_HOME from the
// environment ps reports, its port from --port or else lsof. Nothing is
// written anywhere to find them, and an instance that isn't running has no
// row: its home is one the dsh row's own picker would name anyway.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/proc"
)

// dshInstance is one dsh running: the home it reads (its profiles, keys
// and .env), the profile it booted, the port its server answers on (0 when
// neither --port nor the system named one) and its process.
type dshInstance struct {
	Home    string
	Profile string
	Port    int
	PID     int
}

// dshScanAge is how long a scan is kept. The Agents page asks for every
// agent's state after nearly every click, and a scan is a ps, a ps -E per
// dsh process found and an lsof for each that named no port.
const dshScanAge = 2 * time.Second

// dshFound is dshScan's last answer.
var dshFound struct {
	sync.Mutex
	at   time.Time
	list []dshInstance
}

// dshScan is the dsh instances running on this machine, a var so a test
// can say what runs (as Running is).
var dshScan = func() []dshInstance {
	dshFound.Lock()
	defer dshFound.Unlock()
	if !dshFound.at.IsZero() && time.Since(dshFound.at) < dshScanAge {
		return dshFound.list
	}
	dshFound.at = time.Now()
	dshFound.list = dshRunning()
	return dshFound.list
}

// dshRunning scans the processes running now for dsh instances.
func dshRunning() []dshInstance {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	procs, err := proc.List(ctx, "")
	if err != nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	// the default home, which the dsh row already is
	def := filepath.Join(home, ".dsh")
	// DS Bot's home has a row of its own (dsbot), wired as its models
	bot := ""
	if dir, ok := dsbotHome(); ok {
		bot = filepath.Clean(dir)
	}
	self := os.Getpid()
	var out []dshInstance
	for _, p := range procs {
		// magpie is a dsh host itself in dev, and runs dsh for its own probes
		if p.PID == self || strings.Contains(strings.ToLower(p.Args), "magpie") {
			continue
		}
		profile, port, ok := dshInstanceArgs(p.Args)
		if !ok {
			continue
		}
		dir := ""
		if env, err := proc.EnvOf(ctx, p.PID, "DSH_HOME"); err == nil {
			dir = strings.TrimSpace(env["DSH_HOME"])
		}
		if dir == "" {
			dir = def
		}
		dir = filepath.Clean(dir)
		if dir == bot {
			continue
		}
		if !isDir(dir) {
			continue
		}
		// another app's home, which has a row of its own (DS Bot's)
		if _, claimed := dshApps.Load(dir); claimed {
			continue
		}
		if port == 0 {
			if ports := proc.PortsOf(ctx, p.PID); len(ports) > 0 {
				port = ports[0]
			}
		}
		out = dshRemember(out, dshInstance{Home: dir, Profile: profile, Port: port, PID: p.PID})
	}
	return out
}

// dshRemember is list with in, one entry per home and profile: the process
// that named a port kept over one that didn't (a dsh whose server is the
// child it started, --port 0 in the parent).
func dshRemember(list []dshInstance, in dshInstance) []dshInstance {
	for i, o := range list {
		if o.Home == in.Home && o.Profile == in.Profile {
			if o.Port == 0 && in.Port > 0 {
				list[i] = in
			}
			return list
		}
	}
	return append(list, in)
}

// dshInstanceArgs reads a dsh command line: the profile it booted (web's
// for `dsh web`, dsh's default) and the port it was given (0 for none, and
// for --port 0, the system's pick). false for a line that isn't a dsh
// server at all — `dsh --version`, `magpie dsh …`, a dsh that only prints
// its config.
func dshInstanceArgs(args string) (profile string, port int, ok bool) {
	fields := strings.Fields(args)
	start := -1
	for i, w := range fields {
		if dshProgramArg(w) {
			start = i
			break
		}
	}
	if start < 0 {
		return "", 0, false
	}
	web := false
	for i := start + 1; i < len(fields); i++ {
		w := fields[i]
		next := func() string {
			if i+1 < len(fields) {
				i++
				return fields[i]
			}
			return ""
		}
		switch {
		case w == "--profile" || w == "-p":
			profile = next()
		case strings.HasPrefix(w, "--profile="):
			profile = strings.TrimPrefix(w, "--profile=")
		case w == "--port":
			port, _ = strconv.Atoi(next())
		case strings.HasPrefix(w, "--port="):
			port, _ = strconv.Atoi(strings.TrimPrefix(w, "--port="))
		case w == "web" || w == "serve":
			// `dsh web` boots web's profile, no --profile needed
			web = true
		}
	}
	if profile == "" && !web {
		return "", 0, false
	}
	if profile == "" {
		profile = "web"
	}
	return profile, port, true
}

// dshProgramArg reports whether w starts dsh: the CLI itself (dsh,
// deepseek-harness), or its entry script under a dsh folder, as the
// desktop app runs it (…/dsh/lib/bin.js, …/dsh/dist/main.js).
func dshProgramArg(w string) bool {
	// node's own name is not dsh's: the script after it is
	if base := filepath.Base(w); base == "dsh" || base == "dsh.exe" || base == "deepseek-harness" || base == "deepseek-harness.exe" {
		return true
	}
	if !strings.HasSuffix(w, ".js") {
		return false
	}
	p := filepath.ToSlash(w)
	return strings.Contains(p, "/dsh/") || strings.Contains(p, "/deepseek-harness/")
}

// dshInstanceName is a row's name: the profile it booted and the port it
// answers on, so two instances read apart — DeepSeek Harness · trial-web
// · 3081.
func dshInstanceName(profile string, port int) string {
	name := "DeepSeek Harness"
	if profile != "" {
		name += " · " + profile
	}
	if port > 0 {
		name += fmt.Sprintf(" · %d", port)
	}
	return name
}

// dshKeys names each instance: the profile it booted, or with the folder of
// its home when another instance booted the same profile — a profile name
// alone names the row, and the gateway path, of one instance only.
func dshKeys(found []dshInstance) map[dshInstance]string {
	count := map[string]int{}
	for _, in := range found {
		count[in.Profile]++
	}
	keys := make(map[dshInstance]string, len(found))
	for _, in := range found {
		key := in.Profile
		if count[in.Profile] > 1 {
			key += "-" + strings.TrimPrefix(filepath.Base(in.Home), ".")
		}
		keys[in] = key
	}
	return keys
}

// dshGatewayID is an instance's id at the gateway: which dsh, and which of
// its instances, in the one path segment a URL takes. The row's own id is
// dsh#trial-web; a path can't carry a #, so this is dsh-trial-web.
func dshGatewayID(key string) string { return "dsh-" + key }

// dshGW is the gateway as the dsh on home dir reaches it: an instance's
// requests go through /agent/<instance>/v1, so the gateway counts them as
// that instance's. dsh's User-Agent is the same for every instance and its
// base address used to be too, so two harnesses were one dsh's in the
// Routing and Context tabs — its live calls, models and session counts
// summed.
func dshGW(dir, gw string) string {
	// what an instance was given already: never nested
	if strings.Contains(gw, gateway.AgentPath) {
		return gw
	}
	if id := dshGatewayIDOf(dir); id != "" {
		return gateway.AgentURL(gw, id)
	}
	return gw
}

// dshWrittenHome is the dsh home a patch list belongs to: a profile's
// (<home>/profiles/<name>/cordis.patch.yml), or the home layer's
// (<home>/cordis.patch.yml), which goes over every profile's of that home.
// "" for a file that is neither, so its writes stay as they were.
func dshWrittenHome(path string) string {
	if filepath.Base(path) != "cordis.patch.yml" {
		return ""
	}
	if filepath.Base(filepath.Dir(filepath.Dir(path))) == "profiles" {
		return dshProfileHome(path)
	}
	return filepath.Dir(path)
}

// dshGatewayIDOf is the gateway id of the dsh running on home dir, "" for a
// home no instance runs on (a dsh installed but not started): its requests,
// if any, stay the dsh row's.
func dshGatewayIDOf(dir string) string {
	dir = filepath.Clean(dir)
	found := dshScan()
	keys := dshKeys(found)
	for _, in := range found {
		if in.Home == dir {
			return dshGatewayID(keys[in])
		}
	}
	return ""
}

// dshPathAgent says whether id names one of the dsh instances running now,
// what usage.IsAgentInstance asks: the gateway takes /agent/<id>/ requests
// as that instance's on this word alone (see dshGW).
func dshPathAgent(id string) bool {
	found := dshScan()
	keys := dshKeys(found)
	for _, in := range found {
		if dshGatewayID(keys[in]) == id {
			return true
		}
	}
	return false
}

// dshPortOf is the port a dsh on home dir answers on, 0 when none is
// running there.
func dshPortOf(dir string) int {
	dir = filepath.Clean(dir)
	for _, in := range dshScan() {
		if in.Home == dir {
			return in.Port
		}
	}
	return 0
}

// dshInstances are the rows of the dsh instances running on this machine
// that the dsh row doesn't already name: those on own, the home it reads
// (~/.dsh, or $DSH_HOME's when magpie has one), are that row's, and DS
// Bot's home is its own row.
func dshInstances(home, own string) []*Agent {
	own = filepath.Clean(own)
	var found []dshInstance
	for _, in := range dshScan() {
		if in.Home == own {
			continue
		}
		found = append(found, in)
	}
	if len(found) == 0 {
		return nil
	}
	keys := dshKeys(dshScan())
	rows := make([]*Agent, 0, len(found))
	for _, in := range found {
		rows = append(rows, dshInstanceAgent(home, in, keys[in]))
	}
	return rows
}

// dshInstanceAgent is the row of one instance: dsh's wiring on that
// instance's home, with the profile it booted for the model it shows and
// writes.
func dshInstanceAgent(home string, in dshInstance, key string) *Agent {
	profile := in.Profile
	// the booted profile's patch list, made there under magpie's hand if
	// that profile hasn't written one yet
	fresh := func() string { return filepath.Join(in.Home, "profiles", profile, "cordis.patch.yml") }
	a := dshOn(here(home), in.Home, "dsh", fresh, func() string { return profile })
	a.ID, a.Name, a.Icon = "dsh#"+key, dshInstanceName(profile, in.Port), "deepseek-color"
	// `dsh` on the command line is the default home's: not this row's
	a.Aliases = nil
	// what this row shows and writes: this instance's own profile
	if f := dshLead(in.Home, profile); f != "" {
		a.Path = f
	} else {
		a.Path = fresh()
	}
	a.Notice = func() string {
		if len(dshProfiles(in.Home)) == 0 {
			return ""
		}
		// as dsh's own: the sessions started after a pick start on it, and
		// dsh reloads the file, so one open keeps the model it started on
		return noticeDsh1.String()
	}
	return a
}

// withDshInstances is agents with a row for each dsh instance running
// beside the one dsh's own row reads, just after it.
func withDshInstances(agents []*Agent, home string) []*Agent {
	// the home dsh's row reads: its own is not a second instance
	own := filepath.Join(home, ".dsh")
	at := len(agents)
	for i, a := range agents {
		if a.ID == "dsh" {
			own, at = a.Dir, i+1
			break
		}
	}
	rows := dshInstances(home, own)
	if len(rows) == 0 {
		return agents
	}
	out := make([]*Agent, 0, len(agents)+len(rows))
	out = append(append(append(out, agents[:at]...), rows...), agents[at:]...)
	return out
}
