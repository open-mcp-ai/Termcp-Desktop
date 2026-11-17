package capability

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/approval"
)

// TestTableIsWellFormed pins the invariants every row must satisfy. A row is the
// only statement of an operation's admission policy, so a malformed one is not a
// typo — it is a gate that silently does something other than what it reads.
func TestTableIsWellFormed(t *testing.T) {
	for _, c := range All() {
		if c.ID == "" {
			t.Errorf("a row has no id: %+v", c)
			continue
		}
		if strings.TrimSpace(c.ID) != c.ID || strings.ContainsAny(c.ID, " \t") {
			t.Errorf("%q: an id is a bare token, no spaces", c.ID)
		}
		if c.Staged {
			if c.Kind == "" {
				t.Errorf("%s: staged means a reviewable unit split across calls, so it needs a Kind", c.ID)
			}
			if c.Gate != GateReview {
				t.Errorf("%s: staged without GateReview would hold input nobody reviews", c.ID)
			}
		}
		if c.Kind != "" && !approval.ValidKind(c.Kind) {
			t.Errorf("%s: kind %q is not a valid approval kind", c.ID, c.Kind)
		}
		// A path argument nothing else declares means a typo the whitelist would
		// read as "no path here", which is a hole rather than an error.
		for _, a := range c.PathArgs {
			if strings.TrimSpace(a) != a || a == "" {
				t.Errorf("%s: path argument %q is malformed", c.ID, a)
			}
		}
	}
}

// TestEveryToolHasACapability keeps the two vocabularies in step. A tool with no
// row is unreachable from a restricted session, so the failure is invisible until
// an extension tries it.
func TestEveryToolHasACapability(t *testing.T) {
	for tool := range toolIDs {
		if _, ok := ForTool(tool); !ok {
			t.Errorf("tool %q maps to an id with no row", tool)
		}
	}
	// The reverse direction: every row that a tool can reach is reachable.
	for id, tool := range toolForID {
		if _, ok := Lookup(id); !ok {
			t.Errorf("tool %q maps onto unknown capability %q", tool, id)
		}
	}
}

// TestDeviceAssignedIsAuthorisedAndInstallingIsReviewed is the security boundary
// the whole design turns on, stated as one assertion:
//
//	using a device the agent was given is not reviewed (curl | sh included),
//	growing the list of things it can reach is.
//
// The two halves must behave OPPOSITELY under review mode. If they ever agree,
// either review asks a human to approve a command it cannot judge, or an agent
// installs a resident extension without anyone looking.
func TestDeviceAssignedIsAuthorisedAndInstallingIsReviewed(t *testing.T) {
	// Terminal input is the operator's own keyboard on the Web UI and a
	// GateReview/Staged capability on the MCP surface: the SAME capability, two
	// gates, decided by who is typing. That split is the reason the gate is not
	// simply "the capability's kind" — see TestTypingIsReviewedForAgentsOnly.
	use := []string{"shell.open", "session.start", "session.terminate", "session.delete", "ext.delete", "profile.delete"}
	for _, id := range use {
		c, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if c.Gate == GateReview {
			t.Errorf("%s is using an assigned device and must not be reviewed: what a command installs cannot be judged by a rule", id)
		}
	}

	install := []string{
		"ext.create", "ext.update",
		"profile.create", "profile.edit",
		"file.write", "file.delete", "forward.open", "forward.close",
	}
	for _, id := range install {
		c, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if c.Gate != GateReview {
			t.Errorf("%s changes what the agent can reach and must be reviewed", id)
		}
		if c.Kind == "" {
			t.Errorf("%s is reviewed but has no approval kind, so it cannot actually be queued", id)
		}
	}
}

// TestReadsAreNeverReviewed is the rule that keeps a gate being read: asking a
// human to approve a listing trains them to click Accept without looking.
func TestReadsAreNeverReviewed(t *testing.T) {
	for _, id := range []string{
		"file.read", "file.stat", "file.urls", "file.getwd",
		"session.list", "session.info", "session.message",
		"shell.output", "shell.list", "shell.detect",
		"forward.list", "profile.list", "ext.list", "ext.get",
		"user.notify",
	} {
		c, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if c.Kind != "" {
			t.Errorf("%s is a read but carries approval kind %q", id, c.Kind)
		}
		if c.Gate == GateReview {
			t.Errorf("%s is a read and must not be reviewed", id)
		}
		if !c.Idempotent {
			t.Errorf("%s reads and changes nothing, so repeating it is safe", id)
		}
	}
}

// TestIdempotenceMatchesTheOperation pins the Idempotent header's meaning: it
// answers "may I retry this after a failure". Claiming no for a read makes an
// agent give up on a retryable call; claiming yes for a write makes it run twice.
func TestIdempotenceMatchesTheOperation(t *testing.T) {
	mustNot := []string{
		"shell.input", "shell.key", "shell.close", "shell.open",
		"session.start", "session.terminate", "session.delete",
		"file.write", "file.delete", "file.rename", "file.mkdir",
		"file.perm", "file.link", "file.fs",
		"forward.open", "forward.close",
	}
	for _, id := range mustNot {
		c, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if c.Idempotent {
			t.Errorf("%s must not be marked idempotent: a retry runs it again", id)
		}
	}
}

// TestPathArgumentsAreDeclaredPerCapability pins the move away from one global
// list of path argument names: a capability names the arguments IT carries, so a
// path the whitelist should scope cannot be missed by a capability that never
// mentioned it, and a capability cannot be scoped by arguments it does not have.
func TestPathArgumentsAreDeclaredPerCapability(t *testing.T) {
	want := map[string][]string{
		"file.write":  {"remote_path", "local_path"},
		"file.rename": {"from_path", "to_path"},
		"file.link":   {"remote_path", "target", "link_path", "existing_path", "new_path"},
	}
	for id, args := range want {
		c, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if len(c.PathArgs) != len(args) {
			t.Errorf("%s declares paths %v, want %v", id, c.PathArgs, args)
			continue
		}
		for i := range args {
			if c.PathArgs[i] != args[i] {
				t.Errorf("%s declares paths %v, want %v", id, c.PathArgs, args)
				break
			}
		}
	}
	// A capability that touches no path must declare none, or the guard would
	// scope an operation on an argument it does not take.
	for _, id := range []string{"session.list", "shell.input", "user.notify", "ext.create"} {
		c, _ := Lookup(id)
		if len(c.PathArgs) != 0 {
			t.Errorf("%s takes no path but declares %v", id, c.PathArgs)
		}
	}
}

// TestSurfaceSourceHoldsNoOperationNames is the refactor's own acceptance test.
//
// The point of the capability layer is that ONE table decides admission. The bug
// this replaced was a decision written twice: gatedTools named the operations in
// one package, twelve REST call sites spelled them again, and the second copy is
// the one that drifts. So the test asks the narrow, meaningful question — does
// admission code COMPARE a tool name against a literal? — rather than banning the
// words everywhere, because those words are also argument keys ("ssh_config") and
// UI vocabulary, and a scan that cannot tell them apart only teaches people to
// rename things.
//
// What is caught:
//   - `x == "shell_input"` / `x != "file_write"` where x is a tool name variable;
//   - `case "file_write":` inside a switch on a tool name variable.
//
// What is not: comments, argument keys, handler registries, and the summary
// translator, which exists precisely to map a tool name onto a sentence.
func TestSurfaceSourceHoldsNoOperationNames(t *testing.T) {
	forbidden := make(map[string]bool, len(toolIDs))
	for tool := range toolIDs {
		forbidden[tool] = true
	}

	toolVars := map[string]bool{"tool": true, "toolName": true}

	roots := []string{
		filepath.Join("..", "mcp"),
		filepath.Join("..", "webui"),
	}
	fsc := token.NewFileSet()
	found := 0
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			// A file that DECLARES the tool surface may name its tools; a file that
			// DECIDES admission may not. The split is short and named so a fourth
			// exemption has to be argued for.
			if declaresToolSurface(name) {
				continue
			}
			path := filepath.Join(root, name)
			f, err := parser.ParseFile(fsc, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			found++
			for _, lit := range admissionLiterals(f, toolVars) {
				if forbidden[lit.value] {
					pos := fsc.Position(lit.pos)
					t.Errorf("%s: admission compares a tool name against the literal %q; the decision belongs in the capability table (internal/capability)", pos, lit.value)
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no source files were checked; the scan is not looking where it thinks")
	}
}

// declaresToolSurface reports whether a file is a declaration of the tool
// surface rather than a consumer of it.
func declaresToolSurface(name string) bool {
	switch name {
	case "tools.go", // the tool registration table
		"call.go",      // tool name -> handler registry
		"toolopts.go",  // compact descriptions, indexed by tool name
		"summarize.go": // tool name -> the sentence a reviewer reads
		return true
	}
	return false
}

// namedLiteral is a string literal with the position needed to report it.
type namedLiteral struct {
	value string
	pos   token.Pos
}

// admissionLiterals finds string literals used as the other side of a comparison
// against a tool name variable, and case labels of a switch on one.
func admissionLiterals(f *ast.File, toolVars map[string]bool) []namedLiteral {
	var out []namedLiteral
	record := func(n ast.Node) {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return
		}
		out = append(out, namedLiteral{value: strings.Trim(lit.Value, "\"`"), pos: lit.Pos()})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if node.Op != token.EQL && node.Op != token.NEQ {
				return true
			}
			if ident, ok := node.X.(*ast.Ident); ok && toolVars[ident.Name] {
				record(node.Y)
			}
			if ident, ok := node.Y.(*ast.Ident); ok && toolVars[ident.Name] {
				record(node.X)
			}
		case *ast.SwitchStmt:
			ident, ok := node.Tag.(*ast.Ident)
			if !ok || !toolVars[ident.Name] {
				return true
			}
			for _, stmt := range node.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range clause.List {
					record(expr)
				}
			}
		}
		return true
	})
	return out
}

// allowedNameFile reports whether a file may legitimately spell a tool name.
//
// Admission decisions must not; declarations of the surface must. The split is
// deliberately short — three files, each named for what it declares — so a fourth
// exemption has to be argued for.
func allowedNameFile(name string) bool {
	switch name {
	case "tools.go", // the tool registration table
		"call.go",     // tool name -> handler registry
		"docs.go",     // the embedded documentation index
		"toolopts.go", // compact descriptions, indexed by tool name
		"origin.go":
		return true
	}
	return false
}

// stringLiterals returns every string literal in a file, basic or raw.
func stringLiterals(f *ast.File) []*ast.BasicLit {
	var out []*ast.BasicLit
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			out = append(out, lit)
		}
		return true
	})
	return out
}

// TestTypingIsReviewedForAgentsOnly pins the one capability whose gate depends on
// WHO acts rather than on what the operation is.
//
// A command line typed by an agent is exactly what review mode exists to catch;
// the same bytes typed by the human at the browser are their own keyboard, and
// asking them to approve it locks them out of the session they are watching. So
// the row says "reviewable and staged" (the queue needs to know how to hold it),
// while the operator surfaces never consult the gate at all — the WebSocket and
// the REST input route admit on the capability list and nothing else.
//
// The alternative, and the shape this replaced, was twelve call sites each
// remembering not to write the check. That is a decision living in the absence of
// code; here it is a row plus this test.
func TestTypingIsReviewedForAgentsOnly(t *testing.T) {
	for _, id := range []string{"shell.input", "shell.key"} {
		c, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if !c.Staged {
			t.Errorf("%s must be Staged: a command line is text plus the key that ends it", id)
		}
		if c.Kind != approval.KindShellInput {
			t.Errorf("%s kind = %q, want %q", id, c.Kind, approval.KindShellInput)
		}
	}
}
