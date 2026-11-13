package identity

import (
	"strings"
	"testing"
)

func TestWorkspaceAppearance(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	color, icon := "#6a7ab8", "map"
	if err := s.UpdateWorkspace(ctx, 1, nil, &color, &icon, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := s.AllWorkspaces(ctx)
	if err != nil || len(rows) == 0 {
		t.Fatalf("%v %v", rows, err)
	}
	if rows[0].Color != color || rows[0].Icon != icon || rows[0].Name != "Основное" {
		t.Fatal(rows[0])
	}
	name := strings.Repeat("я", 81)
	if err := s.UpdateWorkspace(ctx, 1, &name, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := s.WorkspaceName(ctx, 1)
	if len([]rune(got)) != 80 {
		t.Fatal(got)
	}
	badColor := "red;display:none"
	if err := s.UpdateWorkspace(ctx, 1, nil, &badColor, nil, nil); err != ErrWorkspaceAppearance {
		t.Fatal(err)
	}
	badIcon := "missing"
	other := "changed"
	if err := s.UpdateWorkspace(ctx, 1, &other, nil, &badIcon, nil); err != ErrWorkspaceAppearance {
		t.Fatal(err)
	}
	after, _ := s.WorkspaceName(ctx, 1)
	if after != got {
		t.Fatal("invalid appearance changed name")
	}
	u, err := s.CreateUser("appearance", "longenoughpassword")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddWorkspaceMember(ctx, 1, u.ID, "member"); err != nil {
		t.Fatal(err)
	}
	list, err := s.UserWorkspaces(ctx, u.ID)
	if err != nil || len(list) != 1 || list[0].Color != color || list[0].Icon != icon {
		t.Fatalf("%+v %v", list, err)
	}
}

func TestWorkspaceStableSlugs(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	slug := "my-space"
	if err := s.UpdateWorkspace(ctx, 1, nil, nil, nil, &slug); err != nil {
		t.Fatal(err)
	}
	name := "Different title"
	if err := s.UpdateWorkspace(ctx, 1, &name, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := s.AllWorkspaces(ctx)
	if err != nil || rows[0].Slug != slug {
		t.Fatalf("%v %v", rows, err)
	}
	invalid := "NOT valid"
	if err := s.UpdateWorkspace(ctx, 1, nil, nil, nil, &invalid); err != ErrWorkspaceSlug {
		t.Fatal(err)
	}
	user, err := s.CreateUser("slugs", "longenoughpassword")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateWorkspace(ctx, user.ID, "Other")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateWorkspace(ctx, other.ID, nil, nil, nil, &slug); err != ErrWorkspaceSlugTaken {
		t.Fatal(err)
	}
}
