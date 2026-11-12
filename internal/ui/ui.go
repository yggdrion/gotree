package ui

import (
	"fmt"
	"strings"
	"sync"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/yggdrion/gotree/internal/repo"
)

const workers = 8

// gitSem caps concurrent git work across pools and actions.
var gitSem = make(chan struct{}, workers)

// inflight marks paths with a running action so fetch workers can skip them (read off the Update goroutine).
var inflight sync.Map

type kind int

const (
	kBack kind = iota
	kPull
	kRemove
	kReset
	kDelete
)

type job struct {
	kind    kind
	repo    *repo.Repo
	discard bool     // kBack: reset+clean first
	force   bool     // kRemove: --force
	files   []string // files the confirm dialog listed (destructive jobs)
}

type confirm struct {
	repo  *repo.Repo
	kind  kind // kBack = discard dialog, kRemove = remove-worktree dialog, kReset = reset-to-upstream dialog
	files []string
}

type (
	repoLoadedMsg struct{ r *repo.Repo }
	loadDoneMsg   struct{}
	fetchedMsg    struct {
		path string
		err  error
		r    *repo.Repo
		skip bool // busy, no git call made
	}
	reloadedMsg struct {
		path string
		r    *repo.Repo
	}
	actionDoneMsg struct {
		path, top, verb string
		err             error
		r               *repo.Repo // freshly loaded top-level repo that contains path
	}
)

type model struct {
	root     string
	paths    []string
	repos    []*repo.Repo
	cursor   string // path of the cursor row
	idx, off int
	w, h     int
	busy     map[string]bool
	sel      map[string]bool
	errs     map[string]string // per-row short error, shown in STATE
	status   string
	statusOK bool
	queue    []confirm
	pending  []job
	filter   textinput.Model
	filterOn bool // filter input has focus
	help     bool
	sort     repo.SortMode
	menu     *menuState
	diff     *diffState
	chosen   string
	out      chan tea.Msg

	loading                bool
	fetchDone, fetchTotal  int
	reloadDone, reloadLeft int
}

func newModel(root string) model {
	ti := textinput.New()
	ti.Prompt = "/ "
	return model{
		root: root, paths: repo.QuickOrder(root), loading: true, filter: ti,
		busy: map[string]bool{}, sel: map[string]bool{}, errs: map[string]string{},
		out: make(chan tea.Msg, 64),
	}
}

func (m model) Chosen() string { return m.chosen }

// runPool feeds paths in order to `workers` goroutines and blocks until all are done.
func runPool(paths []string, fn func(string) tea.Msg, out chan<- tea.Msg) {
	ch := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range ch {
				gitSem <- struct{}{}
				msg := fn(p)
				<-gitSem
				out <- msg
			}
		}()
	}
	for _, p := range paths {
		ch <- p
	}
	close(ch)
	wg.Wait()
}

func listen(out <-chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-out } }

func (m model) Init() tea.Cmd {
	go func() {
		runPool(m.paths, func(p string) tea.Msg { return repoLoadedMsg{repo.LoadRepo(p)} }, m.out)
		m.out <- loadDoneMsg{}
	}()
	return listen(m.out)
}

// planBatch splits targets into clean ones and ones with uncommitted files, keeping order.
func planBatch(targets []*repo.Repo) (clean, dirty []*repo.Repo) {
	for _, r := range targets {
		if len(r.Status.Files) > 0 {
			dirty = append(dirty, r)
		} else {
			clean = append(clean, r)
		}
	}
	return
}

// replace swaps in a freshly loaded repo, keeping fold state of it and its worktrees.
func (m *model) replace(nr *repo.Repo) {
	if nr == nil {
		return
	}
	for i, old := range m.repos {
		if !repo.SamePath(old.Path, nr.Path) {
			continue
		}
		nr.Expanded = old.Expanded
		repo.SortRepos(nr.Worktrees, m.sort)
		for _, w := range nr.Worktrees {
			for _, ow := range old.Worktrees {
				if repo.SamePath(ow.Path, w.Path) {
					w.Expanded = ow.Expanded
				}
			}
		}
		for _, ow := range old.Worktrees {
			if !hasPath(nr, ow.Path) {
				delete(m.sel, ow.Path)
				delete(m.errs, ow.Path)
			}
		}
		m.repos[i] = nr
		return
	}
}

func hasPath(r *repo.Repo, p string) bool {
	for _, w := range r.Worktrees {
		if repo.SamePath(w.Path, p) {
			return true
		}
	}
	return false
}

// cursorError is the full error of the cursor row, lines joined with " · ".
func (m *model) cursorError() string {
	rs := m.rows()
	if len(rs) == 0 {
		return ""
	}
	r := rs[min(m.idx, len(rs)-1)].Repo
	e := m.errs[r.Path]
	if e == "" {
		e = r.Status.Err
	}
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.TrimSpace(e), "\n", " · ")), " ")
}

// drop removes a top-level row that is no longer a repo, and its selection/busy/error state.
func (m *model) drop(path string) {
	for i, r := range m.repos {
		if repo.SamePath(r.Path, path) {
			m.repos = append(m.repos[:i:i], m.repos[i+1:]...)
			delete(m.sel, r.Path)
			delete(m.busy, r.Path)
			delete(m.errs, r.Path)
			return
		}
	}
}

// topPath is the path of the top-level repo holding row path p.
func (m *model) topPath(p string) string {
	for _, r := range m.repos {
		if repo.SamePath(r.Path, p) {
			return r.Path
		}
		for _, w := range r.Worktrees {
			if repo.SamePath(w.Path, p) {
				return r.Path
			}
		}
	}
	return p
}

func (m *model) all() []*repo.Repo {
	var out []*repo.Repo
	for _, r := range m.repos {
		out = append(out, r)
		out = append(out, r.Worktrees...)
	}
	return out
}

// resort orders repos and their worktrees by the current mode; the cursor follows its path.
func (m *model) resort() {
	repo.SortRepos(m.repos, m.sort)
	for _, r := range m.repos {
		repo.SortRepos(r.Worktrees, m.sort)
	}
	m.fix()
}

func (m *model) rows() []row { return visibleRows(m.repos, m.filter.Value()) }

// fix resolves the cursor path to an index, clamps it, and scrolls it into view.
func (m *model) fix() {
	rs := m.rows()
	if len(rs) == 0 {
		m.idx, m.off = 0, 0
		return
	}
	found := false
	for i, r := range rs {
		if r.Repo.Path == m.cursor {
			m.idx, found = i, true
			break
		}
	}
	if !found {
		m.idx = min(m.idx, len(rs)-1)
		m.cursor = rs[m.idx].Repo.Path
	}
	lh := max(1, m.listH())
	if m.idx < m.off {
		m.off = m.idx
	}
	if m.idx >= m.off+lh {
		m.off = m.idx - lh + 1
	}
	m.off = max(0, min(m.off, max(0, len(rs)-lh)))
}

func (m *model) move(d int) {
	rs := m.rows()
	if len(rs) == 0 {
		return
	}
	i := max(0, min(m.idx+d, len(rs)-1))
	m.idx, m.cursor = i, rs[i].Repo.Path
}

func (m *model) setStatus(ok bool, f string, a ...any) {
	m.status, m.statusOK = fmt.Sprintf(f, a...), ok
}

func (m *model) startFetch() {
	paths := make([]string, len(m.repos))
	for i, r := range m.repos {
		paths[i] = r.Path
	}
	m.fetchDone, m.fetchTotal = 0, len(paths)
	out := m.out
	go runPool(paths, func(p string) tea.Msg {
		if _, busy := inflight.Load(p); busy {
			return fetchedMsg{path: p, skip: true}
		}
		err := repo.Fetch(p)
		return fetchedMsg{path: p, err: err, r: repo.LoadRepo(p)}
	}, out)
}

func (m *model) startReload() {
	paths := make([]string, len(m.repos))
	for i, r := range m.repos {
		paths[i] = r.Path
	}
	m.reloadDone, m.reloadLeft = 0, len(paths)
	go runPool(paths, func(p string) tea.Msg { return reloadedMsg{p, repo.LoadRepo(p)} }, m.out)
}

func (m *model) run(j job) tea.Cmd {
	p := j.repo.Path
	m.busy[p] = true
	delete(m.errs, p)
	top := m.topPath(p)
	inflight.Store(p, true)
	inflight.Store(top, true)
	verb := map[kind]string{kBack: "back to default", kPull: "pull", kRemove: "remove worktree", kReset: "reset to upstream", kDelete: "delete repo"}[j.kind]
	return func() tea.Msg {
		gitSem <- struct{}{}
		defer func() { <-gitSem }()
		var err error
		if (j.kind == kBack && j.discard) || (j.kind == kRemove && j.force) || j.kind == kReset {
			err = repo.CheckConfirmed(j.repo.Path, j.files)
		}
		switch {
		case err != nil:
		case j.kind == kBack:
			err = repo.BackToDefault(j.repo, j.discard)
		case j.kind == kPull:
			err = repo.Pull(j.repo)
		case j.kind == kReset:
			err = repo.ResetToUpstream(j.repo)
		case j.kind == kRemove:
			err = repo.RemoveWorktree(j.repo, j.force)
		case j.kind == kDelete:
			err = repo.DeleteRepo(j.repo, j.files)
		}
		return actionDoneMsg{p, top, verb, err, repo.LoadRepo(top)}
	}
}

// advance starts all pending jobs once no dialog is waiting.
func (m *model) advance() tea.Cmd {
	if len(m.queue) > 0 || len(m.pending) == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for _, j := range m.pending {
		cmds = append(cmds, m.run(j))
	}
	m.pending = nil
	return tea.Batch(cmds...)
}

func (m *model) targets() []*repo.Repo {
	var out []*repo.Repo
	for _, r := range m.all() {
		if m.sel[r.Path] {
			out = append(out, r)
		}
	}
	if len(out) > 0 {
		return out
	}
	if rs := m.rows(); len(rs) > 0 {
		return []*repo.Repo{rs[m.idx].Repo}
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	m.fix()
	return m, cmd
}

func (m *model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case repoLoadedMsg:
		if msg.r != nil {
			m.repos = repo.Dedupe(append(m.repos, msg.r))
		}
		return listen(m.out)
	case loadDoneMsg:
		m.loading = false
		m.resort()
		m.startFetch()
		return listen(m.out)
	case fetchedMsg:
		m.fetchDone++
		if msg.skip {
			return listen(m.out)
		}
		// fetch errors are replaced/cleared by the next fetch; action errors survive it
		if old, had := m.errs[msg.path]; !had || strings.HasPrefix(old, "fetch:") {
			if msg.err != nil {
				m.errs[msg.path] = msg.err.Error()
			} else {
				delete(m.errs, msg.path)
			}
		}
		if !m.busy[msg.path] {
			m.replace(msg.r)
		}
		return listen(m.out)
	case reloadedMsg:
		if !m.busy[msg.path] {
			if msg.r == nil {
				m.drop(msg.path)
			} else {
				m.replace(msg.r)
			}
		}
		if m.reloadDone++; m.reloadDone >= m.reloadLeft {
			m.resort()
			m.reloadLeft = 0
		}
		return listen(m.out)
	case actionDoneMsg:
		delete(m.busy, msg.path)
		inflight.Delete(msg.path)
		inflight.Delete(msg.top)
		if msg.r == nil {
			m.drop(msg.top)
		} else {
			m.replace(msg.r)
		}
		if msg.err != nil {
			m.errs[msg.path] = msg.err.Error()
			m.setStatus(false, "%s: %s", msg.verb, msg.err)
		} else {
			m.setStatus(true, "%s: done", msg.verb)
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return nil
}

func (m *model) key(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if k.Mod == tea.ModShift && k.Text == "" && k.ShiftedCode > ' ' {
		s = string(k.ShiftedCode) // kitty keyboards report "shift+r" with no text for R
	}
	if s == "ctrl+c" {
		return tea.Quit
	}
	if len(m.queue) > 0 {
		c := m.queue[0]
		m.queue = m.queue[1:]
		switch s {
		case "y":
			m.pending = append(m.pending, job{kind: c.kind, repo: c.repo, discard: true, force: len(c.files) > 0, files: c.files})
		case "esc":
			m.queue, m.pending = nil, nil
			m.setStatus(false, "cancelled")
		default:
			m.setStatus(false, "skipped %s", c.repo.Name)
		}
		return m.advance()
	}
	if m.diff != nil {
		m.diffKey(s)
		return nil
	}
	if m.menu != nil {
		return m.menuKey(s)
	}
	if m.help {
		switch s {
		case "esc", "q", "?":
			m.help = false
		}
		return nil
	}
	if m.filterOn {
		switch s {
		case "esc":
			m.filter.SetValue("")
			m.filter.Blur()
			m.filterOn = false
		case "enter":
			m.filter.Blur()
			m.filterOn = false
		default:
			var c tea.Cmd
			m.filter, c = m.filter.Update(k)
			return c
		}
		return nil
	}
	rs := m.rows()
	var cur *repo.Repo
	if len(rs) > 0 {
		cur = rs[m.idx].Repo
	}
	switch s {
	case "q":
		return tea.Quit
	case "esc":
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			return nil
		}
		return tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "?":
		m.help = true
	case "/":
		m.filterOn = true
		return m.filter.Focus()
	case "f":
		if m.loading || m.fetchDone < m.fetchTotal {
			m.setStatus(false, "still loading or fetching")
			return nil
		}
		m.startFetch()
	case "s":
		m.sort = m.sort.Next()
		m.resort()
		m.setStatus(true, "sorted by %s", m.sort)
	case "r":
		if m.loading || m.reloadLeft > 0 {
			m.setStatus(false, "still loading")
			return nil
		}
		m.startReload()
	}
	if cur == nil {
		return nil
	}
	switch s {
	case "enter":
		m.openMenu(cur)
	case "x":
		return m.cd(cur)
	case "c":
		m.openEditor(cur)
	case "space":
		m.toggleSel(cur)
	case "right", "l":
		if !rs[m.idx].Child && len(cur.Worktrees) > 0 {
			cur.Expanded = true
		}
	case "left", "h":
		if rs[m.idx].Child {
			for i := m.idx - 1; i >= 0; i-- {
				if !rs[i].Child {
					m.idx, m.cursor = i, rs[i].Repo.Path
					break
				}
			}
		} else {
			cur.Expanded = false
		}
	case "d":
		m.removeWT(cur)
	case "R":
		m.resetUpstream(cur)
		return m.advance()
	case "v":
		m.openDiff(cur)
	case "b":
		return m.dispatch(kBack)
	case "p":
		return m.dispatch(kPull)
	case "P":
		return m.pullAll()
	}
	return nil
}

// simplePull reports whether r can be fast-forwarded with nothing to lose:
// clean, on a branch with a live upstream, behind and not ahead.
func simplePull(r *repo.Repo) bool {
	s := r.Status
	return !s.Detached && s.Upstream != "" && !s.Gone && s.Behind > 0 && s.Ahead == 0 && len(s.Files) == 0 && s.Err == ""
}

// pullAll pulls every repo and worktree where a simple fast-forward is available.
func (m *model) pullAll() tea.Cmd {
	n := 0
	for _, r := range m.all() {
		if m.busy[r.Path] || !simplePull(r) {
			continue
		}
		m.pending = append(m.pending, job{kind: kPull, repo: r})
		n++
	}
	if n == 0 {
		m.setStatus(false, "pull all: nothing behind that can fast-forward")
		return nil
	}
	m.setStatus(true, "pull all: %d repos", n)
	return m.advance()
}

// dispatch handles b and p on the selection (or cursor row); refusals only set the status line.
func (m *model) dispatch(k kind) tea.Cmd {
	var targets []*repo.Repo
	var note string
	for _, r := range m.targets() {
		switch {
		case m.busy[r.Path]:
			note = r.Name + " is busy"
		case k == kBack && r.IsWorktree:
			note = "b: not allowed on a worktree"
		case k == kBack && r.Status.Default == "":
			note = "b: " + r.Name + ": unknown default branch"
		case k == kPull && r.Status.Upstream == "":
			note = "p: " + r.Name + ": no upstream"
		case k == kPull && r.Status.Gone:
			note = "p: " + r.Name + ": upstream gone"
		default:
			targets = append(targets, r)
		}
	}
	if len(targets) == 0 {
		if note == "" {
			note = "nothing to do"
		}
		m.setStatus(false, "%s", note)
		return nil
	}
	if note != "" {
		m.setStatus(false, "skipped: %s", note)
	}
	m.sel = map[string]bool{}
	if k == kPull {
		for _, r := range targets {
			m.pending = append(m.pending, job{kind: kPull, repo: r})
		}
		return m.advance()
	}
	clean, dirty := planBatch(targets)
	for _, r := range dirty {
		m.queue = append(m.queue, confirm{r, kBack, r.Status.Files})
	}
	for _, r := range clean {
		m.pending = append(m.pending, job{kind: kBack, repo: r})
	}
	return m.advance()
}

func (m model) fetching() bool { return m.fetchDone < m.fetchTotal }

// Run starts the TUI on root and returns the chosen path ("" if none).
func Run(root string) (string, error) {
	final, err := tea.NewProgram(newModel(root)).Run()
	if err != nil {
		return "", err
	}
	if m, ok := final.(model); ok {
		return m.Chosen(), nil
	}
	return "", nil
}
