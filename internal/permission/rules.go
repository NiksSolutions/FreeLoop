package permission

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"slices"
	"strings"
	"unicode"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// Effect of a rule.
type Effect string

const (
	Allow Effect = "allow"
	Deny  Effect = "deny"
)

// Scope is where a rule lives (SEC-002). M1 has global (rules.yaml in the
// app data directory) and session; environment, profile and project scopes
// arrive with those features.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeSession Scope = "session"
	// ScopeEnvironment rules come from an environment file (SEC-002).
	ScopeEnvironment Scope = "environment"
	// ScopeProfile rules come from a harness profile (HRN-003).
	ScopeProfile Scope = "profile"
	// ScopeProject rules come from a trusted project's .freeloop/ and can
	// only deny (SET-002).
	ScopeProject Scope = "project"
)

// Rule is Freeloop's canonical rule (OQ-010): a tool, and optionally a
// command or a path pattern.
//
//   - tool: a tool name such as shell or write_file, or a pattern such as
//     mcp__github__* or *.
//   - command (shell): an exact command, or a prefix written "git *", which
//     matches "git" and anything starting with "git ".
//   - path (file tools): a glob with ** for any depth, relative to the
//     project root, or absolute.
//   - literal: the command or path is matched as written, never as a
//     pattern. "Allow always" sets it, so approving `rm -rf *` or a file
//     named `**` approves exactly that and nothing more.
//
// A rule with neither command nor path matches every call of its tools.
//
// Shell deny rules see through common wrappers (variable assignments, env,
// command, exec, a leading backslash, subshells, sh -c '...', cmd /c and
// PowerShell's -Command) and read the program by name whatever its path,
// Windows extension or letter case (programName), but a shell
// can always spell a command another way; until shell commands run in a
// sandbox, deny rules for shell are a guard against mistakes, not against a
// determined agent.
type Rule struct {
	Effect  Effect `yaml:"effect" json:"effect"`
	Tool    string `yaml:"tool" json:"tool"`
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty"`
	Literal bool   `yaml:"literal,omitempty" json:"literal,omitempty"`
	// Scope and Source are filled in by whoever loads the rule.
	Scope  Scope  `yaml:"-" json:"scope"`
	Source string `yaml:"-" json:"source"`
	// Pending marks an allow rule from a file the user has not approved, as the
	// app lists it (SET-005). The broker never sees one.
	Pending bool `yaml:"-" json:"pending,omitempty"`
}

// Validate reports a rule that cannot match anything sensible.
func (r Rule) Validate() error {
	switch {
	case r.Effect != Allow && r.Effect != Deny:
		return fmt.Errorf("effect must be allow or deny, not %q", r.Effect)
	case strings.TrimSpace(r.Tool) == "":
		return errors.New("tool is required")
	case r.Command != "" && r.Path != "":
		return errors.New("a rule has a command or a path, not both")
	}
	if _, err := path.Match(r.Tool, ""); err != nil {
		return fmt.Errorf("bad tool pattern %q: %w", r.Tool, err)
	}
	return nil
}

// String renders a rule the way the UI and decisions show it.
func (r Rule) String() string {
	s := string(r.Effect) + " " + r.Tool
	q := ""
	if r.Literal {
		q = "="
	}
	switch {
	case r.Command != "":
		s += fmt.Sprintf("(%s%s)", q, r.Command)
	case r.Path != "":
		s += fmt.Sprintf("(%s%s)", q, r.Path)
	}
	return s
}

// shellOperators split a command line into the commands it runs.
var shellOperators = []string{"&&", "||", ";", "|", "&", "\n"}

// expanding are the characters outside single quotes that let a shell run
// or write more than the words of a line: substitutions and expansions
// ($, backticks, zsh's =(...) and ${(e)...}), redirections, groups and
// zsh glob qualifiers such as *(e:...:), and escapes.
const expanding = "$`<>(){}\\"

// plain reports whether a line is only words a prefix allow rule can
// approve: no expanding character outside single quotes, and none of $,
// backticks or backslashes inside double quotes, where the shell still
// expands them. zsh, the default shell, and sh agree on both. Anything
// else asks.
func plain(cmd string) bool {
	var quote rune
	for _, c := range cmd {
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if strings.ContainsRune("$`\\", c) {
				return false
			}
		case c == '\'' || c == '"':
			quote = c
		case strings.ContainsRune(expanding, c):
			return false
		}
	}
	return quote == 0
}

func normalize(cmd string) string { return strings.Join(strings.Fields(cmd), " ") }

var blanks = regexp.MustCompile(`[ \t]+`)

// exactForm collapses runs of spaces and tabs but keeps newlines, which the
// shell treats as command separators.
func exactForm(cmd string) string { return strings.TrimSpace(blanks.ReplaceAllString(cmd, " ")) }

// wrappers run the command that follows them, with the options that take a
// value, which must not be read as the command. env's -S is read on its own (envSplit).
var wrappers = map[string]string{
	"env": "uCPLU", "command": "", "exec": "a", "nohup": "", "time": "fo", "builtin": "", "nice": "n", "sudo": "ughpCrtUD", "doas": "uC",
	"ionice": "cnp", "stdbuf": "ioe", "caffeinate": "tw", "xargs": "EILPdnsJR", "watch": "n", "timeout": "ks",
	// zsh's precommand modifiers, which run what follows them, and its repeat, which
	// runs it a number of times.
	"noglob": "", "nocorrect": "", "-": "", "repeat": "",
	// Multi-call programs, which run the program they name: busybox sh -c '…'.
	"busybox": "", "toybox": "",
	// A token optimizer's front end (HRN-019).
	"rtk": "",
}

// rtkRunners are rtk's commands that run the command after them.
var rtkRunners = []string{"proxy", "bypass", "run", "err", "test", "summary"}

// keywords come before a command without being one. time is a wrapper, so that its
// options are not read as the command: time -p curl x runs curl.
var keywords = []string{"!", "if", "then", "elif", "else", "do", "while", "until"}

// shells run the line after -c, or the commands they read from their input. The
// program word has been through programName, so /usr/bin/BASH and bash.exe are bash.
var shells = []string{"sh", "bash", "zsh", "dash", "ksh", "ash", "mksh", "yash", "fish", "csh", "tcsh"}

// shellValued are the shells' long options that take a separate value.
var shellValued = []string{"rcfile", "init-file", "emulate", "debug", "debug-output", "profile", "features"}

// shellLine returns the line a shell runs with -c (fish's --command, and its -C, which
// runs before what it reads), and whether it reads its commands from its input instead,
// which no rule sees: with neither -c nor a script, with -s, or with - where the script
// would be (echo 'git push' | sh, sh <<< …). The options that take a value (-o, +o, -O,
// +O, --rcfile) are skipped with it, so bash -O extglob -c 'git push' runs git push. A
// script runs its own lines, as a Makefile does.
func shellLine(shell string, words []string) (line string, fromInput bool) {
	command, stdin := false, false
	i := 0
options:
	for ; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "--" || w == "-":
			i++
			break options
		case strings.HasPrefix(w, "--"):
			name, value, attached := strings.Cut(w[2:], "=")
			switch {
			case (name == "command" || name == "init-command") && attached:
				return strings.Join(append([]string{value}, words[i+1:]...), " "), false
			case name == "command" || name == "init-command":
				command = true
			case name == "version" || name == "help":
				return "", false
			case slices.Contains(shellValued, name) && !attached:
				i++
			}
		case len(w) > 1 && (w[0] == '-' || w[0] == '+'):
			command = command || (w[0] == '-' && (strings.ContainsRune(w[1:], 'c') || (shell == "fish" && strings.ContainsRune(w[1:], 'C'))))
			stdin = stdin || (w[0] == '-' && strings.ContainsRune(w[1:], 's'))
			i += strings.Count(w[1:], "o") + strings.Count(w[1:], "O")
		default:
			break options
		}
	}
	switch {
	case command:
		return strings.Join(words[min(i, len(words)):], " "), false
	case stdin || i >= len(words):
		return "", true
	}
	return "", false
}

// envSplit reads env's -S (--split-string), which splits the string after it into the
// command env runs: env -S 'git push' runs git push. It returns what of the string is in
// opt itself (-S'git push'), and false for an option that is not -S.
func envSplit(opt string) (string, bool) {
	if long, ok := strings.CutPrefix(opt, "--"); ok {
		name, value, _ := strings.Cut(long, "=")
		return value, name != "" && strings.HasPrefix("split-string", name)
	}
	for i, c := range opt[1:] {
		switch {
		case c == 'S':
			return opt[2+i:], true
		case strings.ContainsRune(wrappers["env"], c):
			return "", false // the rest of the word is this option's value
		}
	}
	return "", false
}

// redirection is a word that redirects a command's input or output: an optional
// descriptor, the operator, and the target when it is in the same word.
var redirection = regexp.MustCompile(`^(?:[0-9]+|\{[A-Za-z_][A-Za-z0-9_]*\})?(?:<<<|<<-|<<|<>|<&|>&|>>!|>>|>\||>!|&>>|&>|<|>)(.*)$`)

// withoutRedirections takes the redirections out of a command's words, with their
// targets, as the shell does before it runs it, wherever they stand: </dev/null git
// push, 2>&1 git push and git 2>/dev/null push all run git push.
func withoutRedirections(words []string) []string {
	var out []string
	for i := 0; i < len(words); i++ {
		m := redirection.FindStringSubmatch(words[i])
		switch {
		case m == nil:
			out = append(out, words[i])
		case m[1] != "":
			i = wordEnd(words, i, m[1])
		case i+1 < len(words):
			i = wordEnd(words, i+1, words[i+1])
		}
	}
	return out
}

// wordEnd returns the index of the field that ends the shell word whose text in
// fields[i] starts with text: a quote opened there closes in a later field, or at the
// end of the line ('git push' after <<< is one word).
func wordEnd(fields []string, i int, text string) int {
	var quote rune
	escaped := false
	for {
		for _, c := range text {
			switch {
			case escaped:
				escaped = false
			case c == '\\' && quote != '\'':
				escaped = true
			case quote == 0 && (c == '\'' || c == '"'):
				quote = c
			case c == quote:
				quote = 0
			}
		}
		if quote == 0 || i+1 >= len(fields) {
			return i
		}
		i++
		text = " " + fields[i]
	}
}

// findRunners are find's actions that run the command after them, up to ; or +.
var findRunners = []string{"-exec", "-execdir", "-ok", "-okdir"}

// programName is how a deny rule reads a program word, so that every spelling of one
// program is one name: the last name of a path split at either slash, without the
// extension Windows runs it by (.exe, .cmd, .bat, .com), in lower case, since macOS's
// and Windows' volumes find GIT and Git.EXE as git. Only deny rules read it: a name
// that matches more makes a deny rule stop more, while an allow rule matched this way
// would approve programs the user never named, so allow rules compare as written.
func programName(word string) string {
	if i := strings.LastIndexAny(word, `/\`); i >= 0 {
		word = word[i+1:]
	}
	word = strings.ToLower(word)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".com"} {
		if s, ok := strings.CutSuffix(word, ext); ok && s != "" {
			return s
		}
	}
	return word
}

// programNames are the names a raw program word can run as: as the shell reads it,
// where a backslash escapes, and, when it has one, as Windows reads it, where a
// backslash separates folders ('C:\Git\cmd\git.exe').
func programNames(raw string) []string {
	names := []string{programName(strings.TrimPrefix(unquoted(raw), "="))}
	if i := strings.LastIndex(raw, `\`); i >= 0 {
		if alt := programName(unquoted(raw[i+1:])); alt != "" && alt != names[0] {
			names = append(names, alt)
		}
	}
	return names
}

// denyPattern is a deny rule's command with its program read as programName reads
// the command's, so `deny Git.exe push *` is `deny git push *`.
func denyPattern(pattern string) string {
	f := strings.Fields(pattern)
	if len(f) == 0 || f[0] == "*" {
		return pattern
	}
	f[0] = programName(strings.TrimPrefix(unquoted(f[0]), "="))
	return strings.Join(f, " ")
}

// Windows' command interpreters, whose line a deny rule must read too.
var (
	cmdShells = []string{"cmd"}
	psShells  = []string{"powershell", "pwsh", "powershell_ise"}
)

// cmdLine returns the line cmd /c (or /k, or Git Bash's //c) runs, and whether cmd
// hides from the rules what it runs: a line read from its input, a switch not known,
// escapes (^), variables (%x%, !x!), and its own launchers (call, start), loops and
// conditions, which run a command a rule cannot see as the program.
func cmdLine(words []string) (inner string, hidden bool) {
	for i, w := range words {
		w = unquoted(w)
		s := strings.TrimLeft(w, "/")
		if s == w || s == "" {
			return "", true
		}
		switch unicode.ToLower(rune(s[0])) {
		case 'c', 'k', 'r':
			inner = strings.Join(append([]string{s[1:]}, words[i+1:]...), " ")
			if strings.TrimSpace(inner) == "" || strings.ContainsAny(inner, "^%!") {
				return inner, true
			}
			for _, seg := range segments(inner) {
				f := strings.Fields(strings.Trim(seg, "(){} \t'\""))
				if len(f) > 0 && slices.Contains([]string{"call", "start", "if", "for"}, strings.ToLower(unquoted(f[0]))) {
					return inner, true
				}
			}
			return inner, false
		case 'd', 'q', 's', 'a', 'u', 'e', 'f', 'v', 't':
		default:
			return "", true
		}
	}
	// Without /c, cmd runs what it reads, which no rule sees: echo git push | cmd.
	return "", true
}

// quotedOperator reports a shell operator inside quotes in line.
func quotedOperator(line string) bool {
	var quote rune
	for _, c := range line {
		switch {
		case quote != 0 && c == quote:
			quote = 0
		case quote != 0 && strings.ContainsRune("&|;\n", c):
			return true
		case quote == 0 && (c == '\'' || c == '"'):
			quote = c
		}
	}
	return false
}

// psNoValue and psValue are PowerShell's parameters before its command, which may be
// shortened to any prefix (OQ-044 runs commands through Git Bash, but a command can
// start PowerShell itself).
var (
	psNoValue = []string{"noprofile", "nologo", "noninteractive", "noexit", "sta", "mta", "login", "interactive", "nop", "noni", "l", "i"}
	psValue   = []string{"executionpolicy", "ex", "ep", "windowstyle", "w", "inputformat", "if", "outputformat", "o", "of", "workingdirectory", "wd", "configurationname", "settingsfile", "version", "v", "psconsolefile", "configurationfile", "custompipename"}
	// psHiders run or rename a command a rule cannot see as the program.
	psHiders = []string{".", "&", "iex", "invoke-expression", "start-process", "saps", "start", "invoke-command", "icm", "invoke-item", "ii",
		"set-alias", "sal", "new-alias", "nal", "set-item", "si", "new-item", "ni", "start-job", "sajb", "start-threadjob", "invoke-restmethod", "irm", "-encodedcommand"}
)

func psParam(name string, list []string) bool {
	return slices.ContainsFunc(list, func(full string) bool {
		return name == full || (len(name) >= 3 && strings.HasPrefix(full, name))
	})
}

// psLine returns the line PowerShell runs with -Command (or with no parameter before
// it, which Windows PowerShell reads as a command), and whether it hides from the
// rules what it runs: an encoded command, a parameter not known, a command read from
// its input, or a line that builds, calls or renames what it runs.
func psLine(words []string) (inner string, hidden bool) {
	for i := 0; i < len(words); i++ {
		w := unquoted(words[i])
		if !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "/") {
			inner = strings.Join(words[i:], " ")
			break
		}
		name, _, valued := strings.Cut(strings.ToLower(strings.TrimLeft(w, "-/")), ":")
		switch {
		case name == "":
			return "", true
		case strings.HasPrefix("command", name), name == "cwa", len(name) > 7 && strings.HasPrefix("commandwithargs", name):
			inner = strings.Join(words[i+1:], " ")
		case name == "f" || (len(name) >= 2 && strings.HasPrefix("file", name)):
			// A script runs its own lines, as `bash script.sh` does; "-" is its input.
			return "", i+1 < len(words) && unquoted(words[i+1]) == "-"
		case psParam(name, psValue):
			if !valued {
				i++
			}
			continue
		case psParam(name, psNoValue):
			continue
		default:
			// -EncodedCommand, -e, -ec and anything else not known.
			return "", true
		}
		break
	}
	if inner == "" {
		// Without a command, or with -Command and nothing after it, PowerShell runs what
		// it reads from its input; or the line was cut at an operator inside its quotes.
		return "", true
	}
	if strings.ContainsAny(inner, "$`(){}&@+[]%") || strings.TrimSpace(unquoted(inner)) == "-" {
		return inner, true
	}
	for _, f := range strings.Fields(inner) {
		if slices.Contains(psHiders, strings.ToLower(strings.Trim(unquoted(f), ";"))) {
			return inner, true
		}
	}
	return inner, false
}

// substitution finds what $(...), backticks and process substitution, <(...) and
// >(...), run.
var substitution = regexp.MustCompile("[$<>]\\(([^()]*)\\)|`([^`]*)`")

// unquoted is how the shell reads a word's plain spelling: 'rm', "rm" and r\m are rm.
func unquoted(word string) string {
	return strings.NewReplacer("'", "", `"`, "", `\`, "").Replace(word)
}

func unquotedWords(words []string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = unquoted(w)
	}
	return out
}

// simpleCommands returns the commands a line runs, unwrapped for matching
// deny rules: each segment between operators, without enclosing
// parentheses, braces or quotes, leading assignments, wrapper commands, a
// leading backslash, a zsh =name or a path on the program, every word read as the
// shell reads it, with its quotes and escapes removed; sh -c and eval strings are
// unwrapped recursively.
func simpleCommands(cmd string, depth int) []string {
	out, _ := commandsOf(cmd, depth)
	return out
}

// commandsOf is simpleCommands and what it could not see: the programs whose
// subcommand is built or renamed when the line runs ("git $sub", "git -c alias.p=push
// p"), a program with the glob that may spell its subcommand ("git pu*h"), and "" for a
// line whose program is built or read from a shell's input. A deny rule cannot be
// compared with those, so nothing that only a rule allows may go ahead while one could
// apply (SEC-003).
func commandsOf(cmd string, depth int) (out, unseen []string) {
	cmd = joinLines(cmd)
	// What runs a line of its own (a substitution, sh -c, eval, cmd /c, -Command) is
	// followed maxDepth deep; deeper, the line is one the rules cannot see, which only
	// the user may run: four nested shells got past a deny rule (fuzzing, 2026-11-20).
	follow := func(line string) {
		if depth >= maxDepth {
			unseen = append(unseen, "")
			return
		}
		inner, in := commandsOf(line, depth+1)
		out, unseen = append(out, inner...), append(unseen, in...)
	}
	for _, m := range substitution.FindAllStringSubmatch(cmd, -1) {
		follow(m[1] + m[2])
	}
	for _, seg := range segments(cmd) {
		seg = strings.Trim(seg, "(){} \t'\"")
		fields := withoutRedirections(strings.Fields(seg))
		// env -S splits a string into the command it runs, and a GIT_CONFIG_ variable
		// before git may give it an alias.
		envS, gitConfig := false, false
		wrapped := 0
		for len(fields) > 0 {
			// A subshell or a group after a keyword or a launcher: ! (git push),
			// time { git push; }, if (git push); then (found by fuzzing on 2026-11-20).
			if g := strings.TrimLeft(fields[0], "({"); g != fields[0] {
				if fields[0] = g; g == "" {
					fields = fields[1:]
				}
				continue
			}
			f := unquoted(fields[0])
			name := ""
			for _, n := range programNames(fields[0]) {
				if _, ok := wrappers[n]; ok {
					name = n
					break
				}
			}
			valued, wrapper := wrappers[name]
			switch {
			case f == "coproc":
				// bash's and zsh's coproc runs the command after it, or the group after the
				// name it gives it: coproc p { git push; }.
				fields = fields[1:]
				if len(fields) > 1 && strings.ContainsAny(fields[1][:1], "({") {
					fields = fields[1:]
				}
			case slices.Contains(keywords, f):
				fields = fields[1:]
			case f == "function" && len(fields) > 1:
				fields = fields[2:] // function f { git push; }: the body is read as commands
			case strings.Index(fields[0], "()") > 0:
				// f() { git push; } and f(){ … }: so is this one's.
				if fields[0] = fields[0][strings.Index(fields[0], "()")+2:]; fields[0] == "" {
					fields = fields[1:]
				}
			case strings.Contains(f, "=") && !strings.HasPrefix(f, "="):
				gitConfig = gitConfig || strings.HasPrefix(f, "GIT_CONFIG")
				fields = fields[1:] // VAR=value
			case wrapper:
				// A rule can name the wrapper itself, which runs what follows it: deny
				// sudo * stops sudo, whatever it runs. Past maxWrappers in one command
				// the line is one the rules no longer read whole, as past maxDepth.
				if wrapped++; wrapped <= maxWrappers {
					out = append(out, strings.Join(append([]string{name}, unquotedWords(fields[1:])...), " "))
				} else if wrapped == maxWrappers+1 {
					unseen = append(unseen, "")
				}
				fields = fields[1:]
				// Wrapper options such as env -i, and those with a value, such as
				// env -u NAME, sudo -u root and env -iu NAME, where the value is the rest
				// of the word or else the next word. repeat has none: its count is next.
				for len(fields) > 0 && strings.HasPrefix(fields[0], "-") && name != "repeat" {
					opt := fields[0]
					if s, ok := envSplit(opt); ok && name == "env" {
						if envS = true; s != "" {
							fields[0] = s
						} else {
							fields = fields[1:]
						}
						break
					}
					fields = fields[1:]
					if i := strings.IndexAny(opt[1:], valued); i >= 0 && i == len(opt)-2 && !strings.HasPrefix(opt, "--") && len(fields) > 0 {
						fields = fields[1:]
					}
				}
				// timeout's first word is the time, and repeat's the count.
				if (name == "timeout" || name == "repeat") && len(fields) > 0 {
					fields = fields[1:]
				}
				// Some of rtk's own commands run what follows them.
				if name == "rtk" {
					for len(fields) > 0 && (slices.Contains(rtkRunners, fields[0]) || strings.HasPrefix(fields[0], "-")) {
						fields = fields[1:]
					}
				}
			default:
				goto done
			}
		}
	done:
		if len(fields) == 0 {
			continue
		}
		raw := slices.Clone(fields)
		// What the words say once the shell has read them: 'push', "push" and pu""sh are
		// push, and zsh's =git is the program git.
		for i := range fields {
			fields[i] = unquoted(fields[i])
		}
		// The program by the name every spelling of it shares (programName).
		names := programNames(raw[0])
		fields[0] = names[0]
		// Where the subcommand is, after git's own options, and whether they, or a
		// GIT_CONFIG_ variable, may rename it.
		sub, renamed := 1, false
		if fields[0] == "git" {
			sub, renamed = gitSubcommand(raw, fields)
			renamed = renamed || gitConfig
		}
		switch {
		case strings.ContainsAny(raw[0], "$`{}") || (strings.ContainsAny(raw[0], "*?[") && raw[0] != "[" && raw[0] != "[["):
			// A glob or a brace in the program word is expanded to the program that runs.
			unseen = append(unseen, "")
		case fields[0] == "alias" && slices.ContainsFunc(fields[1:], func(w string) bool { return strings.Contains(w, "=") }):
			// An alias defined on the line may rename what runs after it.
			unseen = append(unseen, "")
		case envS && slices.ContainsFunc(raw, func(w string) bool { return strings.Contains(w, `\`) }):
			// env -S reads escapes of its own: env -S 'git\_push' runs git push.
			unseen = append(unseen, "")
		case renamed:
			unseen = append(unseen, fields[0])
		case sub < len(raw) && strings.ContainsAny(raw[sub], "$`"):
			unseen = append(unseen, fields[0])
		case sub < len(raw) && strings.ContainsAny(raw[sub], "*?[{"):
			// A glob or a brace may spell the subcommand: git pu*h runs git push. Which
			// words it may spell is for hidesFromDenyRules to say (mayExpand).
			unseen = append(unseen, fields[0]+" "+fields[sub])
		}
		simple := strings.Join(fields, " ")
		out = append(out, simple)
		for _, alt := range names[1:] {
			out = append(out, strings.Join(append([]string{alt}, fields[1:]...), " "))
		}
		if g := withoutGitOptions(fields); g != "" {
			out = append(out, g)
		}
		if slices.Contains(shells, fields[0]) {
			line, fromInput := shellLine(fields[0], fields[1:])
			if fromInput {
				unseen = append(unseen, "")
			}
			if line != "" {
				follow(strings.Trim(line, "'\""))
			}
		}
		// find runs the command after -exec and the like, up to ; or +, and with {} as
		// the program, each file it finds, which no rule sees.
		if fields[0] == "find" {
			for i := 1; i < len(fields); i++ {
				if !slices.Contains(findRunners, fields[i]) {
					continue
				}
				end := i + 1
				for end < len(fields) && fields[end] != ";" && fields[end] != "+" {
					end++
				}
				if end > i+1 && strings.Contains(fields[i+1], "{}") {
					unseen = append(unseen, "")
				}
				follow(strings.Join(raw[i+1:end], " "))
				i = end
			}
		}
		// eval runs its words as a line.
		if fields[0] == "eval" && len(fields) > 1 {
			follow(strings.Join(raw[1:], " "))
			follow(strings.Join(fields[1:], " "))
		}
		// Windows' interpreters run their line too: cmd /c and PowerShell's -Command
		// are read as sh -c is, and what they hide from the rules is unseen.
		var inner string
		var hidden bool
		switch {
		case slices.Contains(cmdShells, fields[0]):
			inner, hidden = cmdLine(raw[1:])
		case slices.Contains(psShells, fields[0]):
			inner, hidden = psLine(raw[1:])
		}
		// segments splits at an operator inside quotes, which sh would not, but cmd and
		// PowerShell then would: their line is not what the segment shows.
		if hidden || (inner != "" && quotedOperator(cmd)) {
			unseen = append(unseen, "")
		}
		if inner != "" {
			follow(inner)
		}
	}
	return out, unseen
}

// shellDenyRule reports a deny rule that compares shell commands.
func shellDenyRule(r Rule) bool {
	ok, _ := path.Match(r.Tool, "shell")
	return ok && r.Effect == Deny && r.Command != ""
}

// hidesFromDenyRules reports a line that may run what a shell deny rule names without
// showing it: a program built when it runs or read from a shell's input ("$a push",
// "echo git push | sh"), or the subcommand of a program a rule names, built, renamed or
// spelled by a glob that may be the rule's word ("git $sub", "git -c alias.p=push p",
// "git pu*h"). No line at all hides everything: an execute request whose client could
// not read its command, such as an external agent's in a shape it does not know.
func hidesFromDenyRules(rules []Rule, cmd string) bool {
	var patterns [][]string
	for _, r := range rules {
		if f := strings.Fields(denyPattern(r.Command)); shellDenyRule(r) && len(f) > 0 {
			patterns = append(patterns, f)
		}
	}
	if len(patterns) == 0 {
		return false
	}
	if strings.TrimSpace(cmd) == "" {
		return true
	}
	_, unseen := commandsOf(cmd, 0)
	return slices.ContainsFunc(unseen, func(u string) bool {
		program, glob, globbed := strings.Cut(u, " ")
		return u == "" || slices.ContainsFunc(patterns, func(f []string) bool {
			return f[0] == program && (!globbed || len(f) < 2 || mayExpand(glob, f[1]))
		})
	})
}

// flagLetters are what a cluster of flags is written with.
const flagLetters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// mayExpand reports whether a word with a glob or a brace in it may become word, a deny
// rule's word after its program, once the shell expands it: pu*h may be push, and * or
// -?f may be -rf, a cluster of flags read as a set (flagSetMatches), while *.o may be
// neither. A brace may be anything.
func mayExpand(glob, word string) bool {
	if word == "*" || strings.Contains(glob, "{") {
		return true
	}
	if ok, err := path.Match(glob, word); ok || err != nil {
		return true
	}
	return isCluster(word) && strings.ContainsAny(glob[:1], "-*?[") && strings.Trim(glob, flagLetters+"-*?[]!^") == ""
}

// gitValueOptions are git's global options that take a separate value.
var gitValueOptions = []string{"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env", "--super-prefix", "--attr-source", "--exec-path"}

// gitSubcommand returns where a git command's subcommand is, after git's own options, and
// whether those may rename it: an alias or an included file set with -c, or a -c value
// built when the line runs, or --config-env, which sets anything from a variable.
func gitSubcommand(raw, fields []string) (int, bool) {
	renamed := false
	i := 1
	for i < len(fields) && strings.HasPrefix(fields[i], "-") {
		opt, _, attached := strings.Cut(fields[i], "=")
		renamed = renamed || opt == "--config-env"
		if slices.Contains(gitValueOptions, opt) && opt != "--exec-path" && !attached && i+1 < len(fields) {
			i++
			key := strings.ToLower(fields[i])
			renamed = renamed || (opt == "-c" && (strings.ContainsAny(raw[i], "$`") || strings.HasPrefix(key, "alias.") || strings.HasPrefix(key, "include")))
		}
		i++
	}
	return i, renamed
}

// withoutGitOptions returns a git command without the global options before
// its subcommand, so a deny rule for "git push *" also matches
// "git -C . push" and "git -c a=b --no-pager push"; "" when fields is not a
// git command with such options.
func withoutGitOptions(fields []string) string {
	if fields[0] != "git" || len(fields) < 2 || !strings.HasPrefix(fields[1], "-") {
		return ""
	}
	i, _ := gitSubcommand(fields, fields)
	return strings.Join(append([]string{"git"}, fields[i:]...), " ")
}

// unquotedLine is a line with its lines joined and every quote and backslash taken out.
// A shell's line quoted inside another's, its quotes closed and reopened around an
// escaped one at each level, is not undone by trimming quotes, and its operators and
// words stay hidden in it; with the quotes gone they are in the clear. A deny rule is
// compared with it as well as with the line, which can only make the rules see more, as
// splitting at a quoted operator already does (found by fuzzing on 2026-11-20).
func unquotedLine(cmd string) string {
	return strings.NewReplacer("'", "", `"`, "", `\`, "").Replace(joinLines(cmd))
}

// maxDepth is how deep commandsOf follows lines that run lines of their own.
const maxDepth = 3

// maxWrappers is how many wrappers in one command commandsOf gives a line of their own,
// for a rule that names one; a long chain of them would otherwise cost the square of
// its length.
const maxWrappers = 16

// joinLines removes each backslash before a line break, as a shell does before it
// reads the line: `git \⏎ push origin` runs git push, and a deny rule compared with its
// two lines apart saw neither (SEC-003; found by fuzzing on 2026-11-20). A backslash
// that is itself escaped is left, with its line break: there the line does end. Inside
// single quotes a shell keeps the pair, but a quoted line that reaches a shell through
// -c is read by that shell the same way, and joining it anywhere else only makes the
// rules see more.
func joinLines(cmd string) string {
	if !strings.Contains(cmd, "\\\n") && !strings.Contains(cmd, "\\\r\n") {
		return cmd
	}
	var b strings.Builder
	for i := 0; i < len(cmd); i++ {
		if cmd[i] != '\\' || i+1 == len(cmd) {
			b.WriteByte(cmd[i])
			continue
		}
		switch {
		case cmd[i+1] == '\n':
			i++
		case cmd[i+1] == '\r' && i+2 < len(cmd) && cmd[i+2] == '\n':
			i += 2
		default:
			b.WriteByte(cmd[i])
			b.WriteByte(cmd[i+1])
			i++
		}
	}
	return b.String()
}

// segments splits a command line at shell operators. An & right after < or > is part of
// a redirection (2>&1), not an operator, and splitting there would cut off the command
// after it; &> is split, which in sh it is.
func segments(cmd string) []string {
	parts := []string{cmd}
	for _, op := range shellOperators {
		var next []string
		for _, p := range parts {
			if op == "&" {
				next = append(next, splitAmpersand(p)...)
				continue
			}
			next = append(next, strings.Split(p, op)...)
		}
		parts = next
	}
	var out []string
	for _, p := range parts {
		if n := normalize(p); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// splitAmpersand splits p at each & that does not follow < or >.
func splitAmpersand(p string) []string {
	var out []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '&' && (i == 0 || (p[i-1] != '<' && p[i-1] != '>')) {
			out = append(out, p[start:i])
			start = i + 1
		}
	}
	return append(out, p[start:])
}

func commandMatches(pattern, cmd string) bool {
	pattern = normalize(pattern)
	if prefix, ok := strings.CutSuffix(pattern, " *"); ok {
		return cmd == prefix || strings.HasPrefix(cmd, prefix+" ")
	}
	if pattern == "*" {
		return true
	}
	return cmd == pattern
}

// rmLong are the long names of rm's options that a rule on rm's short ones must see.
var rmLong = map[string]string{"--recursive": "-r", "--force": "-f"}

// flagForm reads the flag words after the program as sets of letters: -rf, -fr, -r -f
// and rm's --recursive --force are one word, "-fr". A word of the form -x… with
// single-letter flags is merged with the cluster before it, and its letters sorted.
func flagForm(words []string) []string {
	var out []string
	var set string
	flush := func() {
		if set != "" {
			l := strings.Split(set, "")
			slices.Sort(l)
			out = append(out, "-"+strings.Join(slices.Compact(l), ""))
			set = ""
		}
	}
	for i, w := range words {
		if i > 0 && words[0] == "rm" {
			if short, ok := rmLong[w]; ok {
				w = short
			}
		}
		if i > 0 && len(w) > 1 && w[0] == '-' && w[1] != '-' && strings.Trim(w[1:], flagLetters) == "" {
			if words[0] == "rm" {
				w = strings.ReplaceAll(w, "R", "r") // rm's -R is its -r
			}
			set += w[1:]
			continue
		}
		flush()
		out = append(out, w)
	}
	flush()
	return out
}

// flagSetMatches is commandMatches with flag letters read as sets (flagForm), where the
// command may carry more letters than the rule names: `rm -rf *` stops rm -rfv x.
func flagSetMatches(pattern, cmd string) bool {
	prefix, star := strings.CutSuffix(normalize(pattern), " *")
	pw, cw := flagForm(strings.Fields(prefix)), flagForm(strings.Fields(cmd))
	if len(cw) < len(pw) || (!star && len(cw) != len(pw)) {
		return false
	}
	for i, w := range pw {
		if isCluster(w) && isCluster(cw[i]) && i > 0 {
			if strings.Trim(w[1:], cw[i][1:]) != "" {
				return false
			}
			continue
		}
		if cw[i] != w {
			return false
		}
	}
	return true
}

func isCluster(w string) bool { return len(w) > 1 && w[0] == '-' && w[1] != '-' }

// matchCommand: a deny rule matches if any command the line runs matches.
// An exact or literal allow rule matches exactly the line it names, as
// written, operators included, since that is what the user approved; a
// prefix allow rule matches only a single plain command.
func (r Rule) matchCommand(cmd string) bool {
	if r.Effect == Deny {
		if r.Literal {
			return exactForm(cmd) == exactForm(r.Command)
		}
		pattern := denyPattern(r.Command)
		commands := simpleCommands(cmd, 0)
		if u := unquotedLine(cmd); u != cmd {
			commands = append(commands, simpleCommands(u, 0)...)
		}
		return slices.ContainsFunc(commands, func(s string) bool {
			return commandMatches(pattern, s) || flagSetMatches(pattern, s)
		})
	}
	pattern := normalize(r.Command)
	if r.Literal || (pattern != "*" && !strings.HasSuffix(pattern, " *")) {
		return exactForm(cmd) == exactForm(r.Command)
	}
	segs := segments(cmd)
	if len(segs) != 1 || !plain(cmd) {
		return false
	}
	return commandMatches(r.Command, segs[0])
}

func (r Rule) matchPath(root, p string) bool {
	// Both spelled without macOS's data volume name, which is another spelling of the
	// same place (SEC-005).
	p = environment.Unaliased(filepath.Clean(p))
	pattern := environment.Unaliased(TrimDotSlash(rulePath(goruntime.GOOS, r.Path, r.Literal)))
	if r.Literal {
		if filepath.IsAbs(pattern) {
			return environment.FoldPath(filepath.Clean(pattern)) == environment.FoldPath(p)
		}
		for _, rt := range canonical(root) {
			if environment.FoldPath(filepath.Join(rt, filepath.FromSlash(pattern))) == environment.FoldPath(p) {
				return true
			}
		}
		return false
	}
	// Globs ignore case, as macOS volumes do, so SECRETS/key is secrets/key.
	if filepath.IsAbs(pattern) {
		name := environment.FoldPath(strings.TrimPrefix(filepath.ToSlash(p), "/"))
		for _, pat := range absSpellings(pattern) {
			if environment.MatchGlob(environment.FoldPath(strings.TrimPrefix(filepath.ToSlash(pat), "/")), name) {
				return true
			}
		}
		return false
	}
	for _, rt := range canonical(root) {
		if within(p, rt) {
			rel := strings.TrimPrefix(environment.FoldPath(p)[len(environment.FoldPath(rt)):], string(filepath.Separator))
			return environment.MatchGlob(environment.FoldPath(pattern), filepath.ToSlash(rel))
		}
	}
	return false
}

// rulePath is a rule's path as matching reads it on goos. On Windows a backslash
// is a separator there, as a slash is, so src\** is src/** and matches src\a.go
// (NFR-XPLAT-003); elsewhere a backslash in a glob escapes the next character. A
// literal path is joined to the root as it is, which on Windows reads either.
func rulePath(goos, pattern string, literal bool) string {
	if goos == "windows" && !literal {
		return strings.ReplaceAll(pattern, `\`, "/")
	}
	return pattern
}

// TrimDotSlash drops a leading ./ from a rule's path, as Claude Code's documentation
// writes Read(./.env): compared segment by segment, the . matches no file, and a deny
// rule written so would protect nothing. A path that is only ./ stays as it is, so it
// never turns into a rule for every path.
func TrimDotSlash(p string) string {
	for {
		rest, ok := strings.CutPrefix(p, "./")
		rest = strings.TrimLeft(rest, "/")
		if !ok || rest == "" {
			return p
		}
		p = rest
	}
}

// absSpellings returns an absolute pattern under each spelling of its
// folders without globs, so /tmp/** also matches the resolved
// /private/tmp/... paths reach the broker as.
func absSpellings(pattern string) []string {
	parts := strings.Split(filepath.ToSlash(pattern), "/")
	i := slices.IndexFunc(parts, func(s string) bool { return strings.ContainsAny(s, "*?[") })
	if i < 0 {
		i = len(parts)
	}
	prefix, rest := strings.Join(parts[:i], "/"), strings.Join(parts[i:], "/")
	if prefix == "" {
		return []string{pattern}
	}
	var out []string
	for _, sp := range canonical(filepath.FromSlash(prefix)) {
		out = append(out, path.Join(filepath.ToSlash(sp), rest))
	}
	return append(out, pattern)
}

// writeTools are the two ways to write a file, which the model chooses between and an
// external agent's writes are judged as the second of.
var writeTools = map[string]string{"write_file": "edit_file", "edit_file": "write_file"}

// toolMatches reports whether r's tool names tool. A deny rule on a path that names one
// way to write a file holds for the other too: the rule is about the path, and which tool
// the model picks must not decide it. An allow rule stays as narrow as it was written.
func (r Rule) toolMatches(tool string) bool {
	if ok, _ := path.Match(r.Tool, tool); ok {
		return true
	}
	if other, both := writeTools[tool]; both && r.Effect == Deny && r.Path != "" {
		ok, _ := path.Match(r.Tool, other)
		return ok
	}
	return false
}

// Matches reports whether r applies to req in a session rooted at root. An
// allow rule with a path must cover every path the call touches; a deny rule
// with a path matches if any path does.
func (r Rule) Matches(req Request, root string) bool {
	if !r.toolMatches(req.Tool) {
		return false
	}
	switch {
	case r.Command != "":
		return req.Command != "" && r.matchCommand(req.Command)
	case r.Path != "":
		if len(req.Paths) == 0 {
			return false
		}
		match := func(p string) bool { return r.matchPath(root, p) }
		if r.Effect == Deny {
			return slices.ContainsFunc(req.Paths, match)
		}
		for _, p := range req.Paths {
			if !match(p) {
				return false
			}
		}
		return true
	}
	return true
}

// DeniedPathPatterns turns path deny rules into regular expressions over
// absolute paths, for a sandbox: without an approval to stop a command, a deny
// rule has to hold for the shell as well (ENV-009). read lists what no command
// may touch, and write what it may read but not change.
func DeniedPathPatterns(rules []Rule, root string) (read, write []string) {
	covers := func(r Rule, tools ...string) bool {
		return slices.ContainsFunc(tools, func(t string) bool {
			ok, _ := path.Match(r.Tool, t)
			return ok
		})
	}
	for _, r := range rules {
		if r.Effect != Deny || r.Path == "" {
			continue
		}
		re := pathRegex(TrimDotSlash(filepath.ToSlash(r.Path)), root, r.Literal)
		switch {
		case covers(r, "read_file", "grep", "glob", "list_dir"):
			read = append(read, re)
		case covers(r, "write_file", "edit_file"):
			write = append(write, re)
		}
	}
	return read, write
}

// pathRegex writes a rule's path as an anchored regular expression that ignores
// case, as path rules do, and also covers what is under the path.
func pathRegex(pattern, root string, literal bool) string {
	var b strings.Builder
	b.WriteString("^")
	if !strings.HasPrefix(pattern, "/") {
		b.WriteString(regexp.QuoteMeta(root) + "/")
	}
	segments := strings.Split(pattern, "/")
	for i, seg := range segments {
		if !literal && seg == "**" {
			if i == len(segments)-1 {
				b.WriteString(".*")
			} else {
				b.WriteString("(.*/)?")
			}
			continue
		}
		b.WriteString(segmentRegex(seg, literal))
		if i < len(segments)-1 {
			b.WriteString("/")
		}
	}
	b.WriteString("(/.*)?$")
	return b.String()
}

func segmentRegex(seg string, literal bool) string {
	var b strings.Builder
	runes := []rune(seg)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case !literal && c == '*':
			b.WriteString("[^/]*")
		case !literal && c == '?':
			b.WriteString("[^/]")
		case !literal && c == '[':
			// path.Match's class, copied; one left open matches itself.
			end := slices.Index(runes[i+1:], ']')
			if end <= 0 {
				b.WriteString(`\[`)
				continue
			}
			b.WriteString(strings.ReplaceAll(string(runes[i:i+end+2]), `"`, ""))
			i += end + 1
		case c == '"':
			b.WriteString(".")
		case unicode.IsLetter(c) && unicode.ToLower(c) != unicode.ToUpper(c):
			b.WriteString("[" + string(unicode.ToLower(c)) + string(unicode.ToUpper(c)) + "]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// RuleFor is the rule "Allow always" creates for req: the literal command,
// or the literal path (relative to root when inside it), or the tool alone
// for a tool that names neither. It reports false for a request one rule
// cannot narrow, several paths or a command it does not show: a rule for
// the whole tool would allow far more than the user saw.
func RuleFor(req Request, root string, scope Scope) (Rule, bool) {
	r := Rule{Effect: Allow, Tool: req.Tool, Scope: scope}
	switch {
	case req.Command != "":
		r.Command, r.Literal = exactForm(req.Command), true
	case len(req.Paths) == 1:
		r.Path, r.Literal = req.Paths[0], true
		for _, rt := range canonical(root) {
			if rel, err := filepath.Rel(rt, req.Paths[0]); err == nil && !strings.HasPrefix(rel, "..") {
				r.Path = filepath.ToSlash(rel)
				break
			}
		}
	case len(req.Paths) > 1, req.Kind == KindExecute:
		return r, false
	case strings.HasPrefix(req.Tool, "acp:"), slices.Contains([]string{"edit_file", "read_file", "grep"}, req.Tool):
		// An external agent's tool is known only by its kind: "always" for reading
		// one calendar event would also have allowed deleting every calendar.
		return r, false
	}
	return r, true
}

// RuleSource supplies the rules in force for a session, every scope
// together. An error means some rules could not be read; the broker then
// fails closed.
type RuleSource interface {
	Rules(sessionID string) ([]Rule, error)
}

// RuleSink stores a rule created by "Allow always".
type RuleSink interface {
	AddRule(sessionID string, r Rule) error
}
