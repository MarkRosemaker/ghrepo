package ghrepo

import (
	"encoding/json"
	"net/http"
	"slices"
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

	if issue, err := r.FindOpenIssue(t.Context(), "nowhere", "devtool", "coverage"); err != nil || issue != nil {
		t.Errorf("FindOpenIssue(nowhere) = %v, %v; want nil, nil", issue, err)
	}

	if q := rec.requests()[len(rec.requests())-1].Query; !strings.Contains(q, "labels=devtool%2Ccoverage") {
		t.Errorf("listed with %q, want only issues with both labels", q)
	}
}

func TestCreateAndEditIssue(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"POST /repos/o/n/issues":    respond(http.StatusCreated, map[string]any{"number": 7}),
		"PATCH /repos/o/n/issues/7": respond(http.StatusOK, map[string]any{"number": 7}),
	})
	r := &Repository{owner: "o", name: "n", s: s}

	issue, err := r.CreateIssue(t.Context(), "Tests fail", "first", "devtool")
	if err != nil || issue.GetNumber() != 7 {
		t.Fatalf("CreateIssue = %v, %v; want #7", issue, err)
	}

	if _, err := r.EditIssue(t.Context(), 7, "Tests fail", "again"); err != nil {
		t.Fatal(err)
	}

	for i, want := range []string{"first", "again"} {
		var body struct {
			Title, Body string
			Labels      []string
		}
		if err := json.Unmarshal(rec.requests()[i].Body, &body); err != nil {
			t.Fatal(err)
		}

		if body.Title != "Tests fail" || body.Body != want {
			t.Errorf("request %d sent %+v, want title %q and body %q", i, body, "Tests fail", want)
		}

		if wantLabels := []string{"devtool"}; i == 0 && !slices.Equal(body.Labels, wantLabels) {
			t.Errorf("created with labels %v, want %v", body.Labels, wantLabels)
		}
	}
}

// A missing label is created; one the repository has is left alone.
func TestEnsureLabel(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"GET /repos/o/n/labels/coverage": respond(http.StatusNotFound, map[string]any{"message": "Not Found"}),
		"GET /repos/o/n/labels/devtool":  respond(http.StatusOK, map[string]any{"name": "devtool"}),
		"POST /repos/o/n/labels":         respond(http.StatusCreated, map[string]any{"name": "coverage"}),
	})
	r := &Repository{owner: "o", name: "n", s: s}

	for _, name := range []string{"coverage", "devtool"} {
		if err := r.EnsureLabel(t.Context(), name, "fbca04", "about "+name); err != nil {
			t.Fatalf("EnsureLabel(%s): %v", name, err)
		}
	}

	var created []string
	for _, req := range rec.requests() {
		if req.Method == http.MethodPost {
			created = append(created, string(req.Body))
		}
	}

	if len(created) != 1 || !strings.Contains(created[0], `"name":"coverage"`) || !strings.Contains(created[0], `"color":"fbca04"`) {
		t.Errorf("created %v, want only coverage, in its colour", created)
	}
}

func TestEnsureLabelFailsOnOtherErrors(t *testing.T) {
	t.Parallel()

	s, _ := newTestService(t, map[string]http.HandlerFunc{
		"GET /repos/o/n/labels/devtool": respond(http.StatusForbidden, map[string]any{"message": "Forbidden"}),
	})
	r := &Repository{owner: "o", name: "n", s: s}

	if err := r.EnsureLabel(t.Context(), "devtool", "5319e7", ""); err == nil {
		t.Fatal("a refused lookup read as a missing label")
	}
}
