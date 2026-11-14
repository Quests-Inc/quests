package execution

import (
	"context"
	"database/sql"

	"github.com/quests-inc/quests/go/internal/platform/limits"
)

// CheckCopiedValueBudget validates a new environment inside the caller's creation transaction.
// It reads no secrets, executes no formulas or rolls, and leaves inherited values inherited.
func (s *Store) CheckCopiedValueBudget(ctx context.Context, tx *sql.Tx, o SecretOwner, hidePrivate bool) error {
	scoped := *s
	scoped.valueTx = tx
	set, err := scoped.collectNamedValues(ctx, o, hidePrivate)
	if err != nil {
		return err
	}
	size := 0
	for _, v := range set.Own {
		size += len(v.Name) + len(v.Value) + len(v.Formula) + len(v.RollArg)
	}
	if len(set.Own) > int(limits.Get("values_own")) || size > int(limits.Get("values_bytes")) || len(set.Effective) > int(limits.Get("values_effective")) || set.Truncated {
		return &ValueError{Msg: "переменные копии превышают текущую квоту"}
	}
	links, err := scoped.valueLinks(ctx, o)
	if err != nil {
		return err
	}
	if len(set.Own) > 0 && o.Kind == SecretOwnerNote && len(links)-1 > int(limits.Get("values_chain")) {
		return &ValueError{Msg: "превышена глубина наследования переменных"}
	}
	return nil
}
