package graph

// Sample is a ready script to start from: the library's examples (ADR-0003). It is data, not code that runs by itself: using
// one copies its graph into the workspace, where the owner edits it. Anything that runs is the owner's text on a runner.
type Sample struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Subject is the kind of entity the script reads (`step` or `note`).
	Subject     string `json:"subject"`
	NeedsRunner bool   `json:"needs_runner"`
	Graph       Graph  `json:"graph"`
}

const freshnessScript = `# Заметка устарела, если после зафиксированного коммита изменились файлы, о которых она.
# Меняются только REF (хеш коммита, с которого заметка актуальна), D (репозиторий) и PATHS.
REF=0000000
D="$HOME/projects/repo"
PATHS="docs/guide.md"
git -C "$D" fetch -q origin || exit 1
CH=$(git -C "$D" diff --name-only "$REF" origin/main -- $PATHS | tr '\n' ' ')
if [ -n "$CH" ]; then
  echo "{\"outdated\": 1, \"reason\": \"изменились: $CH\"}"
else
  echo '{"outdated": 0}'
fi`

const httpProgressScript = `#!/bin/sh
# Прогресс из JSON по HTTP. Адрес лежит в секрете URL (секреты пространства, квеста или самого шага).
# Ответ сервиса вида {"done": 3, "all": 10} превращается в {"progress": 3, "total": 10}.
curl -fsS "$URL" | jq -c '{progress: .done, total: .all}'`

const templateEmitScript = `# напечатайте один квест JSON-документом; {} — квеста сейчас нет
echo '{"title": "Квест из скрипта", "steps": [{"title": "Шаг"}]}'`

// Samples returns the library's examples.
func Samples() []Sample {
	followProgress := buildGraph([]nodeSpec{
		{1, "source.step", nil},
		{2, "compare", kv{"type": "int", "op": "ge", "operand": 1}},
		{3, "sink.step_result", kv{"result": "done"}},
	},
		[4]any{1, "progress", 2, "in"}, [4]any{1, "ref", 3, "ref"}, [4]any{2, "yes", 3, "trigger"},
	)
	return []Sample{
		{
			ID: "step-number", Title: "Шаг: прогресс из числа", Subject: "step", NeedsRunner: true,
			Description: "Команда печатает число (прогресс шага) или JSON с progress, total и description. Всё, что в ответе есть, попадает в шаг.",
			Graph:       StepPoll("# напечатайте число — прогресс шага, или JSON {\"progress\": 2, \"total\": 5}\necho 0"),
		},
		{
			ID: "step-http-json", Title: "Шаг: прогресс из JSON по HTTP", Subject: "step", NeedsRunner: true,
			Description: "Запрашивает адрес из секрета URL и переводит ответ в progress и total. Нужны curl и jq на устройстве.",
			Graph:       StepPoll(httpProgressScript),
		},
		{
			ID: "step-once", Title: "Шаг: выполнить один раз", Subject: "step", NeedsRunner: true,
			Description: "Один запуск: код 0 закрывает шаг, любая ошибка проваливает квест; устройство не вышло на связь — шаг ждёт, отмена вручную — шаг остановлен.",
			Graph:       StepOnce("# код 0 закрывает шаг\ntrue"),
		},
		{
			ID: "step-self-close", Title: "Шаг: закрыться при прогрессе от 1", Subject: "step", NeedsRunner: false,
			Description: "Без команды и без раннера: сервер сам проверяет прогресс шага и закрывает его, когда он дошёл до порога. Порог — в ноде «Сравнить».",
			Graph:       followProgress,
		},
		{
			ID: "template-emit", Title: "Шаблон: квест из команды", Subject: "template", NeedsRunner: true,
			Description: "Команда печатает один квест JSON-документом (пусто, {} или null — квеста сейчас нет). Сбой команды и неверный ответ — ошибка попытки: выпуск повторится. Устройство не вышло на связь или запуск отменён — период пропускается без повтора.",
			Graph:       TemplateEmit(templateEmitScript),
		},
		{
			ID: "note-freshness", Title: "Заметка: актуальность по коммиту", Subject: "note", NeedsRunner: true,
			Description: "Сравнивает зафиксированный коммит с origin/main по списку файлов: изменились — заметка устарела, с причиной.",
			Graph:       NoteCheck(freshnessScript),
		},
		{
			ID: "event-hook", Title: "Хук: команда на событие", Subject: "quest", NeedsRunner: true,
			Description: "Срабатывает на событие квеста (хук в пространстве). Команда видит JSON события в $QUESTS_PAYLOAD; код 0 — готово.",
			Graph: buildGraph([]nodeSpec{
				{1, "source.quest", nil},
				{2, "code", kv{"text": "# событие в $QUESTS_PAYLOAD\ntrue"}},
				{3, "sink.done", nil},
			},
				[4]any{1, "secrets", 2, "secrets"}, [4]any{2, "stdout", 3, "trigger"},
			),
		},
		{
			ID: "event-webhook", Title: "Хук: webhook", Subject: "quest", NeedsRunner: false,
			Description: "Шлёт JSON события POST-ом на URL (параметр ноды «HTTP-запрос»). Без раннера: сервер сам делает запрос.",
			Graph: buildGraph([]nodeSpec{
				{1, "source.quest", nil},
				{2, "sink.http", kv{"url": "https://example.com/hook", "method": "POST"}},
			},
				[4]any{1, "event", 2, "body"},
			),
		},
	}
}
