package kernel

import (
	"context"
	"errors"
	"strings"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/wsctx"
)

// ErrCommandOwner is returned when a member (not an owner) sets a command.
var ErrCommandOwner = errors.New("команды проверок и источников могут задавать только владельцы пространства")

// ErrNoRunner is returned for a command with no runner: commands execute on runners only
// (quests-runner on any device, the server's own host included).
var ErrNoRunner = errors.New("команды запускаются только на раннере — выберите устройство (раннер)")

// RoleLookup says what role a user has in a workspace ("" = none); identity.Store is one.
type RoleLookup interface {
	MemberRole(ctx context.Context, workspaceID, userID int64) (string, error)
}

// CommandAuthor refuses a command written by someone who is not an owner of the workspace. A command runs with the
// authority of the machine that executes it (a runner the owner registered), so authoring one is an owner's right.
// old is the command already stored (nil for a new one): leaving it unchanged is fine, so a member can still edit
// the rest of a step, a template or a note.
func CommandAuthor(ctx context.Context, roles RoleLookup, command, old *string) error {
	if command == nil || strings.TrimSpace(*command) == "" {
		return nil
	}
	if old != nil && strings.TrimSpace(*old) == strings.TrimSpace(*command) {
		return nil
	}
	uid := wsctx.UserFrom(ctx)
	if uid == 0 {
		return nil // background work, auth off
	}
	if uid == wsctx.ActorAutomation {
		return ErrCommandOwner
	}
	role, err := roles.MemberRole(ctx, wsctx.From(ctx), uid)
	if err != nil {
		return err
	}
	if role != identity.RoleOwner {
		return ErrCommandOwner
	}
	return nil
}

// CommandExec refuses a command that has no runner (non-empty command, run_on empty).
func CommandExec(command, runOn *string) error {
	if command == nil || strings.TrimSpace(*command) == "" {
		return nil
	}
	if runOn != nil && strings.TrimSpace(*runOn) != "" {
		return nil
	}
	return ErrNoRunner
}
