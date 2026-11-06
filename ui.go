package main

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const workers = 8

type kind int

const (
	kBack kind = iota
	kPull
	kRemove
)

type job struct {
	kind    kind
	repo    *Repo
	discard bool // kBack: reset+clean first
	force   bool // kRemove: --force
}

type confirm struct {
	repo  *Repo
	kind  kind // kBack = discard dialog, kRemove = remove-worktree dialog
	files []string
}

type (
	repoLoadedMsg struct{ r *Repo }
	loadDoneMsg   struct{}
	fetchedMsg    struct {
		path string
		err  error
		r    *Repo
	}
	reloadedMsg struct {
		path string
		r    *Repo
	}
	actionDoneMsg struct {
		path, verb string
		err        error
		r          *Repo // freshly loaded top-level repo that contains path
	}
)

type model struct {
	root     string
	paths    []string
	repos    []*Repo
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
		root: root, paths: QuickOrder(root), loading: true, filter: ti,
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
				out <- fn(p)
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
		runPool(m.paths, func(p string) tea.Msg { return repoLoadedMsg{LoadRepo(p)} }, m.out)
		m.out <- loadDoneMsg{}
	}()
	return listen(m.out)
}

// planBatch splits targets into clean ones and ones with uncommitted files, keeping order.
func planBatch(targets []*Repo) (clean, dirty []*Repo) {
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
func (m *model) replace(nr *Repo) {
	if nr == nil {
		return
	}
	for i, old := range m.repos {
		if !samePath(old.Path, nr.Path) {
			continue
		}
		nr.Expanded = old.Expanded
		for _, w := range nr.Worktrees {
			for _, ow := range old.Worktrees {
				if samePath(ow.Path, w.Path) {
					w.Expanded = ow.Expanded
				}
			}
		}
		m.repos[i] = nr
		return
	}
}

// topPath is the path of the top-level repo holding row path p.
func (m *model) topPath(p string) string {
	for _, r := range m.repos {
		if r.Path == p {
			return p
		}
		for _, w := range r.Worktrees {
			if w.Path == p {
				return r.Path
			}
		}
	}
	return p
}

func (m *model) all() []*Repo {
	var out []*Repo
	for _, r := range m.repos {
		out = append(out, r)
		out = append(out, r.Worktrees...)
	}
	return out
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
		err := Fetch(p)
		return fetchedMsg{p, err, LoadRepo(p)}
	}, out)
}

func (m *model) startReload() {
	paths := make([]string, len(m.repos))
	for i, r := range m.repos {
		paths[i] = r.Path
	}
	m.reloadDone, m.reloadLeft = 0, len(paths)
	go runPool(paths, func(p string) tea.Msg { return reloadedMsg{p, LoadRepo(p)} }, m.out)
}

func (m *model) run(j job) tea.Cmd {
	p := j.repo.Path
	m.busy[p] = true
	delete(m.errs, p)
	top := m.topPath(p)
	verb := map[kind]string{kBack: "back to default", kPull: "pull", kRemove: "remove worktree"}[j.kind]
	return func() tea.Msg {
		var err error
		switch j.kind {
		case kBack:
			err = BackToDefault(j.repo, j.discard)
		case kPull:
			err = Pull(j.repo)
		case kRemove:
			err = RemoveWorktree(j.repo, j.force)
		}
		return actionDoneMsg{p, verb, err, LoadRepo(top)}
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

func (m *model) targets() []*Repo {
	if len(m.sel) > 0 {
		var out []*Repo
		for _, r := range m.all() {
			if m.sel[r.Path] {
				out = append(out, r)
			}
		}
		return out
	}
	if rs := m.rows(); len(rs) > 0 {
		return []*Repo{rs[m.idx].Repo}
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
			m.repos = Dedupe(append(m.repos, msg.r))
		}
		return listen(m.out)
	case loadDoneMsg:
		m.loading = false
		SortRepos(m.repos)
		m.startFetch()
		return listen(m.out)
	case fetchedMsg:
		m.fetchDone++
		if msg.err != nil {
			m.errs[msg.path] = firstLine(msg.err)
		} else {
			delete(m.errs, msg.path)
		}
		if !m.busy[msg.path] {
			m.replace(msg.r)
		}
		return listen(m.out)
	case reloadedMsg:
		if !m.busy[msg.path] {
			m.replace(msg.r)
		}
		if m.reloadDone++; m.reloadDone >= m.reloadLeft {
			SortRepos(m.repos)
			m.reloadLeft = 0
		}
		return listen(m.out)
	case actionDoneMsg:
		delete(m.busy, msg.path)
		m.replace(msg.r)
		if msg.err != nil {
			m.errs[msg.path] = msg.err.Error()
			m.setStatus(false, "%s: %s", msg.verb, msg.err)
		} else {
			m.setStatus(true, "%s: done", msg.verb)
		}
	case tea.KeyMsg:
		return m.key(msg)
	}
	return nil
}

func (m *model) key(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "ctrl+c" {
		return tea.Quit
	}
	if len(m.queue) > 0 {
		c := m.queue[0]
		m.queue = m.queue[1:]
		switch s {
		case "y":
			m.pending = append(m.pending, job{kind: c.kind, repo: c.repo, discard: true, force: len(c.files) > 0})
		case "esc":
			m.queue, m.pending = nil, nil
			m.setStatus(false, "cancelled")
		default:
			m.setStatus(false, "skipped %s", c.repo.Name)
		}
		return m.advance()
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
	var cur *Repo
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
		m.help = !m.help
	case "/":
		m.filterOn = true
		return m.filter.Focus()
	case "f":
		if m.loading || m.fetchDone < m.fetchTotal {
			m.setStatus(false, "still loading or fetching")
			return nil
		}
		m.startFetch()
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
		m.chosen = cur.Path
		return tea.Quit
	case "c":
		if err := OpenEditor(cur.Path); err != nil {
			m.setStatus(false, "%s", err)
		} else {
			m.setStatus(true, "opened %s in VS Code", cur.Name)
		}
	case "x":
		if m.sel[cur.Path] {
			delete(m.sel, cur.Path)
		} else {
			m.sel[cur.Path] = true
		}
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
		if !cur.IsWorktree {
			m.setStatus(false, "d: only on worktree rows")
		} else if m.busy[cur.Path] {
			m.setStatus(false, "%s is busy", cur.Name)
		} else {
			m.queue = append(m.queue, confirm{cur, kRemove, cur.Status.Files})
		}
	case "b":
		return m.dispatch(kBack)
	case "p":
		return m.dispatch(kPull)
	}
	return nil
}

// dispatch handles b and p on the selection (or cursor row); refusals only set the status line.
func (m *model) dispatch(k kind) tea.Cmd {
	var targets []*Repo
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
