package execution

import (
	"context"
	"errors"
	"net/http"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// NewFetcher is the way an `http.get` node reaches the outside: the administrator's policy (client), the workspace's grant, the
// owner's secret for the header, the log. It is the only Fetcher the server gives a run.
func (s *Store) NewFetcher(client *egress.Client, scriptID int64) graph.Fetcher {
	return func(ctx context.Context, req graph.FetchRequest) (graph.FetchResult, error) {
		denied := func(msg string) (graph.FetchResult, error) {
			return graph.FetchResult{}, &graph.FetchFailure{Kind: graph.FetchDenied, Message: msg}
		}
		if client == nil || !client.Policy().Enabled() {
			return denied("администратор сервера не разрешил исходящие запросы")
		}
		if !s.EgressEnabled(ctx) {
			return denied("для этого пространства исходящие запросы не включены администратором")
		}
		header := map[string]string{"Accept": "application/json, text/plain;q=0.9, */*;q=0.1"}
		if a := req.Auth; a != nil {
			if req.SecretsOf.ID == 0 && req.SecretsOf.Kind != "workspace" {
				return denied("для заголовка с секретом нужен вход secrets, подключённый к источнику")
			}
			owner := SecretOwner{Kind: req.SecretsOf.Kind, ID: req.SecretsOf.ID}
			if owner.Kind == "workspace" {
				owner = WorkspaceSecretOwner(ctx)
			}
			vals, err := s.ResolveSecrets(ctx, owner)
			if err != nil {
				return graph.FetchResult{}, &graph.FetchFailure{Kind: graph.FetchError, Message: "секреты не прочитать"}
			}
			v, ok := vals[a.Secret]
			if !ok {
				return graph.FetchResult{}, &graph.FetchFailure{Kind: graph.FetchError, Message: "секрет " + a.Secret + " не задан"}
			}
			header[a.Header] = a.Prefix + v
		}
		res, err := client.Do(ctx, egress.Request{Workspace: wsctx.From(ctx), ScriptID: scriptID, Method: http.MethodGet, URL: req.URL, Header: header})
		if err != nil {
			var d *egress.Denied
			switch {
			case errors.As(err, &d):
				return denied(d.Error())
			case errors.Is(err, egress.ErrTooLarge):
				return graph.FetchResult{}, &graph.FetchFailure{Kind: graph.FetchTooLarge, Message: err.Error()}
			case errors.Is(err, context.DeadlineExceeded):
				return graph.FetchResult{}, &graph.FetchFailure{Kind: graph.FetchTimeout, Message: "ответ не пришёл вовремя"}
			}
			return graph.FetchResult{}, &graph.FetchFailure{Kind: graph.FetchError, Message: err.Error()}
		}
		if !res.TextLike() {
			return graph.FetchResult{}, &graph.FetchFailure{Kind: graph.FetchBadContent, Message: "тип ответа не текст и не JSON", Status: res.Status}
		}
		return graph.FetchResult{Status: res.Status, Body: res.Body}, nil
	}
}
