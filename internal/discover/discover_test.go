package discover

import (
	"context"
	"errors"
	"sort"
	"testing"
)

const (
	nsPattern     = `^(?P<repo>.+)-(?P<environment>[a-z0-9]+)$`
	secretPattern = `^(?P<application>.+)-secret-variables$`
)

// fakeLister is an in-memory Lister for tests — no k8s dependency needed
// here, discover only needs List* semantics.
type fakeLister struct {
	namespaces     []string
	secretsByNS    map[string][]string
	listSecretsErr map[string]error
}

func (f *fakeLister) ListNamespaces(ctx context.Context) ([]string, error) {
	return f.namespaces, nil
}

func (f *fakeLister) ListSecrets(ctx context.Context, namespace string) ([]string, error) {
	if err := f.listSecretsErr[namespace]; err != nil {
		return nil, err
	}
	return f.secretsByNS[namespace], nil
}

func TestFind_MatchesBothPatterns(t *testing.T) {
	lister := &fakeLister{
		namespaces: []string{"myrepo-staging", "myrepo-prod"},
		secretsByNS: map[string][]string{
			"myrepo-staging": {"backend-secret-variables", "default-token-abc"},
			"myrepo-prod":    {"ui-secret-variables"},
		},
	}

	got, err := Find(context.Background(), lister, nsPattern, secretPattern)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].Namespace < got[j].Namespace })

	if len(got) != 2 {
		t.Fatalf("len(Find()) = %d, want 2; got %+v", len(got), got)
	}
	if got[0] != (Target{Namespace: "myrepo-prod", SecretName: "ui-secret-variables"}) {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1] != (Target{Namespace: "myrepo-staging", SecretName: "backend-secret-variables"}) {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestFind_SkipsNonMatchingNamespace(t *testing.T) {
	lister := &fakeLister{
		namespaces: []string{"myrepo-staging", "nohyphen"},
		secretsByNS: map[string][]string{
			"myrepo-staging": {"backend-secret-variables"},
			"nohyphen":       {"backend-secret-variables"},
		},
	}

	got, err := Find(context.Background(), lister, nsPattern, secretPattern)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Find()) = %d, want 1; got %+v", len(got), got)
	}
	if got[0].Namespace != "myrepo-staging" {
		t.Errorf("got[0].Namespace = %q, want %q", got[0].Namespace, "myrepo-staging")
	}
}

func TestFind_SkipsSystemNamespaces(t *testing.T) {
	// None of these would ever match nsPattern anyway (no trailing
	// -<environment> segment), but discover must not even bother listing
	// their secrets.
	lister := &fakeLister{
		namespaces: []string{"kube-system", "kube-public", "kube-node-lease", "myrepo-staging"},
		secretsByNS: map[string][]string{
			"myrepo-staging": {"backend-secret-variables"},
		},
		listSecretsErr: map[string]error{
			"kube-system":     errors.New("ListSecrets must not be called for kube-system"),
			"kube-public":     errors.New("ListSecrets must not be called for kube-public"),
			"kube-node-lease": errors.New("ListSecrets must not be called for kube-node-lease"),
		},
	}

	got, err := Find(context.Background(), lister, nsPattern, secretPattern)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Find()) = %d, want 1; got %+v", len(got), got)
	}
}

func TestFind_NoMatches(t *testing.T) {
	lister := &fakeLister{
		namespaces: []string{"nohyphen"},
		secretsByNS: map[string][]string{
			"nohyphen": {"some-secret"},
		},
	}

	got, err := Find(context.Background(), lister, nsPattern, secretPattern)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(Find()) = %d, want 0", len(got))
	}
}

func TestFind_SkipsNonMatchingSecretName(t *testing.T) {
	lister := &fakeLister{
		namespaces: []string{"myrepo-staging"},
		secretsByNS: map[string][]string{
			"myrepo-staging": {"backend-secret-variables", "default-token-abc", "tls-cert"},
		},
	}

	got, err := Find(context.Background(), lister, nsPattern, secretPattern)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(got) != 1 || got[0].SecretName != "backend-secret-variables" {
		t.Errorf("Find() = %+v, want exactly one Target for backend-secret-variables", got)
	}
}

func TestFind_BadPattern(t *testing.T) {
	lister := &fakeLister{namespaces: []string{"myrepo-staging"}}

	_, err := Find(context.Background(), lister, "(unclosed", secretPattern)
	if err == nil {
		t.Fatal("Find() error = nil, want an error for an invalid namespace_pattern regex")
	}
}

func TestFindInNamespace_MatchesSecretPattern(t *testing.T) {
	lister := &fakeLister{
		secretsByNS: map[string][]string{
			"myrepo-staging": {"backend-secret-variables", "default-token-abc", "ui-secret-variables"},
		},
	}

	got, err := FindInNamespace(context.Background(), lister, "myrepo-staging", secretPattern)
	if err != nil {
		t.Fatalf("FindInNamespace() error = %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].SecretName < got[j].SecretName })

	if len(got) != 2 {
		t.Fatalf("len(FindInNamespace()) = %d, want 2; got %+v", len(got), got)
	}
	if got[0].SecretName != "backend-secret-variables" || got[0].Namespace != "myrepo-staging" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].SecretName != "ui-secret-variables" || got[1].Namespace != "myrepo-staging" {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestFindInNamespace_NoMatches(t *testing.T) {
	lister := &fakeLister{
		secretsByNS: map[string][]string{
			"myrepo-staging": {"default-token-abc"},
		},
	}

	got, err := FindInNamespace(context.Background(), lister, "myrepo-staging", secretPattern)
	if err != nil {
		t.Fatalf("FindInNamespace() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(FindInNamespace()) = %d, want 0", len(got))
	}
}

func TestFindInNamespace_BadPattern(t *testing.T) {
	lister := &fakeLister{}

	_, err := FindInNamespace(context.Background(), lister, "myrepo-staging", "(unclosed")
	if err == nil {
		t.Fatal("FindInNamespace() error = nil, want an error for an invalid secret_name_pattern regex")
	}
}
