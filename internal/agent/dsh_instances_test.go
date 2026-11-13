package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dshRan stubs the instances running on this machine for one test.
func dshRan(t *testing.T, list ...dshInstance) {
	t.Helper()
	was := dshScan
	dshScan = func() []dshInstance { return list }
	t.Cleanup(func() { dshScan = was })
}

// The command lines dsh is started with on a Mac and on Linux: the CLI's
// web server (its port the system's pick, --port 0), a profile of its own
// the desktop app runs beside the CLI's, a session in a terminal, and lines
// that aren't a dsh at all.
func TestDshInstanceArgs(t *testing.T) {
	for _, c := range []struct {
		args    string
		profile string
		port    int
		ok      bool
	}{
		{"node /opt/homebrew/bin/dsh web --host 127.0.0.1 --port 0 --no-open", "web", 0, true},
		{"/Applications/DSH Tauri.app/Contents/Resources/runtime/darwin-arm64/DSH Tauri.app/Contents/MacOS/DSH Tauri --expose-internals /Users/hgr/dsh-trial/dsh/lib/bin.js --profile trial-web --port 3081", "trial-web", 3081, true},
		{"dsh --profile acp", "acp", 0, true},
		{"dsh -p work --port=3090", "work", 3090, true},
		{"dsh web", "web", 0, true},
		{"dsh --version", "", 0, false},
		{"dsh --dump-config", "", 0, false},
		{"node /Users/hgr/dsh-trial/dsh/lib/bin.js plugin --profile trial-web add x", "trial-web", 0, true},
		{"/usr/bin/node /opt/homebrew/lib/node_modules/other/bin.js --port 1", "", 0, false},
		{"vite --port 3081", "", 0, false},
	} {
		profile, port, ok := dshInstanceArgs(c.args)
		if profile != c.profile || port != c.port || ok != c.ok {
			t.Errorf("dshInstanceArgs(%q) = %q, %d, %v; want %q, %d, %v",
				c.args, profile, port, ok, c.profile, c.port, c.ok)
		}
	}
}

// The instances running beside the default one are rows of their own, each
// with the model field of its own home: a pick on the trial profile's row
// lands in that profile, and leaves ~/.dsh's alone.
func TestDshInstancesAreRowsOfTheirOwn(t *testing.T) {
	home, dir, web := dshRouteHome(t)
	trial := filepath.Join(home, ".dsh-trial")
	trialWeb := filepath.Join(trial, "profiles", "trial-web", "cordis.patch.yml")
	if err := os.MkdirAll(filepath.Dir(trialWeb), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(web, []byte(dshInstanceOwn), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trialWeb, []byte(dshInstanceOwn), 0o600); err != nil {
		t.Fatal(err)
	}
	// the CLI's web server on the default home, and the desktop app's own
	// checkout on trial-web: only the second is a row here
	dshRan(t,
		dshInstance{Home: dir, Profile: "web", Port: 3080, PID: 1},
		dshInstance{Home: trial, Profile: "trial-web", Port: 3081, PID: 2},
	)
	rows := dshInstances(home, dir)
	if len(rows) != 1 {
		t.Fatalf("dshInstances = %d rows, want 1 (the default home is dsh's own row)", len(rows))
	}
	a := rows[0]
	if a.ID != "dsh#trial-web" {
		t.Errorf("ID = %q, want dsh#trial-web", a.ID)
	}
	if want := "DeepSeek Harness · trial-web · 3081"; a.Name != want {
		t.Errorf("Name = %q, want %q", a.Name, want)
	}
	if a.Path != trialWeb {
		t.Errorf("Path = %q, want the trial profile's patch list %q", a.Path, trialWeb)
	}
	if !a.Detected() {
		t.Error("the row of a running instance reads as not detected")
	}
	if err := a.Field("model").Set("magpie/deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	trialNow := readFile(trialWeb)
	if !strings.Contains(trialNow, "deepseek/pro") {
		t.Errorf("the trial profile has no magpie model in its start:\n%s", trialNow)
	}
	if got := readFile(web); got != dshInstanceOwn {
		t.Errorf("~/.dsh's web profile changed, a pick on the trial row is not its own:\n%s", got)
	}
	// and the pick is what the row shows
	if got := a.Field("model").Get(); got != "magpie/deepseek/pro" {
		t.Errorf("model shown = %q, want magpie/deepseek/pro", got)
	}
}

// $DSH_HOME can move dsh's row onto another home: that home is still the
// row's, not a second instance of it (magpie opened from a shell inside the
// trial harness listed the same profile twice).
func TestDshInstancesSkipTheRowOnDSHHome(t *testing.T) {
	home, _, _ := dshRouteHome(t)
	trial := filepath.Join(home, ".dsh-trial")
	f := filepath.Join(trial, "profiles", "trial-web", "cordis.patch.yml")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, []byte(dshInstanceOwn), 0o600); err != nil {
		t.Fatal(err)
	}
	dshRan(t, dshInstance{Home: trial, Profile: "trial-web", Port: 3081, PID: 1})
	if rows := dshInstances(home, trial); len(rows) != 0 {
		t.Fatalf("dshInstances = %d rows for the home dsh's own row reads, want 0", len(rows))
	}
}

// Two homes booting a profile of the same name: the rows take their
// folder's, so both are listed.
func TestDshInstancesOfTheSameProfileName(t *testing.T) {
	home, _, _ := dshRouteHome(t)
	var list []dshInstance
	for i, name := range []string{".dsh-trial", ".dsh-work"} {
		dir := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Join(dir, "profiles", "web"), 0o755); err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(dir, "profiles", "web", "cordis.patch.yml")
		if err := os.WriteFile(f, []byte(dshInstanceOwn), 0o600); err != nil {
			t.Fatal(err)
		}
		list = append(list, dshInstance{Home: dir, Profile: "web", Port: 3080 + i, PID: i})
	}
	dshRan(t, list...)
	rows := dshInstances(home, filepath.Join(home, ".dsh"))
	if len(rows) != 2 {
		t.Fatalf("dshInstances = %d rows, want 2", len(rows))
	}
	if rows[0].ID == rows[1].ID {
		t.Fatalf("both rows have the ID %q", rows[0].ID)
	}
	for i, want := range []string{"dsh#web-dsh-trial", "dsh#web-dsh-work"} {
		if rows[i].ID != want {
			t.Errorf("rows[%d].ID = %q, want %q", i, rows[i].ID, want)
		}
	}
}

// The dsh row names the port its own home answers on, so that it and an
// instance's read apart on the Agents page.
func TestDshRowNamesItsPort(t *testing.T) {
	home, dir, web := dshRouteHome(t)
	if err := os.WriteFile(web, []byte(dshInstanceOwn), 0o600); err != nil {
		t.Fatal(err)
	}
	dshRan(t, dshInstance{Home: dir, Profile: "web", Port: 3080, PID: 1})
	if got := dsh(home).Name; got != "DeepSeek Harness · 3080" {
		t.Fatalf("dsh row name = %q, want DeepSeek Harness · 3080", got)
	}
	// nothing running: the row is as it was, and a stale scan says nothing
	dshRan(t)
	if got := dsh(home).Name; got != "DeepSeek Harness" {
		t.Fatalf("dsh row name = %q with nothing running, want DeepSeek Harness", got)
	}
}

// dshInstanceOwn is a profile's patch list as a dsh writes it: a start of
// its own and no route of magpie's yet.
const dshInstanceOwn = "# Your patch layer for this dsh profile.\n" +
	"- id: agent-default-model\n  config:\n    provider: deepseek\n    model: deepseek/flash\n"

func TestDshInstanceName(t *testing.T) {
	for _, c := range []struct {
		profile string
		port    int
		want    string
	}{
		{"trial-web", 3081, "DeepSeek Harness · trial-web · 3081"},
		{"web", 0, "DeepSeek Harness · web"},
		{"", 3080, "DeepSeek Harness · 3080"},
	} {
		if got := dshInstanceName(c.profile, c.port); got != c.want {
			t.Errorf("dshInstanceName(%q, %d) = %q, want %q", c.profile, c.port, got, c.want)
		}
	}
}

// Each instance reaches the gateway at an address of its own,
// <gateway>/agent/<instance>/v1: dsh's User-Agent is the same for both, so
// without it the gateway counted two harnesses as one dsh's calls.
func TestDshGatewayPathNamesTheInstance(t *testing.T) {
	home, dir, _ := dshRouteHome(t)
	trial := filepath.Join(home, ".dsh-trial")
	dshRan(t,
		dshInstance{Home: dir, Profile: "web", Port: 3080, PID: 1},
		dshInstance{Home: trial, Profile: "trial-web", Port: 3081, PID: 2},
	)
	const gw = "http://127.0.0.1:3425"
	if got, want := dshGW(dir, gw), gw+"/agent/dsh-web"; got != want {
		t.Errorf("dshGW(default home) = %q, want %q", got, want)
	}
	if got, want := dshGW(trial, gw), gw+"/agent/dsh-trial-web"; got != want {
		t.Errorf("dshGW(trial home) = %q, want %q", got, want)
	}
	// a home nothing runs on keeps dsh's own address
	if got := dshGW(filepath.Join(home, ".dsh-gone"), gw); got != gw {
		t.Errorf("dshGW(a home no instance runs on) = %q, want %q", got, gw)
	}
	// already named: not nested
	if got, want := dshGW(trial, gw+"/agent/dsh-trial-web"), gw+"/agent/dsh-trial-web"; got != want {
		t.Errorf("dshGW(already named) = %q, want %q", got, want)
	}
	// the gateway reads the path as the instance's only if it knows the id
	if !dshPathAgent("dsh-trial-web") || !dshPathAgent("dsh-web") {
		t.Error("the running instances' ids are not agents to the gateway")
	}
	if dshPathAgent("dsh-other") || dshPathAgent("openbot") {
		t.Error("an id no instance has reads as one of them")
	}
	// and with nothing running, no id is (a stale scan names nothing)
	dshRan(t)
	if dshPathAgent("dsh-trial-web") {
		t.Error("an instance still reads as running after the scan said none is")
	}
	// two homes of one profile name are two ids, as they are two rows
	dshRan(t,
		dshInstance{Home: filepath.Join(home, ".dsh-trial"), Profile: "web", Port: 3080, PID: 1},
		dshInstance{Home: filepath.Join(home, ".dsh-work"), Profile: "web", Port: 3090, PID: 2},
	)
	if a, b := dshGW(filepath.Join(home, ".dsh-trial"), gw), dshGW(filepath.Join(home, ".dsh-work"), gw); a == b {
		t.Errorf("two instances of one profile name share the address %q", a)
	}
}

// What each row writes is that row's address at the gateway: the same pick
// on two rows reaches the gateway as two callers, so its live calls, models
// and sessions are two rows' there and not one dsh's.
func TestDshInstancesWriteTheirOwnGatewayPath(t *testing.T) {
	home, dir, web := dshRouteHome(t)
	trial := filepath.Join(home, ".dsh-trial")
	trialWeb := filepath.Join(trial, "profiles", "trial-web", "cordis.patch.yml")
	if err := os.MkdirAll(filepath.Dir(trialWeb), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{web, trialWeb} {
		if err := os.WriteFile(f, []byte(dshInstanceOwn), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dshRan(t,
		dshInstance{Home: dir, Profile: "web", Port: 3080, PID: 1},
		dshInstance{Home: trial, Profile: "trial-web", Port: 3081, PID: 2},
	)
	rows := dshInstances(home, dir)
	if len(rows) != 1 {
		t.Fatalf("dshInstances = %d rows, want 1", len(rows))
	}
	if err := rows[0].Field("model").Set("magpie/deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(trialWeb); !strings.Contains(got, "/agent/dsh-trial-web/v1") {
		t.Errorf("the trial row's route is not at the trial instance's own address:\n%s", got)
	}
	if err := dsh(home).Field("model").Set("magpie/deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(web); !strings.Contains(got, "/agent/dsh-web/v1") {
		t.Errorf("dsh's own row's route is not at its instance's own address:\n%s", got)
	}
}
