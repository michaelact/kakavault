package pathresolver

import "testing"

const (
	nsPattern     = `^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$`
	secretPattern = `^(?P<application>.+)-secret-variables$`
)

func TestResolve_OK(t *testing.T) {
	got, err := Resolve(nsPattern, secretPattern, "myrepo-staging", "backend-secret-variables")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Resolved{Repo: "myrepo", Environment: "staging", Application: "backend"}
	if got != want {
		t.Errorf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestResolve_RepoNameWithHyphens(t *testing.T) {
	// The repo group is greedy (.+), so hyphens inside the repo name
	// itself must still leave the LAST hyphen-delimited segment as the
	// environment.
	got, err := Resolve(nsPattern, secretPattern, "my-multi-word-repo-prod", "ui-secret-variables")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := Resolved{Repo: "my-multi-word-repo", Environment: "prod", Application: "ui"}
	if got != want {
		t.Errorf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestResolve_NamespaceNoMatch(t *testing.T) {
	_, err := Resolve(nsPattern, secretPattern, "nohyphen", "backend-secret-variables")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error for a namespace that doesn't match the pattern")
	}
}

func TestResolve_SecretNameNoMatch(t *testing.T) {
	_, err := Resolve(nsPattern, secretPattern, "myrepo-staging", "backend-wrong-suffix")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error for a Secret name that doesn't match the pattern")
	}
}

func TestResolve_BadNamespacePattern(t *testing.T) {
	_, err := Resolve("(unclosed", secretPattern, "myrepo-staging", "backend-secret-variables")
	if err == nil {
		t.Fatal("Resolve() error = nil, want an error for an invalid namespace_pattern regex")
	}
}

func TestBuildPath(t *testing.T) {
	got := BuildPath("myrepo", "staging", "backend", "internal")
	want := "myrepo/staging/backend/internal"
	if got != want {
		t.Errorf("BuildPath() = %q, want %q", got, want)
	}
}
