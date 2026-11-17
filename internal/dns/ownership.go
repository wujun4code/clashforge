package dns

// DNS changes have a write-ahead, on-disk ownership journal. Expected and Pending
// describe only writes made (or about to be made) by this package. Original is
// never replaced by a later Setup. A failed command or restart retains the ledger.
import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const dnsOwnerMarker = "clashforge DNS ownership journal v1\n"
const dnsManagedHeader = "# clashforge: managed DNS ownership v1\n"

// The override also lets callers sandbox all persistent DNS state in tests.
// The default is on persistent storage, not /run or /tmp.
func dnsStateDir() string {
	if dir := os.Getenv("CLASHFORGE_DNS_STATE_DIR"); dir != "" {
		return dir
	}
	return "/etc/clashforge-dns-ownership"
}

type dnsValue struct {
	Kind   string   `json:"kind,omitempty"`
	Values []string `json:"values,omitempty"`
}
type dnsField struct {
	Original dnsValue  `json:"original"`
	Expected dnsValue  `json:"expected"`
	Pending  *dnsValue `json:"pending,omitempty"`
	Planned  dnsValue  `json:"planned"`
}
type dnsFile struct {
	Expected string `json:"expected"`
	Pending  string `json:"pending,omitempty"`
}
type dnsJournal struct {
	Owner   string               `json:"owner"`
	Backend string               `json:"backend"`
	Target  string               `json:"target,omitempty"`
	Fields  map[string]*dnsField `json:"fields,omitempty"`
	Files   map[string]*dnsFile  `json:"files,omitempty"`
}
type dnsLedger struct {
	dir   string
	lock  *os.File
	state *dnsJournal
}

func secureDNSFile(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	want := os.FileMode(0600)
	if directory {
		want = 0700
	}
	if !ok || st.Uid != uint32(os.Geteuid()) || info.Mode().Perm() != want || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("dns: unsafe ownership state permissions or type")
	}
	return nil
}
func openDNSLedger(create bool) (*dnsLedger, error) {
	dir := dnsStateDir()
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, fmt.Errorf("dns: ownership directory must be an absolute clean path")
	}
	made := false
	if !create {
		if _, err := os.Lstat(dir); os.IsNotExist(err) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
	} else if err := os.Mkdir(dir, 0700); err == nil {
		made = true
	} else if !os.IsExist(err) {
		return nil, fmt.Errorf("dns: open ownership directory: %w", err)
	}
	if err := secureDNSFile(dir, true); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	l := &dnsLedger{dir: dir, lock: os.NewFile(uintptr(fd), dir)}
	if err = syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		l.close()
		return nil, err
	}
	fail := func(err error) (*dnsLedger, error) { l.close(); return nil, err }
	marker := filepath.Join(dir, "owner")
	if made {
		f, e := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return fail(e)
		}
		_, e = f.WriteString(dnsOwnerMarker)
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			return fail(e)
		}
	} else if err = secureDNSFile(marker, false); err != nil {
		return fail(fmt.Errorf("dns: unowned state directory: %w", err))
	}
	b, err := os.ReadFile(marker)
	if err != nil {
		return fail(err)
	}
	if string(b) != dnsOwnerMarker {
		return fail(fmt.Errorf("dns: ownership marker collision"))
	}
	path := filepath.Join(dir, "journal.json")
	if err = secureDNSFile(path, false); os.IsNotExist(err) {
		return l, nil
	} else if err != nil {
		return fail(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(err)
	}
	dec := json.NewDecoder(io.LimitReader(f, 1<<20))
	dec.DisallowUnknownFields()
	var state dnsJournal
	err = dec.Decode(&state)
	if err == nil {
		var trailing any
		if e := dec.Decode(&trailing); e != io.EOF {
			err = errors.New("trailing journal data")
		}
	}
	f.Close()
	if err != nil || state.Owner != dnsOwnerMarker {
		return fail(fmt.Errorf("dns: invalid ownership journal"))
	}
	if state.Backend != "uci" && state.Backend != "file" {
		return fail(fmt.Errorf("dns: invalid journal backend"))
	}
	if state.Backend == "uci" && (!dnsSectionID.MatchString(state.Target) || len(state.Fields) == 0 || len(state.Files) != 0) {
		return fail(fmt.Errorf("dns: invalid journal target or fields"))
	}
	if state.Backend == "file" && (len(state.Files) == 0 || len(state.Fields) != 0 || state.Target != "") {
		return fail(fmt.Errorf("dns: invalid file journal fields"))
	}
	for key, field := range state.Fields {
		if (key != "port" && key != "server" && key != "noresolv") || field == nil || !validDNSValue(field.Original) || !validDNSValue(field.Expected) || !validDNSValue(field.Planned) || (field.Pending != nil && !validDNSValue(*field.Pending)) {
			return fail(fmt.Errorf("dns: invalid field journal"))
		}
	}
	for path, file := range state.Files {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != clashforgeConfName || file == nil || (file.Expected != "" && !strings.HasPrefix(file.Expected, dnsManagedHeader)) || (file.Pending != "" && !strings.HasPrefix(file.Pending, dnsManagedHeader)) {
			return fail(fmt.Errorf("dns: invalid file journal"))
		}
	}
	l.state = &state
	return l, nil
}
func validDNSValue(v dnsValue) bool {
	return (v.Kind == "" && len(v.Values) == 0) || (v.Kind == "option" && len(v.Values) == 1) || (v.Kind == "list" && len(v.Values) > 0)
}
func (l *dnsLedger) close() {
	if l != nil && l.lock != nil {
		syscall.Flock(int(l.lock.Fd()), syscall.LOCK_UN)
		l.lock.Close()
	}
}
func (l *dnsLedger) save() error {
	b, err := json.Marshal(l.state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(l.dir, ".journal-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(l.dir, "journal.json")); err != nil {
		return err
	}
	return l.lock.Sync()
}
func (l *dnsLedger) release() error {
	if err := os.Remove(filepath.Join(l.dir, "journal.json")); err != nil {
		return err
	}
	return l.lock.Sync()
}

var dnsSectionID = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// Parse UCI's shell-quoted export, not `get`/`show` values (which lose the
// distinction between a scalar, a one-element list, and whitespace in values).
func dnsExportRecords(text string) ([][]string, error) {
	var records [][]string
	var record []string
	var token strings.Builder
	quote := byte(0)
	active := false
	word := func() {
		if active {
			record = append(record, token.String())
			token.Reset()
			active = false
		}
	}
	line := func() {
		word()
		if len(record) > 0 {
			records = append(records, record)
			record = nil
		}
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote == '\'' {
			if c == '\'' {
				quote = 0
			} else {
				token.WriteByte(c)
			}
			continue
		}
		if quote == '"' {
			if c == '"' {
				quote = 0
				continue
			}
			if c == '\\' {
				i++
				if i >= len(text) {
					return nil, fmt.Errorf("dns: malformed UCI escape")
				}
				c = text[i]
			}
			token.WriteByte(c)
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			active = true
		case '\\':
			i++
			if i >= len(text) {
				return nil, fmt.Errorf("dns: malformed UCI escape")
			}
			token.WriteByte(text[i])
			active = true
		case ' ', '\t', '\r':
			word()
		case '\n':
			line()
		case '#':
			if !active {
				for i < len(text) && text[i] != '\n' {
					i++
				}
				line()
			} else {
				token.WriteByte(c)
			}
		default:
			token.WriteByte(c)
			active = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("dns: unterminated UCI quote")
	}
	line()
	return records, nil
}
func dnsUCIRead(target string) (string, map[string]dnsValue, error) {
	out, err := exec.Command("uci", "export", "dhcp").Output()
	if err != nil {
		return "", nil, fmt.Errorf("dns: UCI export failed: %w", err)
	}
	records, err := dnsExportRecords(string(out))
	if err != nil {
		return "", nil, err
	}
	var sections []map[string]dnsValue
	var current map[string]dnsValue
	for _, r := range records {
		switch r[0] {
		case "package":
		case "config":
			if len(r) < 2 || len(r) > 3 {
				return "", nil, fmt.Errorf("dns: invalid UCI config record")
			}
			current = nil
			if r[1] == "dnsmasq" {
				current = map[string]dnsValue{}
				sections = append(sections, current)
			}
		case "option", "list":
			if len(r) != 3 {
				return "", nil, fmt.Errorf("dns: invalid UCI field record")
			}
			if current == nil {
				continue
			}
			old, exists := current[r[1]]
			if r[0] == "option" {
				if exists {
					return "", nil, fmt.Errorf("dns: duplicate UCI option")
				}
				current[r[1]] = dnsValue{Kind: "option", Values: []string{r[2]}}
			} else {
				if exists && old.Kind != "list" {
					return "", nil, fmt.Errorf("dns: mixed UCI field types")
				}
				old.Kind = "list"
				old.Values = append(old.Values, r[2])
				current[r[1]] = old
			}
		default:
			return "", nil, fmt.Errorf("dns: unsupported UCI export record")
		}
	}
	for i, fields := range sections {
		out, err = exec.Command("uci", "-X", "-q", "show", fmt.Sprintf("dhcp.@dnsmasq[%d]", i)).Output()
		if err != nil {
			return "", nil, fmt.Errorf("dns: resolve UCI section failed: %w", err)
		}
		first := strings.SplitN(string(out), "\n", 2)[0]
		pair := strings.SplitN(first, "=", 2)
		if len(pair) != 2 || pair[1] != "dnsmasq" || !strings.HasPrefix(pair[0], "dhcp.") {
			return "", nil, fmt.Errorf("dns: invalid UCI section identity")
		}
		id := strings.TrimPrefix(pair[0], "dhcp.")
		if !dnsSectionID.MatchString(id) {
			return "", nil, fmt.Errorf("dns: invalid UCI section identity")
		}
		if target == "" || target == id {
			return id, fields, nil
		}
	}
	return "", nil, fmt.Errorf("dns: owned dnsmasq section missing")
}
func dnsUCICommand(args ...string) error {
	// Command output and argv may contain private DNS values. Do not log them.
	if err := exec.Command("uci", args...).Run(); err != nil {
		return fmt.Errorf("dns: UCI %s failed: %w", args[0], err)
	}
	return nil
}
func sameDNSValue(a, b dnsValue) bool { return reflect.DeepEqual(a, b) }
func ownedDNSValue(f *dnsField, v dnsValue) bool {
	return sameDNSValue(v, f.Expected) || (f.Pending != nil && sameDNSValue(v, *f.Pending))
}
func (l *dnsLedger) writeField(key string, current, desired dnsValue) error {
	field := l.state.Fields[key]
	// Reconcile an interrupted write before planning the next command.
	field.Expected = current
	field.Pending = nil
	if err := l.save(); err != nil {
		return err
	}
	step := func(next dnsValue, args ...string) error {
		_, observed, err := dnsUCIRead(l.state.Target)
		if err != nil {
			return err
		}
		if !sameDNSValue(observed[key], field.Expected) {
			return fmt.Errorf("dns: field changed before write")
		}
		field.Pending = &next
		if err := l.save(); err != nil {
			return err
		}
		if err := dnsUCICommand(args...); err != nil {
			return err
		}
		_, observed, err = dnsUCIRead(l.state.Target)
		if err != nil {
			return err
		}
		if !sameDNSValue(observed[key], next) {
			return fmt.Errorf("dns: UCI write verification failed")
		}
		field.Expected = next
		field.Pending = nil
		return l.save()
	}
	path := "dhcp." + l.state.Target + "." + key
	if sameDNSValue(current, desired) {
		return nil
	}
	if desired.Kind == "option" {
		return step(desired, "set", path+"="+desired.Values[0])
	}
	if current.Kind != "" {
		if err := step(dnsValue{}, "delete", path); err != nil {
			return err
		}
	}
	if desired.Kind == "list" {
		for i, value := range desired.Values {
			next := dnsValue{Kind: "list", Values: append([]string(nil), desired.Values[:i+1]...)}
			if err := step(next, "add_list", path+"="+value); err != nil {
				return err
			}
		}
	}
	return nil
}
func setupOwnedDNS(mode DnsmasqMode, port int) error {
	l, err := openDNSLedger(true)
	if err != nil {
		return err
	}
	defer l.close()
	backend := "file"
	if _, err := exec.LookPath("uci"); err == nil {
		backend = "uci"
	}
	if l.state != nil && l.state.Backend != backend {
		return fmt.Errorf("dns: ownership backend changed; restore first")
	}
	if backend == "file" {
		return l.setupFiles(mode, port, dnsmasqManagedConfigPaths())
	}
	target := ""
	if l.state != nil {
		target = l.state.Target
	}
	id, current, err := dnsUCIRead(target)
	if err != nil {
		return err
	}
	if l.state == nil {
		l.state = &dnsJournal{Owner: dnsOwnerMarker, Backend: "uci", Target: id, Fields: map[string]*dnsField{}}
	}
	desired := map[string]dnsValue{"port": {Kind: "option", Values: []string{"0"}}}
	keys := []string{"port"}
	if mode == ModeUpstream {
		desired["port"] = dnsValue{Kind: "option", Values: []string{"53"}}
		desired["server"] = dnsValue{Kind: "list", Values: []string{fmt.Sprintf("127.0.0.1#%d", port)}}
		desired["noresolv"] = dnsValue{Kind: "option", Values: []string{"1"}}
		keys = append(keys, "server", "noresolv")
	}
	// Preflight everything, then persist ALL originals and this round's plan
	// before the first UCI mutation. A user edit is not a new owned baseline.
	for _, key := range keys {
		if field := l.state.Fields[key]; field != nil && !ownedDNSValue(field, current[key]) {
			return fmt.Errorf("dns: owned field changed externally; refusing takeover")
		}
	}
	for _, key := range keys {
		field := l.state.Fields[key]
		if field == nil {
			field = &dnsField{Original: current[key], Expected: current[key]}
			l.state.Fields[key] = field
		}
		field.Planned = desired[key]
	}
	if err = l.save(); err != nil {
		return err
	}
	for _, key := range keys {
		if err = l.writeField(key, current[key], desired[key]); err != nil {
			return err
		}
	}
	if err = dnsUCICommand("commit", "dhcp"); err != nil {
		return err
	}
	if err = restartDnsmasqFull(); err != nil {
		return err
	}
	if mode == ModeReplace {
		// procd may create its dnsmasq hijack table asynchronously after restart.
		time.Sleep(1500 * time.Millisecond)
		removeDnsmasqNftHijack()
	}
	return nil
}
func restoreOwnedDNS() error {
	l, err := openDNSLedger(false)
	if err != nil {
		return err
	}
	if l == nil {
		return nil
	}
	defer l.close()
	if l.state == nil {
		return nil
	}
	if l.state.Backend == "file" {
		return l.restoreFiles()
	}
	_, current, err := dnsUCIRead(l.state.Target)
	if err != nil {
		return err
	}
	for _, key := range []string{"port", "server", "noresolv"} {
		field := l.state.Fields[key]
		if field == nil {
			continue
		}
		value := current[key]
		if sameDNSValue(value, field.Original) || !ownedDNSValue(field, value) {
			continue
		}
		field.Planned = field.Original
		if err = l.save(); err != nil {
			return err
		}
		if err = l.writeField(key, value, field.Original); err != nil {
			return err
		}
	}
	// Always retry commit/restart while the journal exists, even if an earlier
	// attempt already restored every field before its commit or restart failed.
	if err = dnsUCICommand("commit", "dhcp"); err != nil {
		return err
	}
	if err = restartDnsmasqFull(); err != nil {
		return err
	}
	return l.release()
}

func readDNSConfig(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("dns: managed config is not a regular file")
	}
	b, err := os.ReadFile(path)
	return string(b), true, err
}
func ownedDNSConfig(file *dnsFile, content string) bool {
	return strings.HasPrefix(content, dnsManagedHeader) && (content == file.Expected || content == file.Pending)
}
func (l *dnsLedger) setupFiles(mode DnsmasqMode, port int, paths []string) error {
	content := dnsManagedHeader + "port=0\n"
	if mode == ModeUpstream {
		content = dnsManagedHeader + fmt.Sprintf("port=53\nserver=127.0.0.1#%d\nno-resolv\n", port)
	}
	if l.state == nil {
		l.state = &dnsJournal{Owner: dnsOwnerMarker, Backend: "file", Files: map[string]*dnsFile{}}
	}
	// Include previously acquired paths even if dnsmasq's conf-dir changed.
	for path := range l.state.Files {
		found := false
		for _, p := range paths {
			if p == path {
				found = true
			}
		}
		if !found {
			paths = append(paths, path)
		}
	}
	currentFiles := make(map[string]string)
	for _, path := range paths {
		old, exists, err := readDNSConfig(path)
		if err != nil {
			return err
		}
		file := l.state.Files[path]
		if exists && (file == nil || !ownedDNSConfig(file, old)) {
			return fmt.Errorf("dns: config collision or external edit; refusing overwrite")
		}
		if exists {
			currentFiles[path] = old
		}
	}
	for _, path := range paths {
		if l.state.Files[path] == nil {
			l.state.Files[path] = &dnsFile{}
		}
		if old, exists := currentFiles[path]; exists {
			l.state.Files[path].Expected = old
		}
		l.state.Files[path].Pending = content
	}
	if err := l.save(); err != nil {
		return err
	}
	for _, path := range paths {
		file := l.state.Files[path]
		old, exists, err := readDNSConfig(path)
		if err != nil {
			return err
		}
		if exists && !ownedDNSConfig(file, old) {
			return fmt.Errorf("dns: config changed before write")
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		f, e := os.CreateTemp(filepath.Dir(path), ".clashforge-")
		if e != nil {
			return e
		}
		name := f.Name()
		_, e = f.WriteString(content)
		if e == nil {
			e = f.Chmod(0644)
		}
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e == nil && !exists {
			// Publish complete content atomically, but never replace a collision.
			e = os.Link(name, path)
		} else if e == nil {
			// Compare immediately before replacing an already-owned file.
			now, present, re := readDNSConfig(path)
			if re != nil || !present || now != old {
				e = fmt.Errorf("dns: config changed during write")
			} else {
				e = os.Rename(name, path)
			}
		}
		os.Remove(name)
		if e != nil {
			return e
		}
		published, present, e := readDNSConfig(path)
		if e != nil {
			return e
		}
		if !present || published != content {
			return fmt.Errorf("dns: config write verification failed")
		}
		file.Expected = content
		file.Pending = ""
		if err = l.save(); err != nil {
			return err
		}
	}
	if err := restartDnsmasqFull(); err != nil {
		return err
	}
	if mode == ModeReplace {
		removeDnsmasqNftHijack()
	}
	return nil
}
func (l *dnsLedger) restoreFiles() error {
	for path, file := range l.state.Files {
		content, exists, err := readDNSConfig(path)
		if err != nil {
			return err
		}
		if exists && ownedDNSConfig(file, content) {
			if err = os.Remove(path); err != nil {
				return err
			}
		}
	}
	if err := restartDnsmasqFull(); err != nil {
		return err
	}
	return l.release()
}
