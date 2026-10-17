package kernel

import (
	"context"
	"errors"
	"testing"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

type fakeRoles map[int64]string

func (f fakeRoles) MemberRole(_ context.Context, _, userID int64) (string, error) {
	return f[userID], nil
}

func TestCommandsPolicy(t *testing.T) {
	str := func(s string) *string { return &s }
	c := Commands{Roles: fakeRoles{1: identity.RoleOwner, 2: identity.RoleMember}}
	as := func(uid int64) context.Context { return wsctx.WithUser(wsctx.With(t.Context(), 1), uid) }

	cases := []struct {
		name            string
		ctx             context.Context
		cmd, old, runOn *string
		want            error
	}{
		{"owner with a runner", as(1), str("true"), nil, str("pc"), nil},
		{"owner without a runner", as(1), str("true"), nil, nil, ErrNoRunner},
		{"member writes a command", as(2), str("true"), nil, str("pc"), ErrCommandOwner},
		{"member leaves the command as it was", as(2), str("true"), str("true"), str("pc"), nil},
		{"member changes it", as(2), str("false"), str("true"), str("pc"), ErrCommandOwner},
		{"automation actor", as(wsctx.ActorAutomation), str("true"), nil, str("pc"), ErrCommandOwner},
		{"no command needs neither", as(2), nil, nil, nil, nil},
		{"blank command needs neither", as(2), str("  "), nil, nil, nil},
		{"background work (no user)", wsctx.With(t.Context(), 1), str("true"), nil, str("pc"), nil},
	}
	for _, tc := range cases {
		if got := c.Check(tc.ctx, tc.cmd, tc.old, tc.runOn); !errors.Is(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
