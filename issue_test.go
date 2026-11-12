package ghrepo

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The open issue holding the text is found on any page; pull requests and
// issues without it are passed over.
func TestFindOpenIssue(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"GET /repos/o/n/issues": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page") != "2" {
				w.Header().Set("Link", `<`+r.URL.Path+`?page=2>; rel="next"`)
				writeJSON(w, http.StatusOK, []map[string]any{
					{"number": 1, "body": "other"},
					{"number": 2, "body": "marker", "pull_request": map[string]any{"url": "x"}},
				})

				return
			}

			writeJSON(w, http.StatusOK, []map[string]any{{"number": 3, "body": "says marker here"}})
		},
	})
	r := &Repository{owner: "o", name: "n", s: s}

	issue, err := r.FindOpenIssue(t.Context(), "marker")
	if err != nil {
		t.Fatal(err)
	}

	if issue.GetNumber() != 3 {
		t.Fatalf("found #%d, want #3", issue.GetNumber())
	}

	if q := rec.requests()[0].Query; !strings.Contains(q, "state=open") {
		t.Errorf("listed with %q, want only open issues", q)
	}

	if issue, err := r.FindOpenIssue(t.Context(), "nowhere"); err != nil || issue != nil {
		t.Errorf("FindOpenIssue(nowhere) = %v, %v; want nil, nil", issue, err)
	}
}

func TestCreateAndEditIssue(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"POST /repos/o/n/issues":    respond(http.StatusCreated, map[string]any{"number": 7}),
		"PATCH /repos/o/n/issues/7": respond(http.StatusOK, map[string]any{"number": 7}),
	})
	r := &Repository{owner: "o", name: "n", s: s}

	issue, err := r.CreateIssue(t.Context(), "Tests fail", "first")
	if err != nil || issue.GetNumber() != 7 {
		t.Fatalf("CreateIssue = %v, %v; want #7", issue, err)
	}

	if _, err := r.EditIssue(t.Context(), 7, "Tests fail", "again"); err != nil {
		t.Fatal(err)
	}

	for i, want := range []string{"first", "again"} {
		var body map[string]string
		if err := json.Unmarshal(rec.requests()[i].Body, &body); err != nil {
			t.Fatal(err)
		}

		if body["title"] != "Tests fail" || body["body"] != want {
			t.Errorf("request %d sent %v, want title %q and body %q", i, body, "Tests fail", want)
		}
	}
}
