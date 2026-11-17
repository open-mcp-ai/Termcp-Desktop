package webui

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/open-mcp-ai/termcp/pkg/api"
)

// Activity is one thing an agent (or a person) did to this instance, as seen from
// outside any single terminal.
//
// Every field is derived from marks that already exist in each shell's log
// (api.LogMark): a mark is written when input is submitted, when an agent's line
// goes through, and when a review decision releases it. Nothing new is recorded
// here — the audit trail was always there, but it could only be read one
// terminal at a time, which is why "what did the agent do at 14:02?" had no
// answer short of opening every window (issue #110).
type Activity struct {
	// Time is the mark's own Unix millisecond stamp, as RFC 3339 in UTC.
	Time string `json:"time"`
	// UnixMS is the same instant, for a client that sorts or buckets it.
	UnixMS int64 `json:"unix_ms"`
	// SessionID and ShellID name where it happened.
	SessionID string `json:"session_id"`
	ShellID   string `json:"shell_id"`
	// SessionName is the session's display name, so a reader does not have to
	// resolve the id to know which host it was.
	SessionName string `json:"session_name,omitempty"`
	// Source is "ai" for an agent, "human" for the API/Web UI, and "system" for a
	// review transition that did not produce input itself.
	Source string `json:"source"`
	// Kind is the raw mark status ("a", "i", "o", "q", "A"), kept so a reader
	// that speaks the log format needs no mapping. Output spans are excluded from
	// this feed (see below), so the useful values are the input and review ones.
	Kind string `json:"kind"`
	// Verdict is what happened, in words: "input", "review-requested",
	// "review-granted".
	Verdict string `json:"verdict"`
	// Bytes is where in that shell's log the span starts, so a client can fetch
	// the exact bytes with shell_output / the REST range route.
	Bytes int64 `json:"bytes"`
}

// activityFromMark classifies one mark, or reports false when the feed should not
// carry it.
//
// Output spans are excluded on purpose. The question this feed answers is "what
// was DONE here", and every command a shell runs emits output — including a
// `find` that prints ten thousand lines — so including them would bury the few
// rows a reader cares about under the machine's own noise. The first output span
// after an input is where its result lives, and that is what the byte offset is
// for: follow it into the terminal (or the REST range route) if you want it.
func activityFromMark(sessionID, sessionName, shellID string, m api.LogMark) (Activity, bool) {
	a := Activity{
		Time:        time.UnixMilli(m.Time).UTC().Format(time.RFC3339),
		UnixMS:      m.Time,
		SessionID:   sessionID,
		SessionName: sessionName,
		ShellID:     shellID,
		Kind:        string(m.Status),
		Bytes:       m.Offset,
	}
	switch m.Status {
	case api.LogAIInput:
		a.Source, a.Verdict = "ai", "input"
	case api.LogAPIInput:
		a.Source, a.Verdict = "human", "input"
	case api.LogApprovalRequest:
		a.Source, a.Verdict = "system", "review-requested"
	case api.LogApprovalGranted:
		a.Source, a.Verdict = "system", "review-granted"
	default:
		return Activity{}, false
	}
	return a, true
}

// handleActivity answers GET /api/activity — recent agent and human actions across
// every session, newest first.
//
// It reads each session's marks through the same accessor the terminal timeline
// uses, so there is one interpretation of the log and no second copy of it. The
// cost is bounded by `limit` and by only reading the shells of the sessions asked
// for; a deployment with hundreds of archived sessions can narrow it with
// `session_id`.
//
//	GET /api/activity?limit=200&session_id=<id>&since_ms=<unix ms>
func (h *Handler) handleActivity(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		// A cap rather than an unbounded read: the feed is a window on history,
		// and a caller that wants all of it can page by since_ms.
		if n > 2000 {
			n = 2000
		}
		limit = n
	}
	var sinceMS int64
	if raw := r.URL.Query().Get("since_ms"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			http.Error(w, "since_ms must be a non-negative integer", http.StatusBadRequest)
			return
		}
		sinceMS = n
	}
	onlySession := r.URL.Query().Get("session_id")

	if h.Sessions == nil {
		writeJSON(w, http.StatusOK, map[string]any{"activity": []Activity{}})
		return
	}

	sessions := h.Sessions.ListAll()
	var events []Activity
	for _, info := range sessions {
		if onlySession != "" && info.ID != onlySession {
			continue
		}
		sess := h.Sessions.Get(info.ID)
		if sess == nil {
			continue
		}
		for _, shell := range sess.ShellsForView() {
			marks, err := h.Sessions.Marks(info.ID, shell.ID)
			if err != nil {
				// One unreadable log must not blank the whole feed.
				continue
			}
			for _, m := range marks {
				if sinceMS > 0 && m.Time < sinceMS {
					continue
				}
				if a, ok := activityFromMark(info.ID, info.Name, shell.ID, m); ok {
					events = append(events, a)
				}
			}
		}
	}

	// Newest first: the feed is read from "what just happened" backwards. The
	// tiebreak is deterministic so two marks in the same millisecond do not
	// reorder between requests.
	sort.Slice(events, func(i, j int) bool {
		if events[i].UnixMS != events[j].UnixMS {
			return events[i].UnixMS > events[j].UnixMS
		}
		if events[i].SessionID != events[j].SessionID {
			return events[i].SessionID < events[j].SessionID
		}
		return events[i].Bytes < events[j].Bytes
	})
	if len(events) > limit {
		events = events[:limit]
	}
	if events == nil {
		events = []Activity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": events})
}
